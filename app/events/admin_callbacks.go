package events

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/hashicorp/go-multierror"

	"github.com/umputun/tg-spam/app/bot"
)

func (a *admin) InlineCallbackHandler(ctx context.Context, query *tbapi.CallbackQuery) error {
	callbackData := query.Data
	chatID := query.Message.Chat.ID
	if chatID != a.adminChatID {
		return nil
	}

	if strings.HasPrefix(callbackData, confirmationPrefix) {
		if err := a.callbackAskBanConfirmation(query); err != nil {
			return fmt.Errorf("failed to make ban confirmation dialog: %w", err)
		}
		log.Printf("[DEBUG] unban confirmation request sent, chatID: %d, userID: %s, orig: %q",
			chatID, callbackData[1:], query.Message.Text)
		return nil
	}

	if strings.HasPrefix(callbackData, banPrefix) {
		if err := a.callbackBanConfirmed(ctx, query); err != nil {
			return fmt.Errorf("failed confirmation ban: %w", err)
		}
		log.Printf("[DEBUG] ban confirmed, chatID: %d, userID: %s, orig: %q", chatID, callbackData, query.Message.Text)
		return nil
	}

	if strings.HasPrefix(callbackData, infoPrefix) {
		if err := a.callbackShowInfo(ctx, query); err != nil {
			return fmt.Errorf("failed to show spam info: %w", err)
		}
		log.Printf("[DEBUG] spam info sent, chatID: %d, userID: %s, orig: %q", chatID, callbackData, query.Message.Text)
		return nil
	}

	log.Printf("[DEBUG] unban action activated, chatID: %d, userID: %s, orig: %q", chatID, callbackData, query.Message.Text)
	if err := a.callbackUnbanConfirmed(ctx, query); err != nil {
		return fmt.Errorf("failed to unban user: %w", err)
	}
	log.Printf("[INFO] user unbanned, chatID: %d, userID: %s, orig: %q", chatID, callbackData, query.Message.Text)

	return nil
}

func (a *admin) callbackAskBanConfirmation(query *tbapi.CallbackQuery) error {
	callbackData := query.Data

	keepBanned := "Оставить бан"
	if a.trainingMode {
		keepBanned = "Подтвердить бан"
	}

	confirmationKeyboard := tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("Разбанить", callbackData[1:]),
			tbapi.NewInlineKeyboardButtonData(keepBanned, banPrefix+callbackData[1:]),
		),
	)
	editMsg := tbapi.NewEditMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, confirmationKeyboard)
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to make confiramtion, chatID:%d, msgID:%d, %w", query.Message.Chat.ID, query.Message.MessageID, err)
	}
	return nil
}

func (a *admin) callbackBanConfirmed(ctx context.Context, query *tbapi.CallbackQuery) error {
	updText := query.Message.Text + fmt.Sprintf("\n\nбан подтвержден администратором %s за %v",
		markdownUserLink(query.From.UserName, query.From.ID), sinceQuery(query))
	editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, updText)
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: [][]tbapi.InlineKeyboardButton{}}
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to clear confirmation, chatID:%d, msgID:%d, %w", query.Message.Chat.ID, query.Message.MessageID, err)
	}

	if cleanMsg, err := a.getCleanMessage(query.Message.Text); err == nil && cleanMsg != "" {
		if err = a.bot.UpdateSpam(cleanMsg); err != nil {
			return fmt.Errorf("failed to update spam for %q: %w", cleanMsg, err)
		}
		if a.autoLearner != nil {
			a.autoLearner.LearnSpam(ctx, cleanMsg, query.From.UserName)
		}
	} else {
		log.Printf("[DEBUG] failed to get clean message: %v", err)
	}

	userID, msgID, parseErr := parseCallbackData(query.Data)
	if parseErr != nil {
		return fmt.Errorf("failed to parse callback's userID %q: %w", query.Data, parseErr)
	}

	if a.trainingMode {
		if err := a.deleteAndBan(ctx, query, userID, msgID); err != nil {
			return fmt.Errorf("failed to ban user %d: %w", userID, err)
		}
	}

	if a.softBan && !a.trainingMode {
		userName, err := a.extractUsername(query.Message.Text)
		if err != nil {
			log.Printf("[DEBUG] failed to extract username from %q: %v", query.Message.Text, err)
			userName = ""
		}
		banReq := banRequest{duration: bot.PermanentBanDuration, userID: userID, channelID: channelIDFromCallback(userID),
			chatID: a.primChatID, tbAPI: a.tbAPI, dry: a.dry, training: a.trainingMode, userName: userName, restrict: false}
		if err := banUserOrChannel(ctx, banReq); err != nil {
			return fmt.Errorf("failed to ban user %d: %w", userID, err)
		}
	}

	return nil
}

func (a *admin) callbackUnbanConfirmed(ctx context.Context, query *tbapi.CallbackQuery) error {
	callbackData := query.Data
	chatID := query.Message.Chat.ID
	log.Printf("[DEBUG] unban action activated, chatID: %d, userID: %s", chatID, callbackData)
	callbackResponse := tbapi.NewCallback(query.ID, "принято")
	if _, err := a.tbAPI.Request(callbackResponse); err != nil {
		return fmt.Errorf("failed to send callback response: %w", err)
	}

	userID, _, err := parseCallbackData(callbackData)
	if err != nil {
		return fmt.Errorf("failed to parse callback msgsData %q: %w", callbackData, err)
	}

	if cleanMsg, cleanErr := a.getCleanMessage(query.Message.Text); cleanErr == nil && cleanMsg != "" {
		if upErr := a.bot.UpdateHam(cleanMsg); upErr != nil {
			return fmt.Errorf("failed to update ham for %q: %w", cleanMsg, upErr)
		}
		if a.autoLearner != nil {
			a.autoLearner.LearnHam(ctx, cleanMsg, query.From.UserName)
		}
	} else {
		log.Printf("[DEBUG] failed to get clean message: %v", cleanErr)
	}

	if !a.trainingMode {
		if userID < 0 {
			if uerr := a.unbanChannel(userID); uerr != nil {
				return uerr
			}
		} else {
			if uerr := a.unban(userID); uerr != nil {
				return uerr
			}
		}
	}

	name, err := a.extractUsername(query.Message.Text)
	if err != nil {
		log.Printf("[DEBUG] failed to extract username from %q: %v", query.Message.Text, err)
		name = ""
	}
	if err := a.bot.AddApprovedUser(userID, name); err != nil {
		return fmt.Errorf("failed to add user %d to approved list: %w", userID, err)
	}

	updText := query.Message.Text + fmt.Sprintf("\n\nразбанено администратором %s за %v",
		markdownUserLink(query.From.UserName, query.From.ID), sinceQuery(query))

	if !strings.Contains(query.Message.Text, "диагностика") && !strings.Contains(query.Message.Text, "spam detection results") && userID != 0 {
		spamInfoText := []string{"\n\n**исходная диагностика**\n"}

		info, found := a.locator.Spam(ctx, userID)
		if found {
			for _, check := range info.Checks {
				spamInfoText = append(spamInfoText, "- "+escapeMarkDownV1Text(check.String()))
			}
		}

		if len(spamInfoText) > 1 {
			updText += strings.Join(spamInfoText, "\n")
		}
	}

	editMsg := tbapi.NewEditMessageText(chatID, query.Message.MessageID, updText)
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: [][]tbapi.InlineKeyboardButton{}}
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to edit message, chatID:%d, msgID:%d, %w", chatID, query.Message.MessageID, err)
	}
	return nil
}

func (a *admin) unban(userID int64) error {
	if a.softBan {
		_, err := a.tbAPI.Request(tbapi.RestrictChatMemberConfig{
			ChatMemberConfig: tbapi.ChatMemberConfig{UserID: userID, ChatConfig: tbapi.ChatConfig{ChatID: a.primChatID}},
			Permissions: &tbapi.ChatPermissions{
				CanSendMessages:      true,
				CanSendAudios:        true,
				CanSendDocuments:     true,
				CanSendPhotos:        true,
				CanSendVideos:        true,
				CanSendVideoNotes:    true,
				CanSendVoiceNotes:    true,
				CanSendOtherMessages: true,
				CanChangeInfo:        true,
				CanInviteUsers:       true,
				CanPinMessages:       true,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to drop restrictions for user %d: %w", userID, err)
		}
		return nil
	}

	cfg := tbapi.UnbanChatMemberConfig{
		ChatMemberConfig: tbapi.ChatMemberConfig{UserID: userID, ChatConfig: tbapi.ChatConfig{ChatID: a.primChatID}},
		OnlyIfBanned:     true,
	}
	_, err := a.tbAPI.Request(cfg)
	if err != nil {
		return fmt.Errorf("failed to unban user %d: %w", userID, err)
	}
	return nil
}

func (a *admin) unbanChannel(channelID int64) error {
	_, err := a.tbAPI.Request(tbapi.UnbanChatSenderChatConfig{
		ChatConfig:   tbapi.ChatConfig{ChatID: a.primChatID},
		SenderChatID: channelID,
	})
	if err != nil {
		return fmt.Errorf("failed to unban channel %d: %w", channelID, err)
	}
	return nil
}

func (a *admin) callbackShowInfo(ctx context.Context, query *tbapi.CallbackQuery) error {
	callbackData := query.Data
	spamInfoText := "**can't get spam info**"
	spamInfo := []string{}
	userID, _, err := parseCallbackData(callbackData)
	if err != nil {
		spamInfo = append(spamInfo, fmt.Sprintf("**failed to parse userID from %q: %v**", callbackData[1:], err))
	}

	if userID != 0 {
		info, found := a.locator.Spam(ctx, userID)
		if found {
			for _, check := range info.Checks {
				spamInfo = append(spamInfo, "- "+escapeMarkDownV1Text(check.String()))
			}
		}
		if len(spamInfo) > 0 {
			spamInfoText = strings.Join(spamInfo, "\n")
		}
	}

	escapedMessage := escapeMarkDownV1Text(query.Message.Text) + "\n\n**spam detection results**\n" + spamInfoText
	confirmationKeyboard := [][]tbapi.InlineKeyboardButton{}
	if query.Message.ReplyMarkup != nil && len(query.Message.ReplyMarkup.InlineKeyboard) > 0 {
		confirmationKeyboard = query.Message.ReplyMarkup.InlineKeyboard
		confirmationKeyboard[0] = confirmationKeyboard[0][:1]
	}
	editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, escapedMessage)
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: confirmationKeyboard}
	editMsg.ParseMode = tbapi.ModeMarkdown
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to send spam info, chatID:%d, msgID:%d, %w", query.Message.Chat.ID, query.Message.MessageID, err)
	}
	return nil
}

func (a *admin) deleteAndBan(ctx context.Context, query *tbapi.CallbackQuery, userID int64, msgID int) error {
	errs := new(multierror.Error)
	userName := a.locator.UserNameByID(ctx, userID)
	banReq := banRequest{
		duration:  bot.PermanentBanDuration,
		userID:    userID,
		channelID: channelIDFromCallback(userID),
		chatID:    a.primChatID,
		tbAPI:     a.tbAPI,
		dry:       a.dry,
		training:  false,
		userName:  userName,
	}

	msgFromSuper := userName != "" && a.superUsers.IsSuper(userName, userID)
	if !msgFromSuper {
		if err := banUserOrChannel(ctx, banReq); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to ban user %d: %w", userID, err))
		}
	}

	_, err := a.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID:  msgID,
		ChatConfig: tbapi.ChatConfig{ChatID: a.primChatID},
	}})
	if err != nil {
		return fmt.Errorf("failed to delete message %d: %w", query.Message.MessageID, err)
	}

	if errs.ErrorOrNil() != nil {
		errMsgs := []string{}
		for _, err := range errs.Errors {
			errStr := err.Error()
			errMsgs = append(errMsgs, errStr)
		}
		return errors.New(strings.Join(errMsgs, "\n"))
	}

	if msgFromSuper {
		log.Printf("[INFO] message %d deleted, user %q (%d) is super, not banned", msgID, userName, userID)
	} else {
		log.Printf("[INFO] message %d deleted, user %q (%d) banned", msgID, userName, userID)
	}
	return nil
}

func (a *admin) getCleanMessage(msg string) (string, error) {
	msgLines := strings.Split(msg, "\n")
	if len(msgLines) < 2 {
		return "", fmt.Errorf("unexpected message from callback msgsData: %q", msg)
	}

	spamInfoLine := len(msgLines)
	for i, line := range msgLines {
		if strings.HasPrefix(line, "spam detection results") || strings.HasPrefix(line, "**spam detection results**") {
			spamInfoLine = i
			break
		}
	}

	if spamInfoLine <= 2 {
		return "", fmt.Errorf("no original message found in callback msgsData: %q", msg)
	}

	cleanMsg := strings.Join(msgLines[2:spamInfoLine], "\n")
	return strings.TrimSpace(cleanMsg), nil
}

func (a *admin) sendWithUnbanMarkup(text, action string, user bot.User, msgID int, chatID int64) error {
	log.Printf("[DEBUG] action response %q: user %+v, msgID:%d, text: %q",
		action, user, msgID, strings.ReplaceAll(text, "\n", "\\n"))
	tbMsg := tbapi.NewMessage(chatID, text)
	tbMsg.ParseMode = tbapi.ModeHTML
	tbMsg.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	log.Printf("[DEBUG] sending message with ParseMode=%q, text contains tg://user: %v",
		tbMsg.ParseMode, strings.Contains(text, "tg://user?id="))

	tbMsg.ReplyMarkup = tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("⛔︎ "+action, fmt.Sprintf("%s%d:%d", confirmationPrefix, user.ID, msgID)),
			tbapi.NewInlineKeyboardButtonData("️⚑ info", fmt.Sprintf("%s%d:%d", infoPrefix, user.ID, msgID)),
		),
	)

	if _, err := a.tbAPI.Send(tbMsg); err != nil {
		log.Printf("[ERROR] failed to send message with HTML ParseMode: %v", err)
		log.Printf("[DEBUG] attempting to send without parse mode as fallback")
		tbMsg.ParseMode = ""
		if _, err := a.tbAPI.Send(tbMsg); err != nil {
			return fmt.Errorf("can't send message to telegram %q: %w", text, err)
		}
		log.Printf("[WARN] message sent without parse mode (HTML failed)")
	} else {
		log.Printf("[DEBUG] message sent successfully with HTML ParseMode")
	}
	return nil
}

func (a *admin) extractUsername(text string) (string, error) {
	// HTML format: <a href="tg://user?id=123">name</a>
	htmlUserRegex := regexp.MustCompile(`<a href="tg://user\?id=\d+">(.+?)</a>`)
	if matches := htmlUserRegex.FindStringSubmatch(text); len(matches) > 1 {
		return matches[1], nil
	}

	// HTML format: <a href="https://t.me/username">username</a>
	htmlTmeRegex := regexp.MustCompile(`<a href="https://t\.me/(\S+?)">`)
	if matches := htmlTmeRegex.FindStringSubmatch(text); len(matches) > 1 {
		return matches[1], nil
	}

	markdownLinkRegex := regexp.MustCompile(`\[(.+?)\]\((?:tg://user\?id=\d+|https://t\.me/[^)]+)\)`)
	if matches := markdownLinkRegex.FindStringSubmatch(text); len(matches) > 1 {
		return matches[1], nil
	}

	plainChannelRegex := regexp.MustCompile(`(?:permanently banned|забанен навсегда) (.+?) \(-?\d+\)`)
	if matches := plainChannelRegex.FindStringSubmatch(text); len(matches) > 1 {
		return matches[1], nil
	}

	plainRegex := regexp.MustCompile(`\{\d+ (\S+) .+?\}`)
	if matches := plainRegex.FindStringSubmatch(text); len(matches) > 1 {
		return matches[1], nil
	}

	return "", errors.New("username not found")
}
