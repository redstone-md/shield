package events

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/hashicorp/go-multierror"

	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/observability"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/spamcheck"
)

const manualWarnSignalSource = "manual_warn"

func (a *admin) DirectSpamReport(ctx context.Context, update tbapi.Update) error {
	return a.directReport(ctx, update, true)
}

func (a *admin) DirectBanReport(ctx context.Context, update tbapi.Update) error {
	return a.directReport(ctx, update, false)
}

func (a *admin) DirectBanTarget(ctx context.Context, update tbapi.Update, target string) error {
	userID, userName, err := a.resolveUserTarget(ctx, target, "ban")
	if err != nil {
		return err
	}
	if a.superUsers.IsSuper(userName, userID) {
		return fmt.Errorf("ban target is super-user %s (%d), ignored", userName, userID)
	}

	if err := a.bot.RemoveApprovedUser(userID); err != nil {
		log.Printf("[DEBUG] can't remove user %d from approved list: %v", userID, err)
	}

	msg := fmt.Sprintf("пользователь %s забанен администратором %s",
		markdownBanTarget(userName, userID),
		markdownUserLink(update.Message.From.UserName, update.Message.From.ID))
	if err := send(tbapi.NewMessage(a.adminChatID, msg), a.tbAPI); err != nil {
		return fmt.Errorf("failed to send direct ban notification: %w", err)
	}

	if err := a.deleteMessage(ctx, update.Message.Chat.ID, update.Message.MessageID, "admin ban command"); err != nil {
		return fmt.Errorf("direct ban target failed: %w", err)
	}

	banReq := banRequest{duration: bot.PermanentBanDuration, userID: userID,
		tbAPI: a.tbAPI, dry: a.dry, training: a.trainingMode, userName: userName}
	if err := a.banInAllChats(ctx, banReq); err != nil {
		return fmt.Errorf("failed to ban user %d: %w", userID, err)
	}
	return nil
}

func markdownBanTarget(userName string, userID int64) string {
	link := markdownUserLink(userName, userID)
	if strings.TrimSpace(userName) == "" {
		return link
	}
	return fmt.Sprintf("%s (%d)", link, userID)
}

func (a *admin) resolveBanTarget(ctx context.Context, target string) (userID int64, userName string, err error) {
	return a.resolveUserTarget(ctx, target, "ban")
}

func (a *admin) resolveUserTarget(ctx context.Context, target, label string) (userID int64, userName string, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return 0, "", fmt.Errorf("%s target is empty", label)
	}
	if id, parseErr := strconv.ParseInt(target, 10, 64); parseErr == nil {
		if a.locator != nil {
			userName = a.locator.UserNameByID(ctx, id)
		}
		return id, userName, nil
	}

	userName = strings.TrimPrefix(target, "@")
	if userName == "" {
		return 0, "", fmt.Errorf("%s target username is empty", label)
	}
	if a.locator == nil {
		return 0, userName, fmt.Errorf("can't resolve username %q: locator is not configured", userName)
	}
	userID = a.locator.UserIDByName(ctx, userName)
	if userID == 0 {
		return 0, userName, fmt.Errorf("can't resolve username %q to user id", userName)
	}
	return userID, userName, nil
}

func (a *admin) DirectDeleteReply(ctx context.Context, update tbapi.Update) error {
	if update.Message == nil || update.Message.ReplyToMessage == nil {
		return fmt.Errorf("delete command requires a reply message")
	}
	chatID := update.Message.Chat.ID
	if update.Message.ReplyToMessage.Chat.ID != 0 {
		chatID = update.Message.ReplyToMessage.Chat.ID
	}
	if err := a.deleteMessage(ctx, chatID, update.Message.ReplyToMessage.MessageID, "reply target"); err != nil {
		return fmt.Errorf("direct delete reply failed: %w", err)
	}
	if err := a.deleteMessage(ctx, update.Message.Chat.ID, update.Message.MessageID, "delete command"); err != nil {
		return fmt.Errorf("direct delete reply failed: %w", err)
	}
	return nil
}

func (a *admin) DirectDeleteByID(ctx context.Context, update tbapi.Update, chatID int64, msgID int) error {
	if err := a.deleteMessage(ctx, chatID, msgID, "delete by id"); err != nil {
		return fmt.Errorf("direct delete by id failed: %w", err)
	}
	if err := a.deleteMessage(ctx, update.Message.Chat.ID, update.Message.MessageID, "delete command"); err != nil {
		return fmt.Errorf("direct delete by id failed: %w", err)
	}
	return nil
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

	if err := a.deleteWarnMessage(ctx, origMsg.Chat.ID, origMsg.MessageID, "warn"); err != nil {
		errs = multierror.Append(errs, err)
	}
	if err := a.deleteWarnMessage(ctx, update.Message.Chat.ID, update.Message.MessageID, "admin warn report"); err != nil {
		errs = multierror.Append(errs, err)
	}

	origBotMsg := transform(origMsg)
	subjectID := warnSubjectID(origMsg)
	strikeCount := a.warnStrikeCount(ctx, subjectID)
	duration, restrict, warn := spamPenalty(strikeCount, a.softBan, a.moderation)
	if !warn {
		if err := a.applyWarnEscalation(ctx, origMsg, duration, restrict); err != nil {
			errs = multierror.Append(errs, err)
		}
		if err := errs.ErrorOrNil(); err != nil {
			return fmt.Errorf("direct warn report failed: %w", err)
		}
		return nil
	}

	warnNum := strikeCount + 1
	warnMsg := buildWarningText(warnNum, a.moderation.WarnStrikes, warnDisplayUser(origMsg), subjectID, a.warnMsg, "")
	if err := a.sendWarnMessage(ctx, origMsg, warnMsg); err != nil {
		errs = multierror.Append(errs, err)
	} else {
		a.recordManualWarn(ctx, origBotMsg, subjectID, warnNum)
	}

	if err := errs.ErrorOrNil(); err != nil {
		return fmt.Errorf("direct warn report failed: %w", err)
	}
	return nil
}

func (a *admin) DirectUnwarnReport(update tbapi.Update) error {
	origMsg := update.Message.ReplyToMessage
	ctx := a.warnContext(update, origMsg)
	if err := a.deleteWarnMessage(ctx, update.Message.Chat.ID, update.Message.MessageID, "admin unwarn report"); err != nil {
		return fmt.Errorf("direct unwarn report failed: %w", err)
	}
	if a.detectedSpam == nil {
		return fmt.Errorf("direct unwarn report failed: detected spam storage is not configured")
	}

	subjectID, userName := unwarnSubject(origMsg)
	if subjectID == 0 && userName == "" {
		return fmt.Errorf("direct unwarn report failed: can't identify warning subject")
	}

	remaining, deleted, err := a.deleteLatestWarn(ctx, subjectID, userName)
	if err != nil {
		return fmt.Errorf("direct unwarn report failed: %w", err)
	}

	text := unwarnResultText(subjectID, userName, remaining, deleted)
	if err := send(tbapi.NewMessage(a.adminChatID, text), a.tbAPI); err != nil {
		return fmt.Errorf("direct unwarn report failed: %w", err)
	}
	return nil
}

func (a *admin) deleteAllWarns(ctx context.Context, subjectID int64, userName string) error {
	if subjectID != 0 {
		manualCount, err := a.detectedSpam.CountByUserIDAndSignalSource(ctx, subjectID, manualWarnSignalSource)
		if err != nil {
			return err
		}
		for range manualCount {
			_, deleteErr := a.detectedSpam.DeleteLatestByUserIDAndSignalSource(ctx, subjectID, manualWarnSignalSource)
			if deleteErr != nil {
				return deleteErr
			}
		}

		remaining, err := a.detectedSpam.CountByUserID(ctx, subjectID)
		if err != nil {
			return err
		}
		for range remaining {
			_, deleteErr := a.detectedSpam.DeleteLatestByUserID(ctx, subjectID)
			if deleteErr != nil {
				return deleteErr
			}
		}
		return nil
	}
	manualCount, err := a.detectedSpam.CountByUserNameAndSignalSource(ctx, userName, manualWarnSignalSource)
	if err != nil {
		return err
	}
	for range manualCount {
		_, deleteErr := a.detectedSpam.DeleteLatestByUserNameAndSignalSource(ctx, userName, manualWarnSignalSource)
		if deleteErr != nil {
			return deleteErr
		}
	}
	return nil
}

func (a *admin) deleteLatestWarn(ctx context.Context, subjectID int64, userName string) (remaining int, deleted bool, err error) {
	if subjectID != 0 {
		deleted, err = a.detectedSpam.DeleteLatestByUserIDAndSignalSource(ctx, subjectID, manualWarnSignalSource)
		if err != nil {
			return 0, false, err
		}
		if !deleted {
			deleted, err = a.detectedSpam.DeleteLatestByUserID(ctx, subjectID)
			if err != nil {
				return 0, false, err
			}
		}
		remaining, err = a.detectedSpam.CountByUserID(ctx, subjectID)
		return remaining, deleted, err
	}
	deleted, err = a.detectedSpam.DeleteLatestByUserNameAndSignalSource(ctx, userName, manualWarnSignalSource)
	if err != nil {
		return 0, false, err
	}
	remaining, err = a.detectedSpam.CountByUserNameAndSignalSource(ctx, userName, manualWarnSignalSource)
	return remaining, deleted, err
}

func unwarnResultText(subjectID int64, userName string, remaining int, deleted bool) string {
	subject := userName
	if subject == "" {
		subject = fmt.Sprintf("user %d", subjectID)
	}
	if subjectID != 0 && userName != "" {
		subject = fmt.Sprintf("%s (%d)", userName, subjectID)
	}
	if !deleted {
		return fmt.Sprintf("Предупреждений для %s не найдено", subject)
	}
	return fmt.Sprintf("Предупреждение снято: %s, осталось %d", subject, remaining)
}

func unwarnSubject(msg *tbapi.Message) (subjectID int64, userName string) {
	if msg == nil {
		return 0, ""
	}
	if id, userName, ok := unwarnSubjectFromText(msg.Text); ok {
		return id, userName
	}
	if msg.SenderChat != nil && msg.SenderChat.ID != 0 {
		return msg.SenderChat.ID, msg.SenderChat.UserName
	}
	if msg.From != nil {
		return msg.From.ID, msg.From.UserName
	}
	return 0, ""
}

func unwarnSubjectFromText(text string) (subjectID int64, userName string, ok bool) {
	for _, expr := range []string{`tg://user\?id=(-?\d+)`, `\((-?\d+)\)`, `user (-?\d+)`} {
		re := regexp.MustCompile(expr)
		if match := re.FindStringSubmatch(text); len(match) > 1 {
			id, err := strconv.ParseInt(match[1], 10, 64)
			if err == nil {
				return id, "", true
			}
		}
	}
	re := regexp.MustCompile(`https://t\.me/([A-Za-z0-9_]+)`)
	if match := re.FindStringSubmatch(text); len(match) > 1 {
		return 0, match[1], true
	}
	return 0, "", false
}

func warnDisplayUser(msg *tbapi.Message) bot.User {
	if msg != nil && msg.SenderChat != nil && msg.SenderChat.ID != 0 {
		return bot.User{ID: msg.SenderChat.ID, Username: msg.SenderChat.UserName, FirstName: msg.SenderChat.Title}
	}
	if msg == nil || msg.From == nil {
		return bot.User{}
	}
	return bot.User{ID: msg.From.ID, Username: msg.From.UserName, FirstName: msg.From.FirstName}
}

func (a *admin) warnStrikeCount(ctx context.Context, subjectID int64) int {
	if a.detectedSpam == nil || subjectID == 0 {
		return 0
	}
	count, err := a.detectedSpam.CountByUserIDAndSignalSource(ctx, subjectID, manualWarnSignalSource)
	if err != nil {
		log.Printf("[WARN] failed to count warning strikes for user %d: %v", subjectID, err)
		return 0
	}
	return count
}

func (a *admin) applyWarnEscalation(ctx context.Context, origMsg *tbapi.Message, duration time.Duration, restrict bool) error {
	req := banRequest{
		duration: duration,
		userID:   origMsg.From.ID,
		chatID:   a.chatIDOrFallback(origMsg.Chat.ID),
		dry:      a.dry,
		training: a.trainingMode,
		userName: origMsg.From.UserName,
		restrict: restrict,
	}
	if origMsg.SenderChat != nil && origMsg.SenderChat.ID != 0 {
		req.channelID = origMsg.SenderChat.ID
		req.userName = origMsg.SenderChat.UserName
	}
	if a.actions != nil {
		if err := a.actions.ApplyBan(ctx, req); err != nil {
			return fmt.Errorf("failed to escalate warning for %d: %w", warnSubjectID(origMsg), err)
		}
		return nil
	}
	req.tbAPI = a.tbAPI
	if err := banUserOrChannel(ctx, req); err != nil {
		return fmt.Errorf("failed to escalate warning for %d: %w", warnSubjectID(origMsg), err)
	}
	return nil
}

func (a *admin) recordManualWarn(ctx context.Context, msg *bot.Message, subjectID int64, warnNum int) {
	if a.detectedSpam == nil || msg == nil || subjectID == 0 {
		return
	}
	checks := []spamcheck.Response{{
		Name:    manualWarnSignalSource,
		Spam:    true,
		Details: fmt.Sprintf("ручное предупреждение %d/%d", warnNum, a.moderation.WarnStrikes),
	}}
	userName := msg.From.Username
	if msg.SenderChat.UserName != "" {
		userName = msg.SenderChat.UserName
	}
	entry := storage.DetectedSpamInfo{
		GID:            fmt.Sprint(a.firstChatID()),
		Text:           msg.Text,
		UserID:         subjectID,
		UserName:       userName,
		Timestamp:      time.Now().UTC(),
		SignalSource:   manualWarnSignalSource,
		Score:          1,
		RuleSetVersion: 0,
	}
	if err := a.detectedSpam.Write(ctx, entry, checks); err != nil {
		log.Printf("[WARN] failed to record manual warning for user %d: %v", subjectID, err)
	}
}

func (a *admin) deleteWarnMessage(ctx context.Context, chatID int64, msgID int, label string) error {
	return a.deleteMessage(ctx, a.chatIDOrFallback(chatID), msgID, label)
}

func (a *admin) deleteMessage(ctx context.Context, chatID int64, msgID int, label string) error {
	if a.actions != nil {
		if err := a.actions.DeleteMessage(ctx, chatID, msgID); err != nil {
			return fmt.Errorf("failed to delete message %d: %w", msgID, err)
		}
		log.Printf("[INFO] %s message %d deleted", label, msgID)
		return nil
	}

	_, err := a.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID:  msgID,
		ChatConfig: tbapi.ChatConfig{ChatID: chatID},
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
			chatID:      a.chatIDOrFallback(origMsg.Chat.ID),
			subjectID:   warnSubjectID(origMsg),
			messageID:   origMsg.MessageID,
			text:        warnMsg,
			warnDelTime: a.warnDeleteDuration,
		}); err != nil {
			return fmt.Errorf("failed to send warning to main chat: %w", err)
		}
		return nil
	}

	if err := send(tbapi.NewMessage(a.chatIDOrFallback(origMsg.Chat.ID), escapeMarkDownV1Text(warnMsg)), a.tbAPI); err != nil {
		return fmt.Errorf("failed to send warning to main chat: %w", err)
	}
	return nil
}

func (a *admin) warnContext(update tbapi.Update, origMsg *tbapi.Message) context.Context {
	eventID := fmt.Sprintf("warn-%d-%d", a.chatIDOrFallback(origMsg.Chat.ID), origMsg.MessageID)
	correlationID := fmt.Sprintf("corr-warn-%d", origMsg.MessageID)
	origChat := a.chatIDOrFallback(origMsg.Chat.ID)
	idempotencyKey := fmt.Sprintf("warn:chat:%d:msg:%d:cmd:%d",
		origChat, origMsg.MessageID, update.Message.MessageID)
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

func (a *admin) directReport(ctx context.Context, update tbapi.Update, updateSamples bool) error {
	logFrom := update.Message.ReplyToMessage.From.UserName
	logID := update.Message.ReplyToMessage.From.ID
	if sc := update.Message.ReplyToMessage.SenderChat; sc != nil && sc.ID != 0 {
		logFrom = a.channelDisplayName(sc)
		logID = sc.ID
	}
	log.Printf("[DEBUG] direct ban by admin %q: msg id: %d, from: %q (%d)",
		update.Message.From.UserName, update.Message.ReplyToMessage.MessageID, logFrom, logID)

	origMsg := update.Message.ReplyToMessage

	if from := origMsg.From; from != nil {
		log.Printf("[DEBUG] reply src: chat=%d From.ID=%d From.UserName=%q From.IsBot=%t SenderChat=%+v ViaBot=%+v",
			origMsg.Chat.ID, from.ID, from.UserName, from.IsBot, origMsg.SenderChat, origMsg.ViaBot)
	}

	msgTxt := origMsg.Text
	if msgTxt == "" {
		m := transform(origMsg)
		msgTxt = m.Text
	}
	if origMsg.Quote != nil && origMsg.Quote.Text != "" {
		msgTxt = msgTxt + "\n" + origMsg.Quote.Text
	}
	log.Printf("[DEBUG] reported spam message from superuser %q (%d): %q",
		update.Message.From.UserName, update.Message.From.ID, msgTxt)

	if origMsg.From.UserName != "" && a.superUsers.IsSuper(origMsg.From.UserName, origMsg.From.ID) {
		return fmt.Errorf("banned message is from super-user %s (%d), ignored", origMsg.From.UserName, origMsg.From.ID)
	}

	errs := new(multierror.Error)

	var channelID int64
	if origMsg.SenderChat != nil && origMsg.SenderChat.ID != 0 && origMsg.SenderChat.ID != a.chatIDOrFallback(origMsg.Chat.ID) {
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
	spamInfoText := "**не удалось получить диагностику спама**"
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
	newMsgText := fmt.Sprintf("**исходная диагностика для %s (%d)**\n\n%s\n\n%s\n\n\n"+
		"пользователь забанен администратором %s, сообщение удалено",
		escapeMarkDownV1Text(displayName), displayID, msgTxt, escapeMarkDownV1Text(spamInfoText),
		markdownUserLink(update.Message.From.UserName, update.Message.From.ID))
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
		if a.autoLearner != nil {
			a.autoLearner.LearnSpam(ctx, msgTxt, update.Message.From.UserName)
		}
	}

	_, err := a.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID:  origMsg.MessageID,
		ChatConfig: tbapi.ChatConfig{ChatID: a.chatIDOrFallback(origMsg.Chat.ID)},
	}})
	if err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to delete message %d: %w", origMsg.MessageID, err))
	} else {
		log.Printf("[INFO] spam message %d deleted", origMsg.MessageID)
	}

	_, err = a.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID:  update.Message.MessageID,
		ChatConfig: tbapi.ChatConfig{ChatID: update.Message.Chat.ID},
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

	if origMsg.SenderChat != nil && origMsg.SenderChat.ID == a.chatIDOrFallback(origMsg.Chat.ID) {
		log.Printf("[WARN] skipping ban for anonymous admin post, sender chat %d matches group chat",
			a.chatIDOrFallback(origMsg.Chat.ID))
	} else {
		banReq := banRequest{duration: bot.PermanentBanDuration, userID: origMsg.From.ID, channelID: channelID,
			tbAPI: a.tbAPI, dry: a.dry, training: a.trainingMode, userName: username}

		if err := a.banInAllChats(ctx, banReq); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to ban user %d: %w", origMsg.From.ID, err))
		}
	}

	cleanupUserID := origMsg.From.ID
	if channelID != 0 {
		cleanupUserID = channelID
	}
	if a.aggressiveCleanup && !a.dry && (origMsg.SenderChat == nil || origMsg.SenderChat.ID != a.chatIDOrFallback(origMsg.Chat.ID)) {
		go func() {
			cleanupCtx := context.WithoutCancel(ctx)
			deleted, err := a.deleteUserMessages(cleanupCtx, cleanupUserID)
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
				notifyMsg := fmt.Sprintf("_удалено %d сообщений спамера %q (%d)_",
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
