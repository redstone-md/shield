package events

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/hashicorp/go-multierror"

	"github.com/umputun/tg-spam/app/bot"
)

type admin struct {
	tbAPI                  TbAPI
	bot                    Bot
	locator                Locator
	superUsers             SuperUsers
	actions                ActionExecutor
	autoLearner            AutoLearner
	detectedSpam           DetectedSpamCounter
	primChatID             int64
	adminChatID            int64
	trainingMode           bool
	softBan                bool
	dry                    bool
	warnMsg                string
	moderation             ModerationConfig
	warnDeleteDuration     time.Duration
	aggressiveCleanup      bool
	aggressiveCleanupLimit int
}

const (
	confirmationPrefix = "?"
	banPrefix          = "+"
	infoPrefix         = "!"
)

func (a *admin) ReportBan(banUserStr string, msg *bot.Message, duration time.Duration, restrict bool, reasons ...string) {
	log.Printf("[DEBUG] report to admin chat, ban msgsData for %s, group: %d", banUserStr, a.adminChatID)
	msgText := msg.Text
	if msg.Quote != "" {
		msgText = msg.Text + "\n" + msg.Quote
	}
	if msgText == "" {
		switch {
		case msg.WithSticker && msg.Sticker != nil:
			parts := []string{"[sticker"}
			if msg.Sticker.SetName != "" {
				parts = append(parts, "from "+msg.Sticker.SetName)
			}
			if msg.Sticker.IsAnimated {
				parts = append(parts, "(animated)")
			}
			if msg.Sticker.IsVideo {
				parts = append(parts, "(video)")
			}
			parts = append(parts, "]")
			msgText = strings.Join(parts, " ")
		case msg.Image != nil:
			msgText = "[image]"
		case msg.WithVideo:
			msgText = "[video]"
		}
	}
	text := strings.ReplaceAll(htmlEscape(msgText), "\n", " ")
	callbackUser := msg.From
	if msg.SenderChat.ID != 0 {
		callbackUser = bot.User{ID: msg.SenderChat.ID, Username: msg.SenderChat.UserName}
	}

	actionText := moderationActionText(duration, restrict, a.dry)
	userID := msg.From.ID
	username := msg.From.Username
	if msg.SenderChat.ID != 0 {
		userID = msg.SenderChat.ID
		username = msg.SenderChat.UserName
	}

	banLine := ""
	if username != "" {
		banLine = fmt.Sprintf(`<b>%s</b> <a href="https://t.me/%s">%s</a>`,
			htmlEscape(actionText), username, htmlEscape(adminDisplayName(msg.From)))
	} else if msg.From.FirstName != "" {
		banLine = fmt.Sprintf(`<b>%s</b> %s (%d)`, htmlEscape(actionText), htmlEscape(msg.From.FirstName), userID)
	} else {
		banLine = fmt.Sprintf(`<b>%s</b> user %d`, htmlEscape(actionText), userID)
	}
	forwardMsg := fmt.Sprintf("%s\n\n%s\n\n", banLine, text)
	forwardMsg = appendReasonHTML(forwardMsg, firstNotificationReason(reasons))
	if err := a.sendWithUnbanMarkup(forwardMsg, "change ban", callbackUser, msg.ID, a.adminChatID); err != nil {
		log.Printf("[WARN] failed to send admin message, %v", err)
	}
}

func (a *admin) ReportWarn(warnUserStr string, msg *bot.Message, warnNum, warnTotal int, reasons ...string) {
	log.Printf("[DEBUG] report to admin chat, warn for %s, strike %d/%d", warnUserStr, warnNum, warnTotal)
	msgText := msg.Text
	if msgText == "" {
		if msg.WithSticker {
			msgText = "[sticker]"
		} else if msg.Image != nil {
			msgText = "[image]"
		} else if msg.WithVideo {
			msgText = "[video]"
		}
	}
	text := strings.ReplaceAll(htmlEscape(msgText), "\n", " ")

	userID := msg.From.ID
	username := msg.From.Username
	if msg.SenderChat.ID != 0 {
		userID = msg.SenderChat.ID
		username = msg.SenderChat.UserName
	}
	hasUsername := username != ""

	var warnLine string
	if hasUsername {
		warnLine = fmt.Sprintf(`<b>⚠️ WARNING %d/%d</b> <a href="https://t.me/%s">%s</a> (%d)`,
			warnNum, warnTotal, username, htmlEscape(adminDisplayName(msg.From)), userID)
	} else if msg.From.FirstName != "" {
		warnLine = fmt.Sprintf(`<b>⚠️ WARNING %d/%d</b> %s (%d)`,
			warnNum, warnTotal, htmlEscape(msg.From.FirstName), userID)
	} else {
		warnLine = fmt.Sprintf(`<b>⚠️ WARNING %d/%d</b> user %d`,
			warnNum, warnTotal, userID)
	}

	forwardMsg := fmt.Sprintf("%s\n\n%s\n\n", warnLine, text)
	forwardMsg = appendReasonHTML(forwardMsg, firstNotificationReason(reasons))
	msgConfig := tbapi.NewMessage(a.adminChatID, forwardMsg)
	msgConfig.ParseMode = tbapi.ModeHTML
	msgConfig.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	if _, err := a.tbAPI.Send(msgConfig); err != nil {
		log.Printf("[WARN] failed to send admin warn message, %v", err)
	}
}

func adminDisplayName(user bot.User) string {
	if user.FirstName != "" {
		return user.FirstName
	}
	if user.Username != "" {
		return user.Username
	}
	return fmt.Sprintf("user %d", user.ID)
}

func (a *admin) MsgHandler(ctx context.Context, update tbapi.Update) error {
	shrink := func(inp string, maxLen int) string {
		if utf8.RuneCountInString(inp) <= maxLen {
			return inp
		}
		return string([]rune(inp)[:maxLen]) + "..."
	}

	fwdID, username := a.getForwardUsernameAndID(update)

	log.Printf("[DEBUG] message from admin chat: msg id: %d, update id: %d, from: %s, sender: %q (%d)",
		update.Message.MessageID, update.UpdateID, update.Message.From.UserName,
		username, fwdID)

	if username == "" && update.Message.ForwardOrigin == nil {
		return a.demoCheck(update)
	}

	msgTxt := update.Message.Text
	if msgTxt == "" {
		m := transform(update.Message)
		msgTxt = m.Text
	}

	if msgTxt == "" {
		return errors.New("empty message text")
	}

	log.Printf("[DEBUG] forwarded message from superuser %q (%d) to admin chat %d: %q",
		update.Message.From.UserName, update.Message.From.ID, a.adminChatID, msgTxt)

	info, ok := a.locator.Message(ctx, msgTxt)
	if !ok {
		if fwdID != 0 {
			return a.msgHandlerFallback(ctx, update, fwdID, username, msgTxt)
		}
		return fmt.Errorf("not found %q in locator", shrink(msgTxt, 50))
	}

	log.Printf("[DEBUG] locator found message %s", info)
	errs := new(multierror.Error)

	if a.superUsers.IsSuper(info.UserName, info.UserID) {
		return fmt.Errorf("forwarded message is about super-user %s (%d), ignored", info.UserName, info.UserID)
	}

	if err := a.bot.RemoveApprovedUser(info.UserID); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to remove user %d from approved list: %w", info.UserID, err))
	}

	spamInfo := []string{}
	resp := a.bot.OnMessage(bot.Message{Text: update.Message.Text, From: bot.User{ID: info.UserID}}, true)
	spamInfoText := "**не удалось получить диагностику спама**"
	for _, check := range resp.CheckResults {
		spamInfo = append(spamInfo, "- "+escapeMarkDownV1Text(check.String()))
	}
	if len(spamInfo) > 0 {
		spamInfoText = strings.Join(spamInfo, "\n")
	}
	newMsgText := fmt.Sprintf("**исходная диагностика для %q (%d)**\n\n%s\n\n\n*пользователь забанен, сообщение удалено*",
		escapeMarkDownV1Text(info.UserName), info.UserID, spamInfoText)
	if err := send(tbapi.NewMessage(a.adminChatID, newMsgText), a.tbAPI); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to send spap detection results to admin chat: %w", err))
	}

	if a.dry {
		if err := errs.ErrorOrNil(); err != nil {
			return fmt.Errorf("dry run errors: %w", err)
		}
		return nil
	}

	if err := a.bot.UpdateSpam(msgTxt); err != nil {
		return fmt.Errorf("failed to update spam for %q: %w", msgTxt, err)
	}

	if a.autoLearner != nil && msgTxt != "" {
		a.autoLearner.LearnSpam(ctx, msgTxt, "admin_forward")
	}

	_, err := a.tbAPI.Request(tbapi.DeleteMessageConfig{
		BaseChatMessage: tbapi.BaseChatMessage{
			MessageID:  info.MsgID,
			ChatConfig: tbapi.ChatConfig{ChatID: a.primChatID},
		},
	})
	if err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to delete message %d: %w", info.MsgID, err))
	} else {
		log.Printf("[INFO] message %d deleted", info.MsgID)
	}

	if info.UserID == a.primChatID {
		log.Printf("[WARN] skipping ban in MsgHandler, user ID %d matches group chat", a.primChatID)
	} else {
		banReq := banRequest{duration: bot.PermanentBanDuration, userID: info.UserID,
			channelID: channelIDFromCallback(info.UserID),
			chatID:    a.primChatID, tbAPI: a.tbAPI, dry: a.dry, training: a.trainingMode, userName: username}
		if err := banUserOrChannel(ctx, banReq); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to ban user %d: %w", info.UserID, err))
		}
	}

	if err := errs.ErrorOrNil(); err != nil {
		return fmt.Errorf("spam notification failed: %w", err)
	}
	return nil
}

func (a *admin) demoCheck(update tbapi.Update) error {
	msg := transform(update.Message)
	if strings.TrimSpace(msg.Text) == "" && msg.Image == nil && !msg.WithVideoNote && !msg.WithVideo && !msg.WithSticker {
		return nil
	}

	resp := a.bot.OnMessage(*msg, true)
	status := "сообщение пройдет"
	if resp.Send && resp.BanInterval > 0 {
		status = "сообщение НЕ пройдет"
	}

	checks := make([]string, 0, len(resp.CheckResults))
	for _, check := range resp.CheckResults {
		checks = append(checks, "- "+escapeMarkDownV1Text(check.String()))
	}
	checksText := "проверки не вернули диагностических сигналов"
	if len(checks) > 0 {
		checksText = strings.Join(checks, "\n")
	}

	msgText := msg.Text
	if msgText == "" {
		msgText = adminDemoContentLabel(msg)
	}
	text := fmt.Sprintf("**демо-проверка**: %s\n\n%s\n\n%s",
		escapeMarkDownV1Text(status), escapeMarkDownV1Text(msgText), checksText)
	if err := send(tbapi.NewMessage(a.adminChatID, text), a.tbAPI); err != nil {
		return fmt.Errorf("failed to send demo check results to admin chat: %w", err)
	}
	return nil
}

func adminDemoContentLabel(msg *bot.Message) string {
	switch {
	case msg.WithSticker:
		return "[sticker]"
	case msg.Image != nil:
		return "[image]"
	case msg.WithVideo:
		return "[video]"
	case msg.WithVideoNote:
		return "[video note]"
	default:
		return "[message]"
	}
}

func (a *admin) msgHandlerFallback(ctx context.Context, update tbapi.Update, fwdID int64, username, msgTxt string) error {
	log.Printf("[INFO] locator fallback: forwarded user %q (%d), processing without locator data", username, fwdID)
	errs := new(multierror.Error)

	if a.superUsers.IsSuper(username, fwdID) {
		return fmt.Errorf("forwarded message is about super-user %s (%d), ignored", username, fwdID)
	}

	if err := a.bot.RemoveApprovedUser(fwdID); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to remove user %d from approved list: %w", fwdID, err))
	}

	spamInfo := []string{}
	resp := a.bot.OnMessage(bot.Message{Text: update.Message.Text, From: bot.User{ID: fwdID}}, true)
	spamInfoText := "**не удалось получить диагностику спама**"
	for _, check := range resp.CheckResults {
		spamInfo = append(spamInfo, "- "+escapeMarkDownV1Text(check.String()))
	}
	if len(spamInfo) > 0 {
		spamInfoText = strings.Join(spamInfo, "\n")
	}

	detectionMsg := fmt.Sprintf("**исходная диагностика для %q (%d)**\n\n%s\n\n\n*пользователь забанен*",
		escapeMarkDownV1Text(username), fwdID, spamInfoText)
	if err := send(tbapi.NewMessage(a.adminChatID, detectionMsg), a.tbAPI); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to send spam detection results to admin chat: %w", err))
	}

	if a.dry {
		warnMsg := fmt.Sprintf("⚠ *резервный режим locator* (dry mode): пользователь %q (%d), исходное сообщение нужно удалить вручную",
			escapeMarkDownV1Text(username), fwdID)
		if err := send(tbapi.NewMessage(a.adminChatID, warnMsg), a.tbAPI); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to send fallback warning: %w", err))
		}
		if err := errs.ErrorOrNil(); err != nil {
			return fmt.Errorf("dry run errors: %w", err)
		}
		return nil
	}

	if err := a.bot.UpdateSpam(msgTxt); err != nil {
		return fmt.Errorf("failed to update spam for %q: %w", msgTxt, err)
	}
	if a.autoLearner != nil && msgTxt != "" {
		a.autoLearner.LearnSpam(ctx, msgTxt, "admin_forward")
	}

	banReq := banRequest{duration: bot.PermanentBanDuration, userID: fwdID, chatID: a.primChatID,
		tbAPI: a.tbAPI, dry: a.dry, training: a.trainingMode, userName: username}
	if err := banUserOrChannel(ctx, banReq); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to ban user %d: %w", fwdID, err))
	}

	snippet := msgTxt
	if len([]rune(snippet)) > 100 {
		snippet = string([]rune(snippet)[:100]) + "..."
	}
	warnMsg := fmt.Sprintf("⚠ *резервный режим locator*: исходное сообщение от %q (%d) нужно удалить вручную\n\n_%s_",
		escapeMarkDownV1Text(username), fwdID, escapeMarkDownV1Text(snippet))
	if err := send(tbapi.NewMessage(a.adminChatID, warnMsg), a.tbAPI); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to send fallback warning: %w", err))
	}

	if err := errs.ErrorOrNil(); err != nil {
		return fmt.Errorf("spam notification failed: %w", err)
	}
	return nil
}

func (a *admin) getForwardUsernameAndID(update tbapi.Update) (fwdID int64, username string) {
	if update.Message.ForwardOrigin != nil {
		if update.Message.ForwardOrigin.IsUser() {
			return update.Message.ForwardOrigin.SenderUser.ID, update.Message.ForwardOrigin.SenderUser.UserName
		}
		if update.Message.ForwardOrigin.IsHiddenUser() {
			return 0, update.Message.ForwardOrigin.SenderUserName
		}
	}
	return 0, ""
}

func (a *admin) channelDisplayName(ch *tbapi.Chat) string {
	if ch == nil {
		return ""
	}
	if ch.UserName != "" {
		return ch.UserName
	}
	if ch.Title != "" {
		return ch.Title
	}
	return fmt.Sprintf("channel_%d", ch.ID)
}

func (a *admin) deleteUserMessages(ctx context.Context, userID int64) (deleted int, err error) {
	msgIDs, err := a.locator.GetUserMessageIDs(ctx, userID, a.aggressiveCleanupLimit)
	if err != nil {
		return 0, fmt.Errorf("failed to get user messages: %w", err)
	}

	rateLimiter := time.NewTicker(35 * time.Millisecond)
	defer rateLimiter.Stop()

	const maxConsecutiveFailures = 5
	consecutiveFailures := 0
	failed := 0

	for _, msgID := range msgIDs {
		<-rateLimiter.C

		if consecutiveFailures >= maxConsecutiveFailures {
			return deleted, fmt.Errorf("stopped after %d consecutive failures (deleted %d, failed %d)",
				maxConsecutiveFailures, deleted, failed)
		}

		_, err := a.tbAPI.Request(tbapi.DeleteMessageConfig{
			BaseChatMessage: tbapi.BaseChatMessage{
				MessageID:  msgID,
				ChatConfig: tbapi.ChatConfig{ChatID: a.primChatID},
			},
		})
		if err == nil {
			deleted++
			consecutiveFailures = 0
		} else {
			failed++
			consecutiveFailures++
		}
	}

	if failed > 0 {
		log.Printf("[INFO] aggressive cleanup completed: deleted %d messages, failed %d", deleted, failed)
	}
	return deleted, nil
}
