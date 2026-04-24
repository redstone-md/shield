package events

import (
	"context"
	"fmt"
	"log"
	"strings"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/hashicorp/go-multierror"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/observability"
)

func (a *admin) DirectSpamReport(update tbapi.Update) error {
	return a.directReport(update, true)
}

func (a *admin) DirectBanReport(update tbapi.Update) error {
	return a.directReport(update, false)
}

func (a *admin) DirectWarnReport(update tbapi.Update) error {
	warnLogFrom := update.Message.ReplyToMessage.From.UserName
	warnLogID := update.Message.ReplyToMessage.From.ID
	if sc := update.Message.ReplyToMessage.SenderChat; sc != nil && sc.ID != 0 {
		warnLogFrom = a.channelDisplayName(sc)
		warnLogID = sc.ID
	}
	log.Printf("[DEBUG] direct warn by admin %q: msg id: %d, from: %q (%d)",
		update.Message.From.UserName, update.Message.ReplyToMessage.MessageID, warnLogFrom, warnLogID)
	origMsg := update.Message.ReplyToMessage

	msgTxt := origMsg.Text
	if msgTxt == "" {
		m := transform(origMsg)
		msgTxt = m.Text
	}
	log.Printf("[DEBUG] reported warn message from superuser %q (%d): %q",
		update.Message.From.UserName, update.Message.From.ID, msgTxt)
	if origMsg.From.UserName != "" && a.superUsers.IsSuper(origMsg.From.UserName, origMsg.From.ID) {
		return fmt.Errorf("warn message is from super-user %s (%d), ignored", origMsg.From.UserName, origMsg.From.ID)
	}
	errs := new(multierror.Error)
	ctx := a.warnContext(update, origMsg)

	if err := a.deleteWarnMessage(ctx, origMsg.MessageID, "warn"); err != nil {
		errs = multierror.Append(errs, err)
	}
	if err := a.deleteWarnMessage(ctx, update.Message.MessageID, "admin warn report"); err != nil {
		errs = multierror.Append(errs, err)
	}

	warnMsg := fmt.Sprintf("warning from %s\n\n%s %s", update.Message.From.UserName,
		a.warnTarget(origMsg), a.warnMsg)
	if err := a.sendWarnMessage(ctx, origMsg, warnMsg); err != nil {
		errs = multierror.Append(errs, err)
	}

	if err := errs.ErrorOrNil(); err != nil {
		return fmt.Errorf("direct warn report failed: %w", err)
	}
	return nil
}

func (a *admin) deleteWarnMessage(ctx context.Context, msgID int, label string) error {
	if a.actions != nil {
		if err := a.actions.DeleteMessage(ctx, a.primChatID, msgID); err != nil {
			return fmt.Errorf("failed to delete message %d: %w", msgID, err)
		}
		log.Printf("[INFO] %s message %d deleted", label, msgID)
		return nil
	}

	_, err := a.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID: msgID,
		ChatConfig: tbapi.ChatConfig{ChatID: a.primChatID},
	}})
	if err != nil {
		return fmt.Errorf("failed to delete message %d: %w", msgID, err)
	}
	log.Printf("[INFO] %s message %d deleted", label, msgID)
	return nil
}

func (a *admin) sendWarnMessage(ctx context.Context, origMsg *tbapi.Message, warnMsg string) error {
	if a.actions != nil {
		if err := a.actions.WarnUser(ctx, warnRequest{
			chatID:    a.primChatID,
			subjectID: warnSubjectID(origMsg),
			messageID: origMsg.MessageID,
			text:      warnMsg,
		}); err != nil {
			return fmt.Errorf("failed to send warning to main chat: %w", err)
		}
		return nil
	}

	if err := send(tbapi.NewMessage(a.primChatID, escapeMarkDownV1Text(warnMsg)), a.tbAPI); err != nil {
		return fmt.Errorf("failed to send warning to main chat: %w", err)
	}
	return nil
}

func (a *admin) warnTarget(origMsg *tbapi.Message) string {
	warnTarget := "@" + origMsg.From.UserName
	if origMsg.SenderChat != nil && origMsg.SenderChat.ID != 0 && origMsg.SenderChat.ID != a.primChatID {
		chName := a.channelDisplayName(origMsg.SenderChat)
		if origMsg.SenderChat.UserName != "" {
			return "@" + chName
		}
		return chName
	}
	return warnTarget
}

func (a *admin) warnContext(update tbapi.Update, origMsg *tbapi.Message) context.Context {
	eventID := fmt.Sprintf("warn-%d-%d", a.primChatID, origMsg.MessageID)
	correlationID := fmt.Sprintf("corr-warn-%d", origMsg.MessageID)
	idempotencyKey := fmt.Sprintf("warn:chat:%d:msg:%d:cmd:%d", a.primChatID, origMsg.MessageID, update.Message.MessageID)
	return observability.WithModerationMetadata(context.Background(), eventID, correlationID, idempotencyKey)
}

func warnSubjectID(msg *tbapi.Message) int64 {
	if msg.SenderChat != nil && msg.SenderChat.ID != 0 {
		return msg.SenderChat.ID
	}
	if msg.From != nil {
		return msg.From.ID
	}
	return 0
}

func (a *admin) directReport(update tbapi.Update, updateSamples bool) error {
	logFrom := update.Message.ReplyToMessage.From.UserName
	logID := update.Message.ReplyToMessage.From.ID
	if sc := update.Message.ReplyToMessage.SenderChat; sc != nil && sc.ID != 0 {
		logFrom = a.channelDisplayName(sc)
		logID = sc.ID
	}
	log.Printf("[DEBUG] direct ban by admin %q: msg id: %d, from: %q (%d)",
		update.Message.From.UserName, update.Message.ReplyToMessage.MessageID, logFrom, logID)

	origMsg := update.Message.ReplyToMessage

	msgTxt := origMsg.Text
	if msgTxt == "" {
		m := transform(origMsg)
		msgTxt = m.Text
	}
	if origMsg.Quote != nil && origMsg.Quote.Text != "" {
		msgTxt = msgTxt + "\n" + origMsg.Quote.Text
	}
	log.Printf("[DEBUG] reported spam message from superuser %q (%d): %q", update.Message.From.UserName, update.Message.From.ID, msgTxt)

	if origMsg.From.UserName != "" && a.superUsers.IsSuper(origMsg.From.UserName, origMsg.From.ID) {
		return fmt.Errorf("banned message is from super-user %s (%d), ignored", origMsg.From.UserName, origMsg.From.ID)
	}

	errs := new(multierror.Error)

	var channelID int64
	if origMsg.SenderChat != nil && origMsg.SenderChat.ID != 0 && origMsg.SenderChat.ID != a.primChatID {
		channelID = origMsg.SenderChat.ID
	}

	removeID := origMsg.From.ID
	if channelID != 0 {
		removeID = channelID
	}
	if err := a.bot.RemoveApprovedUser(removeID); err != nil {
		log.Printf("[DEBUG] can't remove user %d from approved list: %v", removeID, err)
	}

	spamInfo := []string{}
	diagMsg := bot.Message{Text: msgTxt, From: bot.User{ID: origMsg.From.ID}}
	if origMsg.SenderChat != nil && origMsg.SenderChat.ID != 0 {
		diagMsg.SenderChat = bot.SenderChat{ID: origMsg.SenderChat.ID, UserName: origMsg.SenderChat.UserName}
	}
	resp := a.bot.OnMessage(diagMsg, true)
	spamInfoText := "**can't get spam info**"
	for _, check := range resp.CheckResults {
		spamInfo = append(spamInfo, "- "+escapeMarkDownV1Text(check.String()))
	}
	if len(spamInfo) > 0 {
		spamInfoText = strings.Join(spamInfo, "\n")
	}
	displayName := origMsg.From.UserName
	displayID := origMsg.From.ID
	if channelID != 0 {
		displayName = a.channelDisplayName(origMsg.SenderChat)
		displayID = channelID
	}
	newMsgText := fmt.Sprintf("**original detection results for %s (%d)**\n\n%s\n\n%s\n\n\n"+
		"*the user banned by %q and message deleted*",
		escapeMarkDownV1Text(displayName), displayID, msgTxt, escapeMarkDownV1Text(spamInfoText),
		escapeMarkDownV1Text(update.Message.From.UserName))
	if err := send(tbapi.NewMessage(a.adminChatID, newMsgText), a.tbAPI); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to send spam detection results to admin chat: %w", err))
	}

	if a.dry {
		if err := errs.ErrorOrNil(); err != nil {
			return fmt.Errorf("dry run errors: %w", err)
		}
		return nil
	}

	if updateSamples && msgTxt != "" {
		if err := a.bot.UpdateSpam(msgTxt); err != nil {
			return fmt.Errorf("failed to update spam for %q: %w", msgTxt, err)
		}
	}

	_, err := a.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID: origMsg.MessageID,
		ChatConfig: tbapi.ChatConfig{ChatID: a.primChatID},
	}})
	if err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to delete message %d: %w", origMsg.MessageID, err))
	} else {
		log.Printf("[INFO] spam message %d deleted", origMsg.MessageID)
	}

	_, err = a.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID: update.Message.MessageID,
		ChatConfig: tbapi.ChatConfig{ChatID: a.primChatID},
	}})
	if err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to delete message %d: %w", update.Message.MessageID, err))
	} else {
		log.Printf("[INFO] admin spam reprot message %d deleted", update.Message.MessageID)
	}

	_, username := a.getForwardUsernameAndID(update)
	if username == "" && channelID != 0 && origMsg.SenderChat != nil {
		username = a.channelDisplayName(origMsg.SenderChat)
	}

	if origMsg.SenderChat != nil && origMsg.SenderChat.ID == a.primChatID {
		log.Printf("[WARN] skipping ban for anonymous admin post, sender chat %d matches group chat", a.primChatID)
	} else {
		banReq := banRequest{duration: bot.PermanentBanDuration, userID: origMsg.From.ID, channelID: channelID,
			chatID: a.primChatID, tbAPI: a.tbAPI, dry: a.dry, training: a.trainingMode, userName: username}

		if err := banUserOrChannel(context.Background(), banReq); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to ban user %d: %w", origMsg.From.ID, err))
		}
	}

	cleanupUserID := origMsg.From.ID
	if channelID != 0 {
		cleanupUserID = channelID
	}
	if a.aggressiveCleanup && !a.dry && (origMsg.SenderChat == nil || origMsg.SenderChat.ID != a.primChatID) {
		go func() {
			deleted, err := a.deleteUserMessages(cleanupUserID)
			if err != nil {
				log.Printf("[WARN] aggressive cleanup failed: %v", err)
				return
			}
			if deleted > 0 {
				cleanupName := origMsg.From.UserName
				if origMsg.SenderChat != nil && origMsg.SenderChat.UserName != "" {
					cleanupName = origMsg.SenderChat.UserName
				}
				log.Printf("[INFO] aggressive cleanup: deleted %d messages from %d", deleted, cleanupUserID)
				notifyMsg := fmt.Sprintf("_deleted %d messages from spammer %q (%d)_",
					deleted, escapeMarkDownV1Text(cleanupName), cleanupUserID)
				if err := send(tbapi.NewMessage(a.adminChatID, notifyMsg), a.tbAPI); err != nil {
					log.Printf("[WARN] failed to send deletion notification: %v", err)
				}
			}
		}()
	}

	if err := errs.ErrorOrNil(); err != nil {
		return fmt.Errorf("spam notification failed: %w", err)
	}
	return nil
}
