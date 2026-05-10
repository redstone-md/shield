package events

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/app/slowpath"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

func (l *TelegramListener) makeIncomingEvent(update tbapi.Update, msg *bot.Message) moderation.IncomingEvent {
	subjectID := msg.From.ID
	subjectUserName := msg.From.Username
	if msg.SenderChat.ID != 0 {
		subjectID = msg.SenderChat.ID
		subjectUserName = msg.SenderChat.UserName
	}

	editedMessageID := 0
	if update.EditedMessage != nil {
		editedMessageID = update.EditedMessage.MessageID
	}

	return moderation.IncomingEvent{
		EventID:         l.nextEventID(),
		CorrelationID:   l.currentCorrelationID(),
		TenantID:        l.TenantID,
		Source:          "telegram.update",
		UpdateID:        update.UpdateID,
		ChatID:          msg.ChatID,
		MessageID:       msg.ID,
		EditedMessageID: editedMessageID,
		IdempotencyKey:  telegramIdempotencyKey(update.UpdateID, msg.ChatID, msg.ID, editedMessageID),
		Subject: moderation.Subject{
			ID:       subjectID,
			UserName: subjectUserName,
			IsBot:    msg.From.ID == 136817688,
		},
		Content: moderation.Content{
			Text:  msg.Text,
			Links: collectLinks(msg),
			HasMedia: msg.Image != nil || msg.WithVideo || msg.WithVideoNote || msg.WithAudio || msg.WithSticker ||
				msg.Animation != nil || msg.CustomEmojiID != "",
			Attributes: incomingEventAttributes(msg),
		},
		ReceivedAt: msg.Sent.UTC(),
	}
}

func telegramIdempotencyKey(updateID int, chatID int64, messageID, editedMessageID int) string {
	return fmt.Sprintf("telegram:update:%d:chat:%d:message:%d:edited:%d", updateID, chatID, messageID, editedMessageID)
}

func (l *TelegramListener) nextEventID() string {
	seq := atomic.AddUint64(&l.pipeline.eventID, 1)
	return fmt.Sprintf("evt-%s-%d", strings.TrimSpace(l.TenantID), seq)
}

func (l *TelegramListener) currentCorrelationID() string {
	return fmt.Sprintf("corr-%s", strings.TrimSpace(l.TenantID))
}

func incomingEventAttributes(msg *bot.Message) map[string]string {
	attrs := map[string]string{
		"with_forward":    strconv.FormatBool(msg.WithForward),
		"with_keyboard":   strconv.FormatBool(msg.WithKeyboard),
		"with_contact":    strconv.FormatBool(msg.WithContact),
		"with_giveaway":   strconv.FormatBool(msg.WithGiveaway),
		"with_video":      strconv.FormatBool(msg.WithVideo),
		"with_video_note": strconv.FormatBool(msg.WithVideoNote),
		"with_audio":      strconv.FormatBool(msg.WithAudio),
		"with_sticker":    strconv.FormatBool(msg.WithSticker),
		"with_animation":  strconv.FormatBool(msg.Animation != nil),
		"custom_emoji_id": msg.CustomEmojiID,
	}
	if msg.SenderChat.ID != 0 {
		attrs["sender_chat_id"] = strconv.FormatInt(msg.SenderChat.ID, 10)
		if msg.SenderChat.UserName != "" {
			attrs["sender_chat_username"] = msg.SenderChat.UserName
		}
	}
	return attrs
}

func collectLinks(msg *bot.Message) []string {
	var links []string
	appendLinks := func(entities *[]bot.Entity) {
		if entities == nil {
			return
		}
		for _, entity := range *entities {
			switch entity.Type {
			case "url":
				links = append(links, entity.URL)
			case "text_link":
				if entity.URL != "" {
					links = append(links, entity.URL)
				}
			}
		}
	}

	appendLinks(msg.Entities)
	if msg.Image != nil {
		appendLinks(msg.Image.Entities)
	}
	return links
}

func (l *TelegramListener) completeIncomingEvent(ctx context.Context, event moderation.IncomingEvent,
	decision moderation.PolicyDecision, actionResult moderation.ModerationActionResult,
) error {
	if l.IncomingEvents == nil {
		return nil
	}
	if err := l.IncomingEvents.Complete(ctx, event.IdempotencyKey, decision, actionResult); err != nil {
		return fmt.Errorf("complete incoming event %s: %w", event.EventID, err)
	}
	return nil
}

type contextualBot interface {
	OnMessageWithContext(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response
}

func (l *TelegramListener) botOnMessage(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response {
	if b, ok := l.Bot.(contextualBot); ok {
		return b.OnMessageWithContext(ctx, msg, checkOnly)
	}
	return l.Bot.OnMessage(msg, checkOnly)
}

func (l *TelegramListener) mediaSlowPathConfig() mediaSlowPathConfig {
	return mediaSlowPathConfig{
		enabled:        l.SlowPathEnabled,
		api:            l.TbAPI,
		engine:         l.SlowPathEngine,
		tenantID:       l.TenantID,
		observeLatency: l.observeLatency,
		meter:          l.meter,
		sleep:          time.Sleep,
	}
}

type mediaSlowPathConfig struct {
	enabled        bool
	api            TbAPI
	engine         SlowPathChecker
	tenantID       string
	observeLatency func(string, time.Duration)
	meter          func(context.Context, string)
	sleep          func(time.Duration)
}

func applyMediaSlowPath(ctx context.Context, cfg mediaSlowPathConfig, event moderation.IncomingEvent,
	msg *bot.Message, resp bot.Response,
) bot.Response {
	if !cfg.enabled || cfg.engine == nil || resp.Send {
		return resp
	}

	dl := newImageDownloader(cfg.api)
	fileID, slowReason := mediaSlowPathFile(ctx, cfg, msg)
	if fileID == "" {
		return resp
	}

	data, mime, dlErr := dl.download(ctx, fileID)
	if dlErr != nil {
		observability.Logf(ctx, "[WARN] file download failed for slowpath: %v", dlErr)
		return resp
	}
	defer func() { data = nil }()
	observability.Logf(ctx, "[DEBUG] downloaded file for slowpath: %d bytes, mime=%s", len(data), mime)
	logMediaSlowPathPayload(ctx, mime, data)

	slowReq := slowpath.SlowPathRequest{
		EventID:       event.EventID,
		CorrelationID: event.CorrelationID,
		TenantID:      cfg.tenantID,
		Reason:        slowReason,
		Content:       slowpath.Content{Text: msg.Text, HasMedia: true},
		ImageData:     data,
		ImageMIME:     mime,
	}

	slowStart := time.Now()
	slowResult, slowErr := checkSlowPathWithRetry(ctx, cfg, slowReq)
	if cfg.observeLatency != nil {
		cfg.observeLatency("slow_path_latency", time.Since(slowStart))
	}

	if slowErr != nil {
		observability.Logf(ctx, "[WARN] slowpath check failed: %v", slowErr)
		resp.CheckResults = append(resp.CheckResults, slowPathErrorCheck(slowErr))
		return resp
	}
	if slowResult == nil {
		observability.Logf(ctx, "[WARN] slowpath check returned no result")
		return resp
	}

	observability.Logf(ctx, "[INFO] slowpath completed: skipped=%v spam=%v confidence=%d providers=%s reason=%s",
		slowResult.Skipped, slowResult.Spam, slowResult.Confidence, strings.Join(slowResult.Providers, ","), slowResult.Reason)
	if slowResult.Skipped {
		return resp
	}
	resp.CheckResults = append(resp.CheckResults, slowPathCheckResponses(slowResult)...)
	if !slowResult.Spam {
		return resp
	}

	observability.Logf(ctx, "[INFO] slowpath detected spam: confidence=%d, reason=%s", slowResult.Confidence, slowResult.Reason)
	if cfg.meter != nil {
		cfg.meter(ctx, "slowpath_spam")
	}
	resp.Send = true
	resp.User = msg.From
	resp.ReplyTo = msg.ID
	return resp
}

func logMediaSlowPathPayload(ctx context.Context, mime string, data []byte) {
	headLen := min(len(data), 64)
	encoded := base64.StdEncoding.EncodeToString(data)
	prefix := "data:" + mime + ";base64,"
	previewLen := min(len(encoded), 128)
	sum := sha256.Sum256(data)
	observability.Logf(ctx,
		"[DEBUG] slowpath image payload: mime=%s image_bytes=%d image_sha256=%x image_head_hex=%x data_url_len=%d data_url_prefix=%q",
		mime, len(data), sum, data[:headLen], len(prefix)+len(encoded), prefix+encoded[:previewLen],
	)
}

func slowPathCheckResponses(result *slowpath.SlowPathResult) []spamcheck.Response {
	if len(result.Signals) > 0 {
		checks := make([]spamcheck.Response, 0, len(result.Signals))
		for _, signal := range result.Signals {
			checks = append(checks, spamcheck.Response{
				Name:    slowPathCheckName(signal.Provider),
				Spam:    signal.Spam,
				Details: slowPathCheckDetails(signal.Reason, signal.Confidence),
			})
		}
		return checks
	}

	return []spamcheck.Response{{
		Name:    slowPathCheckName(strings.Join(result.Providers, ",")),
		Spam:    result.Spam,
		Details: slowPathCheckDetails(result.Reason, result.Confidence),
	}}
}

func slowPathErrorCheck(err error) spamcheck.Response {
	return spamcheck.Response{Name: "slowpath", Spam: false, Details: "error: " + err.Error(), Error: err}
}

func slowPathCheckName(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "slowpath"
	}
	return provider
}

func slowPathCheckDetails(reason string, confidence int) string {
	reason = strings.TrimSpace(reason)
	confidenceText := fmt.Sprintf("confidence: %d%%", confidence)
	if reason == "" {
		return confidenceText
	}
	return reason + ", " + confidenceText
}

func mediaSlowPathFile(ctx context.Context, cfg mediaSlowPathConfig, msg *bot.Message) (string, slowpath.EscalationReason) {
	if msg.Image != nil && msg.Image.FileID != "" {
		return msg.Image.FileID, slowpath.EscalationImageContent
	}
	if msg.Animation != nil {
		if msg.Animation.ThumbFileID != "" {
			return msg.Animation.ThumbFileID, slowpath.EscalationImageContent
		}
		observability.Logf(ctx, "[DEBUG] animation slowpath using original file for first-frame extraction: mime=%s", msg.Animation.MimeType)
		return msg.Animation.FileID, slowpath.EscalationImageContent
	}
	if msg.WithSticker && msg.Sticker != nil {
		return stickerDownloadFileID(msg.Sticker), slowpath.EscalationImageContent
	}
	if msg.CustomEmojiID != "" {
		return customEmojiDownloadFileID(ctx, cfg.api, msg.CustomEmojiID), slowpath.EscalationImageContent
	}
	return "", ""
}

func customEmojiDownloadFileID(ctx context.Context, api TbAPI, customEmojiID string) string {
	if api == nil || customEmojiID == "" {
		return ""
	}
	stickers, err := api.GetCustomEmojiStickers(tbapi.GetCustomEmojiStickersConfig{CustomEmojiIDs: []string{customEmojiID}})
	if err != nil {
		observability.Logf(ctx, "[WARN] get custom emoji sticker failed for slowpath: %v", err)
		return ""
	}
	if len(stickers) == 0 {
		return ""
	}
	info := &bot.StickerInfo{FileID: stickers[0].FileID, IsAnimated: stickers[0].IsAnimated, IsVideo: stickers[0].IsVideo}
	if stickers[0].Thumbnail != nil {
		info.ThumbFileID = stickers[0].Thumbnail.FileID
	}
	return stickerDownloadFileID(info)
}

func checkSlowPathWithRetry(ctx context.Context, cfg mediaSlowPathConfig, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error) {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		result, err := cfg.engine.Check(ctx, req)
		if err == nil || !retryableSlowPathError(err) || attempt == 9 {
			return result, err
		}
		lastErr = err
		delay := 3 * time.Second
		observability.Logf(ctx, "[WARN] slowpath check failed, retrying in %s: %v", delay, err)
		if !sleepContext(ctx, cfg.sleep, delay) {
			return nil, errors.Join(ctx.Err(), lastErr)
		}
	}
	return nil, lastErr
}

func retryableSlowPathError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	retryableMarkers := []string{"retryable", "bad gateway", "502", "503", "504", "429", "rate limit", "timeout", "temporarily unavailable"}
	for _, marker := range retryableMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func sleepContext(ctx context.Context, sleep func(time.Duration), delay time.Duration) bool {
	if sleep == nil {
		sleep = time.Sleep
	}
	done := make(chan struct{})
	go func() {
		sleep(delay)
		close(done)
	}()
	select {
	case <-ctx.Done():
		return false
	case <-done:
		return true
	}
}

func slowpathReason(checks []spamcheck.Response) string {
	for _, check := range checks {
		if check.Spam && check.Name == "slowpath" {
			return strings.TrimSpace(check.Details)
		}
	}
	return ""
}

func appendReasonHTML(text, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return text
	}
	return fmt.Sprintf("%s\nПричина: %s", text, htmlEscape(reason))
}

func firstNotificationReason(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return strings.TrimSpace(reasons[0])
}

func buildWarningText(warnNum, warnTotal int, user bot.User, userID int64, customMsg, reason string) string {
	userMention := warningUserMention(user, userID)
	if customMsg != "" {
		text := fmt.Sprintf("\u26a0\ufe0f Предупреждение %d/%d\n%s, %s", warnNum, warnTotal, userMention, htmlEscape(customMsg))
		return appendReasonHTML(text, reason)
	}

	warnText := fmt.Sprintf("\u26a0\ufe0f Предупреждение %d/%d\n%s, вы нарушили правила чата. "+
		"При получении %d предупреждений последует мьют на 30 мин, затем на 6 ч, и далее — перманентный бан.",
		warnNum, warnTotal, userMention, warnTotal)
	return appendReasonHTML(warnText, reason)
}

func warningUserMention(user bot.User, userID int64) string {
	userName := user.DisplayName
	if userName == "" {
		userName = user.Username
	}
	if userName == "" {
		userName = fmt.Sprintf("user %d", userID)
	}

	if user.Username != "" {
		return fmt.Sprintf(`<a href="https://t.me/%s">%s</a>`, user.Username, htmlEscape(userName))
	}
	if user.FirstName != "" {
		return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, userID, htmlEscape(user.FirstName))
	}
	return fmt.Sprintf(`<a href="tg://user?id=%d">user %d</a>`, userID, userID)
}
