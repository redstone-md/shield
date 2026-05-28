package events

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/hashicorp/go-multierror"

	"github.com/redstone-md/shield/app/bot"
)

func (l *TelegramListener) procNewChatMemberMessage(ctx context.Context, update tbapi.Update) error {
	fromChat := update.Message.Chat.ID
	if !l.isChatAllowed(fromChat) {
		return nil
	}

	if len(update.Message.NewChatMembers) != 1 {
		log.Printf("[DEBUG] we are expecting only one new chat member, got %d", len(update.Message.NewChatMembers))
		return nil
	}

	errs := new(multierror.Error)

	member := update.Message.NewChatMembers[0]
	msg := fmt.Sprintf("new_%d_%d", fromChat, member.ID)
	if err := l.Locator.AddMessage(ctx, msg, fromChat, member.ID, member.UserName, update.Message.MessageID); err != nil {
		errs = multierror.Append(errs, fmt.Errorf("failed to add new chat member message to locator: %w", err))
	}

	if err := errs.ErrorOrNil(); err != nil {
		return fmt.Errorf("failed to process new chat member: %w", err)
	}
	return nil
}

func (l *TelegramListener) procLeftChatMemberMessage(ctx context.Context, update tbapi.Update) error {
	fromChat := update.Message.Chat.ID
	if !l.isChatAllowed(fromChat) {
		return nil
	}

	if update.Message.From.ID == update.Message.LeftChatMember.ID {
		log.Printf("[DEBUG] left chat member is the same as the message sender, ignored")
		return nil
	}
	msg, found := l.Locator.Message(ctx, fmt.Sprintf("new_%d_%d", fromChat, update.Message.LeftChatMember.ID))
	if !found {
		log.Printf("[DEBUG] no new chat member message found for %d in chat %d", update.Message.LeftChatMember.ID, fromChat)
		return nil
	}
	if _, err := l.TbAPI.Request(tbapi.DeleteMessageConfig{
		BaseChatMessage: tbapi.BaseChatMessage{ChatConfig: tbapi.ChatConfig{ChatID: fromChat}, MessageID: msg.MsgID},
	}); err != nil {
		return fmt.Errorf("failed to delete new chat member message %d: %w", msg.MsgID, err)
	}

	return nil
}

func (l *TelegramListener) deleteSystemMessage(msgID int, chatID int64, msgType string) {
	deleteMsg := tbapi.DeleteMessageConfig{
		BaseChatMessage: tbapi.BaseChatMessage{
			MessageID:  msgID,
			ChatConfig: tbapi.ChatConfig{ChatID: chatID},
		},
	}
	if _, err := l.TbAPI.Request(deleteMsg); err != nil {
		log.Printf("[WARN] failed to delete %s message %d: %v", msgType, msgID, err)
	} else {
		log.Printf("[DEBUG] %s message %d deleted", msgType, msgID)
	}
}

func (l *TelegramListener) isLinkedChannel(msg *tbapi.Message) bool {
	if msg.SenderChat == nil {
		return false
	}
	if l.linkedChannelID != 0 && msg.SenderChat.ID == l.linkedChannelID {
		return true
	}
	for _, linkedID := range l.linkedChannelIDs {
		if msg.SenderChat.ID == linkedID {
			return true
		}
	}
	return false
}

func (l *TelegramListener) isChatAllowed(fromChat int64) bool {
	if l.chatIDsSet != nil {
		if _, ok := l.chatIDsSet[fromChat]; ok {
			return true
		}
	}
	if fromChat == l.chatID {
		return true
	}
	return slices.Contains(l.TestingIDs, fromChat)
}

func (l *TelegramListener) isAdminChat(fromChat int64, from string, fromID int64) bool {
	if fromChat == l.adminChatID {
		log.Printf("[DEBUG] message in admin chat %d, from %s (%d)", fromChat, from, fromID)
		if !l.SuperUsers.IsSuper(from, fromID) {
			log.Printf("[DEBUG] %s (%d) is not superuser in admin chat, ignored", from, fromID)
			return false
		}
		return true
	}
	return false
}

func (l *TelegramListener) getBanUsername(resp bot.Response, update tbapi.Update) string {
	if resp.ChannelID == 0 {
		return fmt.Sprintf("%v", resp.User)
	}
	botChat := bot.SenderChat{
		ID: resp.ChannelID,
	}
	if update.Message.SenderChat != nil {
		botChat.UserName = update.Message.SenderChat.UserName
	}
	if botChat.UserName == "" && update.Message.ReplyToMessage != nil && update.Message.ReplyToMessage.SenderChat != nil {
		if update.Message.ReplyToMessage.ForwardOrigin != nil {
			if update.Message.ReplyToMessage.ForwardOrigin.IsUser() {
				botChat.UserName = update.Message.ReplyToMessage.ForwardOrigin.SenderUser.UserName
			}
			if update.Message.ReplyToMessage.ForwardOrigin.IsHiddenUser() {
				botChat.UserName = update.Message.ReplyToMessage.ForwardOrigin.SenderUserName
			}
		}
	}
	return fmt.Sprintf("%v", botChat)
}

type NotificationType int

const (
	NotificationDefault NotificationType = iota
	NotificationSilent
)

func (l *TelegramListener) sendBotResponse(resp bot.Response, chatID int64, notifyType NotificationType) error {
	if !resp.Send {
		return nil
	}

	log.Printf("[DEBUG] bot response - %+v, reply-to:%d", strings.ReplaceAll(resp.Text, "\n", "\\n"), resp.ReplyTo)
	tbMsg := tbapi.NewMessage(chatID, resp.Text)
	tbMsg.ParseMode = tbapi.ModeMarkdown
	tbMsg.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	tbMsg.ReplyParameters = tbapi.ReplyParameters{MessageID: resp.ReplyTo}
	tbMsg.DisableNotification = notifyType == NotificationSilent

	if err := send(tbMsg, l.TbAPI); err != nil {
		return fmt.Errorf("can't send message to telegram %q: %w", resp.Text, err)
	}

	return nil
}

func (l *TelegramListener) getChatID(group string) (int64, error) {
	chatID, err := strconv.ParseInt(group, 10, 64)
	if err == nil {
		return chatID, nil
	}

	chat, err := l.TbAPI.GetChat(tbapi.ChatInfoConfig{ChatConfig: tbapi.ChatConfig{SuperGroupUsername: "@" + group}})
	if err != nil {
		return 0, fmt.Errorf("can't get chat for %s: %w", group, err)
	}

	return chat.ID, nil
}

func (l *TelegramListener) updateSupers() {
	isSuper := func(username string, id int64) bool {
		for _, super := range l.SuperUsers {
			if super == fmt.Sprintf("%d", id) {
				return true
			}
			if username != "" && super == username {
				return true
			}
		}
		return false
	}

	chatIDs := l.chatIDs
	if len(chatIDs) == 0 {
		chatIDs = []int64{l.chatID}
	}
	seen := make(map[int64]struct{})
	for _, chatID := range chatIDs {
		admins, err := l.TbAPI.GetChatAdministrators(tbapi.ChatAdministratorsConfig{ChatConfig: tbapi.ChatConfig{ChatID: chatID}})
		if err != nil {
			log.Printf("[WARN] failed to get chat administrators for %d: %v", chatID, err)
			continue
		}
		for _, adm := range admins {
			if adm.User.UserName == "" && adm.User.ID == 0 {
				continue
			}
			if _, dup := seen[adm.User.ID]; dup {
				continue
			}
			seen[adm.User.ID] = struct{}{}
			if isSuper(adm.User.UserName, adm.User.ID) {
				continue
			}
			l.SuperUsers = append(l.SuperUsers, fmt.Sprintf("%d", adm.User.ID))
		}
	}

	log.Printf("[INFO] added admins from %d groups, full list of supers: {%s}", len(l.chatIDs), strings.Join(l.SuperUsers, ", "))
}

type SuperUsers []string

func (s SuperUsers) IsSuper(userName string, userID int64) bool {
	for _, super := range s {
		if id, err := strconv.ParseInt(super, 10, 64); err == nil {
			if userID == id {
				return true
			}
			continue
		}
		if strings.EqualFold(userName, super) || strings.EqualFold("/"+userName, super) {
			return true
		}
	}
	return false
}
