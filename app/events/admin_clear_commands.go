package events

import (
	"context"
	"fmt"

	tbapi "github.com/OvyFlash/telegram-bot-api"
)

func (a *admin) DirectClearTarget(ctx context.Context, update tbapi.Update, target string) error {
	userID, userName, err := a.resolveUserTarget(ctx, target, "clear")
	if err != nil {
		return err
	}
	return a.clearUserMessages(ctx, update, userID, userName)
}

func (a *admin) DirectClearReply(ctx context.Context, update tbapi.Update) error {
	if update.Message == nil || update.Message.ReplyToMessage == nil {
		return fmt.Errorf("clear command requires a reply message")
	}
	userID, userName := clearSubject(update.Message.ReplyToMessage)
	if userID == 0 {
		return fmt.Errorf("clear command can't resolve reply target")
	}
	return a.clearUserMessages(ctx, update, userID, userName)
}

func (a *admin) clearUserMessages(ctx context.Context, update tbapi.Update, userID int64, userName string) error {
	if update.Message == nil {
		return fmt.Errorf("clear command requires a message")
	}
	if a.locator == nil {
		return fmt.Errorf("clear command requires locator")
	}
	if a.superUsers.IsSuper(userName, userID) {
		return fmt.Errorf("clear target is super-user %s (%d), ignored", userName, userID)
	}

	deleted, err := a.deleteUserMessages(ctx, userID)
	if err != nil {
		return fmt.Errorf("direct clear failed: %w", err)
	}
	if err := a.deleteMessage(ctx, update.Message.Chat.ID, update.Message.MessageID, "clear command"); err != nil {
		return fmt.Errorf("direct clear failed: %w", err)
	}

	msg := fmt.Sprintf("удалено %d сообщений пользователя %s администратором %s",
		deleted,
		markdownBanTarget(userName, userID),
		markdownUserLink(update.Message.From.UserName, update.Message.From.ID))
	if err := send(tbapi.NewMessage(a.adminChatID, msg), a.tbAPI); err != nil {
		return fmt.Errorf("failed to send direct clear notification: %w", err)
	}
	return nil
}

func clearSubject(msg *tbapi.Message) (userID int64, userName string) {
	if msg == nil {
		return 0, ""
	}
	if msg.SenderChat != nil && msg.SenderChat.ID != 0 {
		return msg.SenderChat.ID, msg.SenderChat.UserName
	}
	if msg.From != nil {
		return msg.From.ID, msg.From.UserName
	}
	return 0, ""
}
