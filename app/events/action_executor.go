package events

import (
	"context"
	"fmt"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/redstone-md/shield/app/observability"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/spamcheck"
)

type ActionExecutor interface {
	ApplyBan(ctx context.Context, req banRequest) error
	DeleteMessage(ctx context.Context, chatID int64, msgID int) error
	ForwardMessage(ctx context.Context, fromChatID, toChatID int64, msgID int) error
	DeleteExtraMessages(ctx context.Context, checkResults []spamcheck.Response, userID int64, username string, chatID int64) error
	WarnUser(ctx context.Context, req warnRequest) error
	PostBanMessage(ctx context.Context, req banMessageRequest) error
}

type telegramActionExecutor struct {
	tbAPI        TbAPI
	dry          bool
	trainingMode bool
	superUsers   SuperUsers
	actions      ModerationActions
}

func newTelegramActionExecutor(tbAPI TbAPI, dry, trainingMode bool, superUsers SuperUsers,
	actions ModerationActions,
) telegramActionExecutor {
	return telegramActionExecutor{
		tbAPI:        tbAPI,
		dry:          dry,
		trainingMode: trainingMode,
		superUsers:   superUsers,
		actions:      actions,
	}
}

func (e telegramActionExecutor) ApplyBan(ctx context.Context, req banRequest) error {
	command := banCommand(req)
	subjectID := banSubjectID(req)
	attempt, replayed := e.replayAttempt(ctx, command, req.chatID, subjectID, 0)
	if replayed {
		return nil
	}

	req.tbAPI = e.tbAPI
	err := banUserOrChannel(ctx, req)
	e.recordAction(ctx, command, req.chatID, subjectID, 0, attempt, err)
	return err
}

func (e telegramActionExecutor) DeleteMessage(ctx context.Context, chatID int64, msgID int) error {
	attempt, replayed := e.replayAttempt(ctx, commandDeleteMessage, chatID, 0, msgID)
	if replayed {
		return nil
	}

	_, err := e.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID:  msgID,
		ChatConfig: tbapi.ChatConfig{ChatID: chatID},
	}})
	if err != nil {
		observability.Logf(ctx, "[WARN] failed to delete message %d: %v", msgID, err)
		e.recordAction(ctx, commandDeleteMessage, chatID, 0, msgID, attempt, err)
		return fmt.Errorf("delete message %d: %w", msgID, err)
	}
	observability.Logf(ctx, "[DEBUG] deleted message %d", msgID)
	e.recordAction(ctx, commandDeleteMessage, chatID, 0, msgID, attempt, nil)
	return nil
}

func (e telegramActionExecutor) ForwardMessage(ctx context.Context, fromChatID, toChatID int64, msgID int) error {
	_, err := e.tbAPI.Send(tbapi.NewForward(toChatID, fromChatID, msgID))
	if err != nil {
		observability.Logf(ctx, "[WARN] failed to forward message %d to admin chat %d: %v", msgID, toChatID, err)
		return fmt.Errorf("forward message %d: %w", msgID, err)
	}
	observability.Logf(ctx, "[DEBUG] forwarded message %d to admin chat %d", msgID, toChatID)
	return nil
}

func (e telegramActionExecutor) DeleteExtraMessages(ctx context.Context, checkResults []spamcheck.Response,
	userID int64, username string, chatID int64,
) error {
	if len(checkResults) == 0 || e.dry || e.trainingMode {
		return nil
	}

	if e.superUsers.IsSuper(username, userID) {
		observability.Logf(ctx, "[DEBUG] skip extra deletions for superuser %s (%d)", username, userID)
		return nil
	}

	for _, checkResult := range checkResults {
		if !checkResult.Spam || len(checkResult.ExtraDeleteIDs) == 0 {
			continue
		}

		observability.Logf(ctx, "[INFO] deleting %d extra messages from user %d", len(checkResult.ExtraDeleteIDs), userID)
		for _, msgID := range checkResult.ExtraDeleteIDs {
			time.Sleep(35 * time.Millisecond)
			if err := e.DeleteMessage(ctx, chatID, msgID); err != nil {
				observability.Logf(ctx, "[WARN] failed to delete extra message %d: %v", msgID, err)
			}
		}
	}
	return nil
}

func (e telegramActionExecutor) WarnUser(ctx context.Context, req warnRequest) error {
	attempt, replayed := e.replayAttempt(ctx, commandWarnUser, req.chatID, req.subjectID, req.messageID)
	if replayed {
		return nil
	}

	msgConfig := tbapi.NewMessage(req.chatID, req.text)
	msgConfig.ParseMode = tbapi.ModeHTML
	msgConfig.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	if kb, ok := appealKeyboard(req.botUsername, req.incidentID); ok {
		msgConfig.ReplyMarkup = kb
	}
	msg, err := e.tbAPI.Send(msgConfig)
	if err != nil {
		e.recordAction(ctx, commandWarnUser, req.chatID, req.subjectID, req.messageID, attempt, err)
		return fmt.Errorf("warn user %d: %w", req.subjectID, err)
	}

	e.recordAction(ctx, commandWarnUser, req.chatID, req.subjectID, req.messageID, attempt, nil)
	e.scheduleDelete(ctx, req.chatID, msg.MessageID, req.warnDelTime)
	return nil
}

// scheduleDelete deletes a posted message after delTime in a background
// goroutine. delTime <= 0 keeps the message.
func (e telegramActionExecutor) scheduleDelete(ctx context.Context, chatID int64, msgID int, delTime time.Duration) {
	if delTime <= 0 {
		return
	}
	go func() {
		observability.Logf(ctx, "[DEBUG] scheduled message %d deletion in %v", msgID, delTime)
		time.Sleep(delTime)
		observability.Logf(ctx, "[DEBUG] deleting scheduled message %d after %v", msgID, delTime)
		if err := e.DeleteMessage(context.Background(), chatID, msgID); err != nil {
			observability.Logf(ctx, "[WARN] failed to delete scheduled message %d: %v", msgID, err)
		} else {
			observability.Logf(ctx, "[DEBUG] scheduled message %d deleted successfully", msgID)
		}
	}()
}

// PostBanMessage posts a short ban notice to the group chat carrying the
// appeal button and schedules its deletion the same way a warning is deleted.
func (e telegramActionExecutor) PostBanMessage(ctx context.Context, req banMessageRequest) error {
	msgConfig := tbapi.NewMessage(req.chatID, req.text)
	msgConfig.ParseMode = tbapi.ModeHTML
	msgConfig.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	if kb, ok := appealKeyboard(req.botUsername, req.incidentID); ok {
		msgConfig.ReplyMarkup = kb
	}
	msg, err := e.tbAPI.Send(msgConfig)
	if err != nil {
		return fmt.Errorf("post ban message to chat %d: %w", req.chatID, err)
	}
	e.scheduleDelete(ctx, req.chatID, msg.MessageID, req.delTime)
	return nil
}

const (
	commandDeleteMessage  = "delete_message"
	commandMuteUser       = "mute_user"
	commandBanUser        = "ban_user"
	commandBanSenderChat  = "ban_sender_chat"
	commandWarnUser       = "warn_user"
	actionStatusCompleted = "completed"
	actionStatusFailed    = "failed"
)

func banCommand(req banRequest) string {
	switch {
	case req.channelID != 0:
		return commandBanSenderChat
	case req.restrict:
		return commandMuteUser
	default:
		return commandBanUser
	}
}

func banSubjectID(req banRequest) int64 {
	if req.channelID != 0 {
		return req.channelID
	}
	return req.userID
}

func (e telegramActionExecutor) replayAttempt(ctx context.Context, command string,
	chatID, subjectID int64, msgID int,
) (attempt int, replayed bool) {
	if e.actions == nil {
		return 1, false
	}

	meta, ok := observability.MetadataFromContext(ctx)
	if !ok || meta.IdempotencyKey == "" {
		return 1, false
	}

	replay, err := e.actions.Last(ctx, storage.ModerationActionLookup{
		IdempotencyKey: meta.IdempotencyKey,
		Command:        command,
		ChatID:         chatID,
		SubjectID:      subjectID,
		MessageID:      msgID,
	})
	if err != nil {
		observability.Logf(ctx, "[WARN] failed to load moderation action replay %s: %v", command, err)
		return 1, false
	}
	if replay.Completed {
		observability.Logf(ctx, "[INFO] skip replayed moderation action %s", command)
		return replay.Attempt, true
	}
	if replay.Found {
		return replay.Attempt + 1, false
	}
	return 1, false
}

func (e telegramActionExecutor) recordAction(ctx context.Context,
	command string, chatID, subjectID int64, msgID, attempt int, execErr error,
) {
	if e.actions == nil {
		return
	}
	meta, _ := observability.MetadataFromContext(ctx)
	entry := storage.ModerationActionEntry{
		EventID:        meta.EventID,
		CorrelationID:  meta.CorrelationID,
		IdempotencyKey: meta.IdempotencyKey,
		Command:        command,
		Status:         actionStatusCompleted,
		ChatID:         chatID,
		SubjectID:      subjectID,
		MessageID:      msgID,
		Attempt:        attempt,
	}
	if execErr != nil {
		entry.Status = actionStatusFailed
		entry.LastError = execErr.Error()
	}
	if err := e.actions.Add(ctx, entry); err != nil {
		observability.Logf(ctx, "[WARN] failed to record moderation action %s: %v", command, err)
	}
}

// appealKeyboard builds the single-button "Обжаловать" inline keyboard that
// deep-links the punished user to the bot DM with the incident id as payload.
// The second return value is false when there is no incident to appeal.
func appealKeyboard(botUsername string, incidentID int64) (tbapi.InlineKeyboardMarkup, bool) {
	if botUsername == "" || incidentID <= 0 {
		return tbapi.InlineKeyboardMarkup{}, false
	}
	url := fmt.Sprintf("https://t.me/%s?start=%d", botUsername, incidentID)
	return tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonURL("Обжаловать", url),
		),
	), true
}

type banRequest struct {
	tbAPI TbAPI

	userID    int64
	channelID int64
	chatID    int64
	duration  time.Duration
	userName  string

	dry      bool
	training bool
	restrict bool
}

type warnRequest struct {
	chatID      int64
	subjectID   int64
	messageID   int
	text        string
	warnDelTime time.Duration // time to delete the warning message, 0 to keep
	incidentID  int64         // incident backing the appeal button, 0 to omit the button
	botUsername string        // bot username for the appeal deep link
}

type banMessageRequest struct {
	chatID      int64
	text        string
	incidentID  int64
	botUsername string
	delTime     time.Duration // time to auto-delete the ban message, 0 to keep
}

// The bot must be an administrator in the supergroup for this to work
// and must have the appropriate admin rights.
// If channel is provided, it is banned instead of provided user, permanently.
func banUserOrChannel(ctx context.Context, r banRequest) error {
	if r.dry {
		bannedEntity := fmt.Sprintf("user %d", r.userID)
		if r.channelID != 0 {
			bannedEntity = fmt.Sprintf("channel %d", r.channelID)
		}
		observability.Logf(ctx, "[INFO] dry run: ban %s for %v", bannedEntity, r.duration)
		return nil
	}

	if r.training {
		bannedEntity := fmt.Sprintf("user %d", r.userID)
		if r.channelID != 0 {
			bannedEntity = fmt.Sprintf("channel %d", r.channelID)
		}
		observability.Logf(ctx, "[INFO] training mode: ban %s for %v", bannedEntity, r.duration)
		return nil
	}

	if r.duration < 30*time.Second {
		r.duration = time.Minute
	}

	if r.channelID != 0 {
		resp, err := r.tbAPI.Request(tbapi.BanChatSenderChatConfig{
			ChatConfig:   tbapi.ChatConfig{ChatID: r.chatID},
			SenderChatID: r.channelID,
			UntilDate:    int(time.Now().Add(r.duration).Unix()),
		})
		if err != nil {
			return fmt.Errorf("failed to ban channel: %w", err)
		}
		if !resp.Ok {
			return fmt.Errorf("response is not Ok: %v", string(resp.Result))
		}
		observability.Logf(ctx, "[INFO] channel %s banned by bot for %v", r.userName, r.duration)
		return nil
	}

	if r.restrict {
		resp, err := r.tbAPI.Request(tbapi.RestrictChatMemberConfig{
			ChatMemberConfig: tbapi.ChatMemberConfig{
				ChatConfig: tbapi.ChatConfig{ChatID: r.chatID},
				UserID:     r.userID,
			},
			UntilDate: time.Now().Add(r.duration).Unix(),
			Permissions: &tbapi.ChatPermissions{
				CanSendMessages:      false,
				CanSendAudios:        false,
				CanSendDocuments:     false,
				CanSendPhotos:        false,
				CanSendVideos:        false,
				CanSendVideoNotes:    false,
				CanSendVoiceNotes:    false,
				CanSendOtherMessages: false,
				CanChangeInfo:        false,
				CanInviteUsers:       false,
				CanPinMessages:       false,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to restrict user: %w", err)
		}
		if !resp.Ok {
			return fmt.Errorf("response is not Ok: %v", string(resp.Result))
		}
		observability.Logf(ctx, "[INFO] %s restricted by bot for %v", r.userName, r.duration)
		return nil
	}

	resp, err := r.tbAPI.Request(tbapi.BanChatMemberConfig{
		ChatMemberConfig: tbapi.ChatMemberConfig{
			ChatConfig: tbapi.ChatConfig{ChatID: r.chatID},
			UserID:     r.userID,
		},
		UntilDate: time.Now().Add(r.duration).Unix(),
	})
	if err != nil {
		return fmt.Errorf("failed to ban user: %w", err)
	}
	if !resp.Ok {
		return fmt.Errorf("response is not Ok: %v", string(resp.Result))
	}

	observability.Logf(ctx, "[INFO] user %s (%d) banned by bot in chat %d for %v (api resp ok=%t)",
		r.userName, r.userID, r.chatID, r.duration, resp.Ok)

	verifyBanApplied(ctx, r.tbAPI, r.chatID, r.userID)
	return nil
}

// verifyBanApplied calls GetChatMember after a ban to confirm Telegram actually
// applied the kick. Logs are diagnostic only; any error or panic from the mock
// must not break the surrounding ban flow.
func verifyBanApplied(ctx context.Context, api TbAPI, chatID, userID int64) {
	defer func() {
		if rec := recover(); rec != nil {
			observability.Logf(ctx, "[WARN] post-ban verify recovered from panic: %v", rec)
		}
	}()
	member, mErr := api.GetChatMember(tbapi.GetChatMemberConfig{
		ChatConfigWithUser: tbapi.ChatConfigWithUser{
			ChatConfig: tbapi.ChatConfig{ChatID: chatID},
			UserID:     userID,
		},
	})
	if mErr != nil {
		observability.Logf(ctx, "[WARN] post-ban GetChatMember failed for user %d in chat %d: %v", userID, chatID, mErr)
		return
	}
	observability.Logf(ctx, "[INFO] post-ban verify: user=%d chat=%d status=%q is_member=%t",
		userID, chatID, member.Status, member.IsMember)
}
