package events

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/audit"
)

// callback prefixes for appeal accept/reject inline buttons; dispatched in
// admin_callbacks.go by InlineCallbackHandler.
const (
	appealAcceptPrefix = "AA"
	appealRejectPrefix = "AR"
)

// appealFiler files appeals and looks up incident/appeal state for the
// /start deep-link flow.
type appealFiler interface {
	Submit(ctx context.Context, incidentID, appellantUserID int64, appellantName, appealText string) (audit.Appeal, error)
	GetForIncident(ctx context.Context, incidentID int64) (audit.Appeal, error)
	GetIncident(ctx context.Context, incidentID int64) (audit.Incident, error)
}

// appealService is the listener-level view of the appeal subsystem: it both
// files appeals (appealFiler) and resolves them. Satisfied by *audit.AppealService.
type appealService interface {
	appealFiler
	appealResolver
}

// appealHandler processes "/start <incidentID>" deep links sent to the bot DM,
// files the appeal and notifies the admin chat.
type appealHandler struct {
	tbAPI       TbAPI
	appeals     appealFiler
	adminChatID int64
}

func newAppealHandler(tbAPI TbAPI, appeals appealFiler, adminChatID int64) *appealHandler {
	return &appealHandler{tbAPI: tbAPI, appeals: appeals, adminChatID: adminChatID}
}

// Handle validates the deep-link payload, files an appeal for the incident and
// posts an admin-chat notification with accept/reject buttons. Validation
// failures reply to the user and return nil; only unexpected errors propagate.
func (h *appealHandler) Handle(ctx context.Context, msg *tbapi.Message, payload string) error {
	if msg == nil || msg.From == nil {
		return nil
	}

	incidentID, err := strconv.ParseInt(strings.TrimSpace(payload), 10, 64)
	if err != nil || incidentID <= 0 {
		return h.reply(msg.Chat.ID, "Неверная ссылка.")
	}

	inc, err := h.appeals.GetIncident(ctx, incidentID)
	if err != nil {
		return h.reply(msg.Chat.ID, "Инцидент не найден.")
	}

	if msg.From.ID != inc.SpamUserID {
		return h.reply(msg.Chat.ID, "Эта ссылка не для вас.")
	}

	if inc.Status == audit.IncidentStatusClosed || inc.Status == audit.IncidentStatusResolved {
		return h.reply(msg.Chat.ID, "Наказание уже неактивно.")
	}

	if existing, gErr := h.appeals.GetForIncident(ctx, incidentID); gErr == nil && existing.ID > 0 {
		if existing.Status == audit.AppealAccepted || existing.Status == audit.AppealRejected {
			return h.reply(msg.Chat.ID, "Апелляция уже рассмотрена.")
		}
		return h.reply(msg.Chat.ID, "Апелляция уже подана, ожидайте решения модераторов.")
	}

	appellantName := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	appeal, err := h.appeals.Submit(ctx, incidentID, msg.From.ID, appellantName, "")
	if err != nil {
		_ = h.reply(msg.Chat.ID, "Не удалось подать апелляцию, попробуйте позже.")
		return fmt.Errorf("submit appeal for incident %d: %w", incidentID, err)
	}

	if err := h.reply(msg.Chat.ID, "✅ Апелляция подана, ожидайте решения модераторов."); err != nil {
		return err
	}

	h.notifyAdmin(appeal, inc, msg.From)
	return nil
}

func (h *appealHandler) reply(chatID int64, text string) error {
	if _, err := h.tbAPI.Send(tbapi.NewMessage(chatID, text)); err != nil {
		return fmt.Errorf("reply to appeal chat %d: %w", chatID, err)
	}
	return nil
}

// notifyAdmin posts the appeal to the admin chat with accept/reject buttons.
// a failure here is logged, not propagated: the appeal is already filed.
func (h *appealHandler) notifyAdmin(appeal audit.Appeal, inc audit.Incident, from *tbapi.User) {
	if h.adminChatID == 0 {
		return
	}
	reason := inc.ReasonText
	if reason == "" {
		reason = string(inc.ReasonCode)
	}
	snippet := strings.ReplaceAll(htmlEscape(truncateString(inc.MessageText, 200, "…")), "\n", " ")
	text := fmt.Sprintf("📩 <b>Апелляция</b> по инциденту #%d\n%s\n\n%s\n\n%s",
		inc.ID, htmlEscape(appealAppellantLabel(from)), htmlEscape(reason), snippet)

	msgConfig := tbapi.NewMessage(h.adminChatID, text)
	msgConfig.ParseMode = tbapi.ModeHTML
	msgConfig.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	msgConfig.ReplyMarkup = tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("✅ Принять", fmt.Sprintf("%s%d", appealAcceptPrefix, appeal.ID)),
			tbapi.NewInlineKeyboardButtonData("❌ Отклонить", fmt.Sprintf("%s%d", appealRejectPrefix, appeal.ID)),
		),
	)
	if _, err := h.tbAPI.Send(msgConfig); err != nil {
		log.Printf("[WARN] failed to send appeal notification to admin chat: %v", err)
	}
}

// appealAppellantLabel renders a "<name> (<id>)" label for the appellant.
func appealAppellantLabel(u *tbapi.User) string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		name = u.UserName
	}
	if name == "" {
		return fmt.Sprintf("user %d", u.ID)
	}
	return fmt.Sprintf("%s (%d)", name, u.ID)
}
