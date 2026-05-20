package events

import (
	"context"
	"fmt"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/redstone-md/shield/app/audit"
)

// appealBotAdapter adapts the telegram listener to the audit.BotService
// interface so AppealService.Accept/Reject can unban users, clear their
// warning strikes and DM them the appeal outcome.
type appealBotAdapter struct {
	listener *TelegramListener
}

// NewAppealBotAdapter returns an audit.BotService backed by the listener.
func NewAppealBotAdapter(listener *TelegramListener) audit.BotService {
	return &appealBotAdapter{listener: listener}
}

// UnbanUser lifts a ban or restriction for the user in the primary chat.
func (b *appealBotAdapter) UnbanUser(_ context.Context, userID int64) error {
	if b.listener.adminHandler == nil {
		return fmt.Errorf("admin handler not initialized")
	}
	return b.listener.adminHandler.unban(userID)
}

// AddHamSample feeds the appealed message back as a ham sample.
func (b *appealBotAdapter) AddHamSample(_ context.Context, text string) error {
	if b.listener.Bot == nil || text == "" {
		return nil
	}
	return b.listener.Bot.UpdateHam(text)
}

// ClearUserWarnings removes every warning strike recorded for the user.
func (b *appealBotAdapter) ClearUserWarnings(ctx context.Context, userID int64) error {
	if b.listener.adminHandler == nil || b.listener.DetectedSpamCounter == nil {
		return nil
	}
	return b.listener.adminHandler.deleteAllWarns(ctx, userID, "")
}

// NotifyAppealResult DMs the user the appeal outcome. Best-effort: a blocked
// bot or closed DM returns an error that the caller logs and ignores.
func (b *appealBotAdapter) NotifyAppealResult(_ context.Context, userID int64, accepted bool) error {
	if b.listener.TbAPI == nil {
		return fmt.Errorf("tbAPI not initialized")
	}
	text := "❌ Апелляция отклонена."
	if accepted {
		text = "✅ Апелляция принята — вы разбанены, предупреждения сняты."
	}
	if _, err := b.listener.TbAPI.Send(tbapi.NewMessage(userID, text)); err != nil {
		return fmt.Errorf("notify user %d: %w", userID, err)
	}
	return nil
}

var _ audit.BotService = (*appealBotAdapter)(nil)
