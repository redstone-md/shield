package events

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/observability"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/spamcheck"
)

const reportLLMContext = `This message was manually reported by a trusted chat member via reply command or bot mention after it passed normal filters. Review it strictly against the moderation rules. Prioritize crypto exchange offers, illegal or suspicious work, scam or fraud, external ad links, drug-related content, hate or ethnic abuse, emoji-spam, and duplicate ad campaigns only when the provided context indicates repetition. Normal profanity alone is allowed unless it targets participants.`

type reportOutcome string

const (
	reportOutcomeAccepted reportOutcome = "accepted"
	reportOutcomeReview   reportOutcome = "review"
	reportOutcomeBanned   reportOutcome = "banned"
)

type ReportConfig struct {
	Storage          Reports
	Enabled          bool
	Threshold        int
	AutoBanThreshold int
	RateLimit        int
	RatePeriod       time.Duration
}

type userReports struct {
	ReportConfig
	tbAPI        TbAPI
	bot          Bot
	locator      Locator
	detectedSpam DetectedSpamCounter
	tenantID     string
	superUsers   SuperUsers
	actions      ActionExecutor
	autoLearner  AutoLearner
	primChatIDs  []int64
	adminChatID  int64
	moderation   ModerationConfig
	trainingMode bool
	softBanMode  bool
	dry          bool
}

func (r *userReports) firstChatID() int64 {
	if len(r.primChatIDs) > 0 {
		return r.primChatIDs[0]
	}
	return 0
}

// chatIDOrFallback returns chatID if non-zero, otherwise the first primary chat ID.
func (r *userReports) chatIDOrFallback(chatID int64) int64 {
	if chatID != 0 {
		return chatID
	}
	return r.firstChatID()
}

func (r *userReports) DirectUserReport(ctx context.Context, update tbapi.Update) error {
	origMsg := update.Message.ReplyToMessage
	if origMsg == nil {
		return fmt.Errorf("must reply to a message to report it")
	}

	if origMsg.From == nil {
		log.Printf("[DEBUG] user report ignored: reported message from channel or anonymous admin")
		return fmt.Errorf("cannot report messages from channels or anonymous admins")
	}

	if origMsg.ForumTopicCreated != nil {
		log.Printf("[DEBUG] user report ignored: reported message is a forum topic creation message")
		return fmt.Errorf("cannot report forum topic creation messages")
	}

	log.Printf("[DEBUG] user report: msg id: %d, reporter: %q (%d), reported: %q (%d)",
		origMsg.MessageID,
		update.Message.From.UserName, update.Message.From.ID,
		origMsg.From.UserName, origMsg.From.ID)

	if r.superUsers.IsSuper(update.Message.From.UserName, update.Message.From.ID) {
		return fmt.Errorf("report from super-user %s (%d), use /spam instead", update.Message.From.UserName, update.Message.From.ID)
	}

	if r.superUsers.IsSuper(origMsg.From.UserName, origMsg.From.ID) {
		return fmt.Errorf("reported message is from super-user %s (%d), ignored", origMsg.From.UserName, origMsg.From.ID)
	}

	rateLimited, err := r.checkReportRateLimit(ctx, update.Message.From.ID)
	if err != nil {
		return fmt.Errorf("failed to check rate limit: %w", err)
	}
	if rateLimited {
		log.Printf("[INFO] reporter %d (%s) exceeded rate limit", update.Message.From.ID, update.Message.From.UserName)
		_, _ = r.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
			MessageID:  update.Message.MessageID,
			ChatConfig: tbapi.ChatConfig{ChatID: r.chatIDOrFallback(origMsg.Chat.ID)},
		}})
		return fmt.Errorf("rate limit exceeded for reporter %d", update.Message.From.ID)
	}

	_, err = r.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID:  update.Message.MessageID,
		ChatConfig: tbapi.ChatConfig{ChatID: r.chatIDOrFallback(origMsg.Chat.ID)},
	}})
	if err != nil {
		log.Printf("[WARN] failed to delete report message %d: %v", update.Message.MessageID, err)
	} else {
		log.Printf("[INFO] report message %d deleted", update.Message.MessageID)
	}

	msgTxt := origMsg.Text
	if msgTxt == "" {
		m := transform(origMsg)
		msgTxt = m.Text
	}
	if origMsg.Quote != nil && origMsg.Quote.Text != "" {
		msgTxt = msgTxt + "\n" + origMsg.Quote.Text
	}

	if handled, llmErr := r.tryLLMReportModeration(ctx, update, origMsg, msgTxt); handled {
		if llmErr == nil {
			duration, restrict := r.reportPenalty(ctx, origMsg.From.ID)
			r.notifyPrimaryChat(reportOutcomeBanned, duration, restrict, r.chatIDOrFallback(origMsg.Chat.ID))
		}
		return llmErr
	}

	if r.Storage == nil {
		return fmt.Errorf("reports storage not initialized")
	}

	report := storage.Report{
		MsgID:            origMsg.MessageID,
		ChatID:           r.chatIDOrFallback(origMsg.Chat.ID),
		ReporterUserID:   update.Message.From.ID,
		ReporterUserName: update.Message.From.UserName,
		ReportedUserID:   origMsg.From.ID,
		ReportedUserName: origMsg.From.UserName,
		MsgText:          msgTxt,
	}

	if addErr := r.Storage.Add(ctx, report); addErr != nil {
		return fmt.Errorf("failed to add report: %w", addErr)
	}

	outcome, err := r.checkReportThreshold(ctx, origMsg.MessageID, r.chatIDOrFallback(origMsg.Chat.ID))
	if err != nil {
		log.Printf("[WARN] failed to check report threshold: %v", err)
	} else {
		r.notifyPrimaryChat(outcome, 0, false, r.chatIDOrFallback(origMsg.Chat.ID))
	}

	return nil
}

func (r *userReports) notifyPrimaryChat(outcome reportOutcome, duration time.Duration, restrict bool, chatID int64) {
	if chatID == 0 {
		chatID = r.firstChatID()
	}
	if chatID == 0 {
		return
	}

	var text string
	switch outcome {
	case reportOutcomeBanned:
		text = reportStatusText(duration, restrict, r.dry)
	case reportOutcomeReview:
		text = "Репорт принят. Сообщение отправлено на проверку."
	case reportOutcomeAccepted:
		text = "Репорт принят."
	default:
		return
	}

	if err := send(tbapi.NewMessage(chatID, text), r.tbAPI); err != nil {
		log.Printf("[WARN] failed to send report status to primary chat: %v", err)
	}
}

func (r *userReports) tryLLMReportModeration(ctx context.Context, update tbapi.Update,
	origMsg *tbapi.Message, msgTxt string) (bool, error) {
	reviewMsg := transform(origMsg)
	reviewMsg.ForceLLM = true
	reviewMsg.LLMContext = reportLLMContext

	resp := r.bot.OnMessage(*reviewMsg, true)
	if !resp.Send {
		return false, nil
	}

	log.Printf("[INFO] LLM confirmed reported message %d from %s (%d) as spam",
		origMsg.MessageID, origMsg.From.UserName, origMsg.From.ID)
	return true, r.applyImmediateReportModeration(ctx, update, origMsg, msgTxt, resp)
}

func (r *userReports) applyImmediateReportModeration(ctx context.Context, update tbapi.Update,
	origMsg *tbapi.Message, msgTxt string, resp bot.Response) error {
	duration, restrict := r.reportPenalty(ctx, origMsg.From.ID)
	ctx = r.reportActionContext(ctx, "llm_auto", origMsg.MessageID, origMsg.From.ID)

	if err := r.bot.RemoveApprovedUser(origMsg.From.ID); err != nil {
		log.Printf("[DEBUG] can't remove user %d from approved list: %v", origMsg.From.ID, err)
	}

	if !r.dry && msgTxt != "" {
		if err := r.bot.UpdateSpam(msgTxt); err != nil {
			log.Printf("[WARN] failed to update spam samples from LLM-reviewed report: %v", err)
		}
		if r.autoLearner != nil {
			r.autoLearner.LearnSpam(ctx, msgTxt, "llm_auto")
		}
	}

	if r.actions != nil {
		if err := r.actions.DeleteMessage(ctx, r.chatIDOrFallback(origMsg.Chat.ID), origMsg.MessageID); err != nil {
			log.Printf("[WARN] failed to delete LLM-confirmed reported message %d: %v", origMsg.MessageID, err)
		}
	} else if !r.dry {
		_, err := r.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
			MessageID:  origMsg.MessageID,
			ChatConfig: tbapi.ChatConfig{ChatID: r.chatIDOrFallback(origMsg.Chat.ID)},
		}})
		if err != nil {
			log.Printf("[WARN] failed to delete LLM-confirmed reported message %d: %v", origMsg.MessageID, err)
		}
	}

	for _, banChatID := range r.primChatIDs {
		req := banRequest{
			duration: duration,
			userID:   origMsg.From.ID,
			chatID:   banChatID,
			dry:      r.dry,
			training: r.trainingMode,
			userName: origMsg.From.UserName,
			restrict: restrict,
		}
		if r.actions != nil {
			if err := r.actions.ApplyBan(ctx, req); err != nil {
				log.Printf("[WARN] failed to ban user %d in chat %d: %v", origMsg.From.ID, banChatID, err)
			}
		} else {
			req.tbAPI = r.tbAPI
			if err := banUserOrChannel(ctx, req); err != nil {
				log.Printf("[WARN] failed to ban user %d in chat %d: %v", origMsg.From.ID, banChatID, err)
			}
		}
	}

	r.recordDetectedSpam(ctx, origMsg, resp.CheckResults)

	if r.adminChatID != 0 {
		reporterName := update.Message.From.UserName
		if reporterName == "" {
			reporterName = fmt.Sprintf("user%d", update.Message.From.ID)
		}

		details := "LLM reviewed report and classified it as spam"
		for _, cr := range resp.CheckResults {
			if cr.Name == "openai" || cr.Name == "gemini" {
				details = cr.Details
				break
			}
		}

		notificationText := fmt.Sprintf(
			"**LLM auto-moderated reported message**\n\n[%s](tg://user?id=%d)\n\n%s\n\n"+
				"**Reporter:** [%s](tg://user?id=%d)\n**Reason:** %s",
			escapeMarkDownV1Text(origMsg.From.UserName),
			origMsg.From.ID,
			truncateString(strings.ReplaceAll(escapeMarkDownV1Text(msgTxt), "\n", " "), 300, "..."),
			escapeMarkDownV1Text(reporterName),
			update.Message.From.ID,
			escapeMarkDownV1Text(details),
		)
		if err := send(tbapi.NewMessage(r.adminChatID, notificationText), r.tbAPI); err != nil {
			log.Printf("[WARN] failed to send LLM auto-moderation notification: %v", err)
		}
	}

	return nil
}

func (r *userReports) reportPenalty(ctx context.Context, userID int64) (time.Duration, bool) {
	strikes := 1
	if r.detectedSpam != nil {
		count, err := r.detectedSpam.CountByUserID(ctx, userID)
		if err != nil {
			log.Printf("[WARN] failed to count spam strikes for reported user %d: %v", userID, err)
		} else {
			strikes = count + 1
		}
	}
	duration, restrict, _ := spamPenalty(strikes, r.softBanMode, r.moderation)
	return duration, restrict
}

func (r *userReports) recordDetectedSpam(ctx context.Context, origMsg *tbapi.Message, checks []spamcheck.Response) {
	if r.detectedSpam == nil {
		return
	}
	userID := origMsg.From.ID
	userName := origMsg.From.UserName
	if origMsg.SenderChat != nil && origMsg.SenderChat.ID != 0 {
		userID = origMsg.SenderChat.ID
		userName = origMsg.SenderChat.UserName
	}
	if userName == "" && origMsg.From != nil {
		userName = strings.TrimSpace(origMsg.From.FirstName + " " + origMsg.From.LastName)
	}
	text := strings.TrimSpace(strings.ReplaceAll(origMsg.Text, "\n", " "))
	if text == "" {
		text = strings.TrimSpace(strings.ReplaceAll(msgTextFromMessage(origMsg), "\n", " "))
	}
	rec := storage.DetectedSpamInfo{
		GID:       r.tenantID,
		Text:      text,
		UserID:    userID,
		UserName:  userName,
		Timestamp: time.Now().In(time.Local),
	}
	if err := r.detectedSpam.Write(ctx, rec, checks); err != nil {
		log.Printf("[WARN] failed to record detected spam for reported user %d: %v", userID, err)
	}
}

func msgTextFromMessage(msg *tbapi.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Text != "" {
		return msg.Text
	}
	if msg.Caption != "" {
		return msg.Caption
	}
	if msg.Quote != nil && msg.Quote.Text != "" {
		return msg.Quote.Text
	}
	return ""
}

func (r *userReports) checkReportRateLimit(ctx context.Context, reporterID int64) (bool, error) {
	if r.RateLimit <= 0 {
		return false, nil
	}
	if r.Storage == nil {
		return false, fmt.Errorf("reports storage not initialized")
	}

	since := time.Now().Add(-r.RatePeriod)
	count, err := r.Storage.GetReporterCountSince(ctx, reporterID, since)
	if err != nil {
		return false, fmt.Errorf("failed to get reporter count: %w", err)
	}

	if count >= r.RateLimit {
		log.Printf("[DEBUG] reporter %d exceeded rate limit: %d >= %d", reporterID, count, r.RateLimit)
		return true, nil
	}

	return false, nil
}

func (r *userReports) checkReportThreshold(ctx context.Context, msgID int, chatID int64) (reportOutcome, error) {
	if r.Storage == nil {
		return "", fmt.Errorf("reports storage not initialized")
	}

	reports, err := r.Storage.GetByMessage(ctx, msgID, chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get reports: %w", err)
	}

	reportCount := len(reports)

	if r.AutoBanThreshold > 0 && reportCount >= r.AutoBanThreshold {
		log.Printf("[INFO] auto-ban threshold reached for msgID:%d, chatID:%d: %d reports (threshold: %d)",
			msgID, chatID, reportCount, r.AutoBanThreshold)
		return reportOutcomeBanned, r.executeAutoBan(ctx, reports)
	}

	if reportCount < r.Threshold {
		log.Printf("[DEBUG] report threshold not reached for msgID:%d, chatID:%d: %d < %d",
			msgID, chatID, reportCount, r.Threshold)
		return reportOutcomeAccepted, nil
	}

	log.Printf("[INFO] report threshold reached for msgID:%d, chatID:%d: %d reports",
		msgID, chatID, reportCount)

	if len(reports) > 0 && reports[0].NotificationSent {
		log.Printf("[DEBUG] updating existing notification for msgID:%d, admin_msg_id:%d",
			msgID, reports[0].AdminMsgID)
		return reportOutcomeReview, r.updateReportNotification(ctx, reports)
	}

	log.Printf("[DEBUG] sending new notification for msgID:%d", msgID)
	return reportOutcomeReview, r.sendReportNotification(ctx, reports)
}

func (r *userReports) executeAutoBan(ctx context.Context, reports []storage.Report) error {
	if len(reports) == 0 {
		return fmt.Errorf("no reports provided")
	}

	firstReport := reports[0]
	msgID := firstReport.MsgID
	chatID := firstReport.ChatID
	reportedUserID := firstReport.ReportedUserID
	reportedUserName := firstReport.ReportedUserName
	msgText := firstReport.MsgText

	log.Printf("[INFO] executing auto-ban for user %d (%s) based on %d reports",
		reportedUserID, reportedUserName, len(reports))
	ctx = r.reportActionContext(ctx, "threshold_auto", msgID, reportedUserID)

	if remErr := r.bot.RemoveApprovedUser(reportedUserID); remErr != nil {
		log.Printf("[DEBUG] can't remove user %d from approved list: %v", reportedUserID, remErr)
	}

	if !r.dry && msgText != "" {
		if spamErr := r.bot.UpdateSpam(msgText); spamErr != nil {
			log.Printf("[WARN] failed to update spam samples: %v", spamErr)
		}
		if r.autoLearner != nil {
			r.autoLearner.LearnSpam(ctx, msgText, "auto_ban")
		}
	}

	if r.actions != nil {
		if err := r.actions.DeleteMessage(ctx, chatID, msgID); err != nil {
			log.Printf("[WARN] failed to delete reported message %d: %v", msgID, err)
		} else {
			log.Printf("[INFO] reported message %d auto-deleted", msgID)
		}
	} else if !r.dry {
		_, err := r.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
			MessageID:  msgID,
			ChatConfig: tbapi.ChatConfig{ChatID: chatID},
		}})
		if err != nil {
			log.Printf("[WARN] failed to delete reported message %d: %v", msgID, err)
		} else {
			log.Printf("[INFO] reported message %d auto-deleted", msgID)
		}
	}

	req := banRequest{
		duration: bot.PermanentBanDuration,
		userID:   reportedUserID,
		chatID:   chatID,
		dry:      r.dry,
		training: r.trainingMode,
		userName: reportedUserName,
		restrict: r.softBanMode,
	}
	if r.actions != nil {
		if err := r.actions.ApplyBan(ctx, req); err != nil {
			log.Printf("[WARN] failed to auto-ban user %d: %v", reportedUserID, err)
		}
	} else {
		req.tbAPI = r.tbAPI
		if err := banUserOrChannel(ctx, req); err != nil {
			log.Printf("[WARN] failed to auto-ban user %d: %v", reportedUserID, err)
		}
	}

	var notificationErr error
	if r.adminChatID != 0 {
		if len(reports) > 0 && reports[0].NotificationSent {
			notificationErr = r.updateNotificationForAutoBan(reports)
		} else {
			notificationErr = r.sendAutoBanNotification(reports)
		}

		if notificationErr != nil {
			log.Printf("[WARN] failed to send/update auto-ban notification: %v", notificationErr)
			return fmt.Errorf("auto-ban executed for user %d but notification failed: %w", reportedUserID, notificationErr)
		}
	}

	if err := r.Storage.DeleteByMessage(ctx, msgID, chatID); err != nil {
		log.Printf("[WARN] failed to delete reports for msgID:%d: %v", msgID, err)
	}

	log.Printf("[INFO] auto-ban executed for user %d by %d reports", reportedUserID, len(reports))
	return nil
}

func (r *userReports) reportActionContext(ctx context.Context, action string, msgID int, userID int64) context.Context {
	eventID := fmt.Sprintf("report-%s-%d-%d", action, r.firstChatID(), msgID)
	correlationID := fmt.Sprintf("corr-report-%s-%d", action, msgID)
	idempotencyKey := fmt.Sprintf("report:%s:chat:%d:msg:%d:user:%d", action, r.firstChatID(), msgID, userID)
	return observability.WithModerationMetadata(ctx, eventID, correlationID, idempotencyKey)
}
