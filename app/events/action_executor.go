package events

import (
	"context"
	"fmt"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

type ActionExecutor interface {
	ApplyBan(ctx context.Context, req banRequest) error
	DeleteMessage(ctx context.Context, chatID int64, msgID int) error
	DeleteExtraMessages(ctx context.Context, checkResults []spamcheck.Response, userID int64, username string, chatID int64) error
}

type telegramActionExecutor struct {
	tbAPI        TbAPI
	dry          bool
	trainingMode bool
	superUsers   SuperUsers
}

func newTelegramActionExecutor(tbAPI TbAPI, dry, trainingMode bool, superUsers SuperUsers) telegramActionExecutor {
	return telegramActionExecutor{
		tbAPI:        tbAPI,
		dry:          dry,
		trainingMode: trainingMode,
		superUsers:   superUsers,
	}
}

func (e telegramActionExecutor) ApplyBan(ctx context.Context, req banRequest) error {
	req.tbAPI = e.tbAPI
	return banUserOrChannel(ctx, req)
}

func (e telegramActionExecutor) DeleteMessage(ctx context.Context, chatID int64, msgID int) error {
	_, err := e.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID:  msgID,
		ChatConfig: tbapi.ChatConfig{ChatID: chatID},
	}})
	if err != nil {
		observability.Logf(ctx, "[WARN] failed to delete message %d: %v", msgID, err)
		return fmt.Errorf("delete message %d: %w", msgID, err)
	}
	observability.Logf(ctx, "[DEBUG] deleted message %d", msgID)
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

	observability.Logf(ctx, "[INFO] user %s banned by bot for %v", r.userName, r.duration)
	return nil
}
