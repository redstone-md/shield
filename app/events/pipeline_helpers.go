package events

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
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
			Text:       msg.Text,
			Links:      collectLinks(msg),
			HasMedia:   msg.Image != nil || msg.WithVideo || msg.WithVideoNote || msg.WithAudio,
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
