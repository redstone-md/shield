package events

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/hashicorp/go-multierror"

	"github.com/redstone-md/shield/app/audit"
	"github.com/redstone-md/shield/app/bot"
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

	if strings.HasPrefix(callbackData, warnHamAskPrefix) {
		if err := a.callbackAskWarningHamConfirmation(query); err != nil {
			return fmt.Errorf("failed to make warning ham confirmation dialog: %w", err)
		}
		log.Printf("[DEBUG] warning ham confirmation request sent, chatID: %d, data: %s, orig: %q",
			chatID, callbackData, query.Message.Text)
		return nil
	}

	if strings.HasPrefix(callbackData, warnHamPrefix) {
		if err := a.callbackWarningHamConfirmed(ctx, query); err != nil {
			return fmt.Errorf("failed to confirm warning ham: %w", err)
		}
		log.Printf("[DEBUG] warning ham confirmed, chatID: %d, data: %s, orig: %q", chatID, callbackData, query.Message.Text)
		return nil
	}

	if strings.HasPrefix(callbackData, warnHamCancel) {
		if err := a.callbackWarningHamCancel(query); err != nil {
			return fmt.Errorf("failed to cancel warning ham: %w", err)
		}
		return nil
	}

	if strings.HasPrefix(callbackData, appealAcceptPrefix) {
		if err := a.callbackAppealResolve(ctx, query, true); err != nil {
			return fmt.Errorf("failed to accept appeal: %w", err)
		}
		log.Printf("[INFO] appeal accepted, chatID: %d, data: %s", chatID, callbackData)
		return nil
	}

	if strings.HasPrefix(callbackData, appealRejectPrefix) {
		if err := a.callbackAppealResolve(ctx, query, false); err != nil {
			return fmt.Errorf("failed to reject appeal: %w", err)
		}
		log.Printf("[INFO] appeal rejected, chatID: %d, data: %s", chatID, callbackData)
		return nil
	}

	if strings.HasPrefix(callbackData, dcUnbanAskPrefix) {
		if err := a.callbackDCUnbanAsk(query); err != nil {
			return fmt.Errorf("failed to make dc-gate unban confirmation: %w", err)
		}
		return nil
	}

	if strings.HasPrefix(callbackData, dcUnbanConfirmPrefix) {
		if err := a.callbackDCUnbanConfirmed(ctx, query); err != nil {
			return fmt.Errorf("failed to confirm dc-gate unban: %w", err)
		}
		log.Printf("[INFO] dc-gate user unbanned, chatID: %d, data: %s", chatID, callbackData)
		return nil
	}

	if strings.HasPrefix(callbackData, dcUnbanCancelPrefix) {
		if err := a.callbackDCUnbanCancel(query); err != nil {
			return fmt.Errorf("failed to cancel dc-gate unban: %w", err)
		}
		return nil
	}

	log.Printf("[DEBUG] unban action activated, chatID: %d, userID: %s, orig: %q", chatID, callbackData, query.Message.Text)
	if err := a.callbackUnbanConfirmed(ctx, query); err != nil {
		return fmt.Errorf("failed to unban user: %w", err)
	}
	log.Printf("[INFO] user unbanned, chatID: %d, userID: %s, orig: %q", chatID, callbackData, query.Message.Text)

	return nil
}

func (a *admin) callbackAskWarningHamConfirmation(query *tbapi.CallbackQuery) error {
	confirmationKeyboard := tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("Да, ham", warnHamPrefix+strings.TrimPrefix(query.Data, warnHamAskPrefix)),
			tbapi.NewInlineKeyboardButtonData("Отмена", warnHamCancel+strings.TrimPrefix(query.Data, warnHamAskPrefix)),
		),
	)
	editMsg := tbapi.NewEditMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, confirmationKeyboard)
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to make warning ham confirmation, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}
	return nil
}

func (a *admin) callbackWarningHamConfirmed(ctx context.Context, query *tbapi.CallbackQuery) error {
	callbackResponse := tbapi.NewCallback(query.ID, "принято")
	if _, err := a.tbAPI.Request(callbackResponse); err != nil {
		return fmt.Errorf("failed to send callback response: %w", err)
	}
	userID, _, err := parseCallbackData(query.Data)
	if err != nil {
		return fmt.Errorf("failed to parse warning ham callback %q: %w", query.Data, err)
	}

	cleanMsg, err := a.getCleanWarningMessage(query.Message.Text)
	if err != nil {
		return fmt.Errorf("failed to get warning message: %w", err)
	}
	if err := a.bot.UpdateHam(cleanMsg); err != nil {
		return fmt.Errorf("failed to update ham for %q: %w", cleanMsg, err)
	}
	if a.autoLearner != nil {
		a.autoLearner.LearnHam(ctx, cleanMsg, query.From.UserName)
	}
	if a.detectedSpam != nil {
		if err := a.deleteAllWarns(ctx, userID, ""); err != nil {
			return fmt.Errorf("failed to remove warning strikes for %d: %w", userID, err)
		}
	}

	updText := htmlEscape(query.Message.Text) + fmt.Sprintf("\n\nham подтвержден администратором %s за %v",
		htmlUserLink(query.From.UserName, query.From.ID), sinceQuery(query))
	editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, updText)
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: [][]tbapi.InlineKeyboardButton{}}
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to edit warning ham message, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}
	return nil
}

func (a *admin) callbackWarningHamCancel(query *tbapi.CallbackQuery) error {
	suffix := strings.TrimPrefix(query.Data, warnHamCancel)
	markup := tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("Не спам", warnHamAskPrefix+suffix),
			tbapi.NewInlineKeyboardButtonData("⚑ info", infoPrefix+suffix),
		),
	)
	editMsg := tbapi.NewEditMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, markup)
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to cancel warning ham confirmation, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}
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
	updText := htmlEscape(query.Message.Text) + fmt.Sprintf("\n\nбан подтвержден администратором %s за %v",
		htmlUserLink(query.From.UserName, query.From.ID), sinceQuery(query))
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

	userID, msgID, callbackChatID, parseErr := parseCallbackDataWithChat(query.Data)
	if parseErr != nil {
		return fmt.Errorf("failed to parse callback's userID %q: %w", query.Data, parseErr)
	}
	targetChatID := resolveCallbackChatID(callbackChatID, a.primChatIDs)

	if a.trainingMode {
		if err := a.deleteAndBan(ctx, query, userID, msgID, targetChatID); err != nil {
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
			tbAPI: a.tbAPI, dry: a.dry, training: a.trainingMode, userName: userName, restrict: false}
		if err := a.banInAllChats(ctx, banReq); err != nil {
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
		if uerr := a.unbanInAllChats(userID); uerr != nil {
			return uerr
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

	updText := htmlEscape(query.Message.Text) + fmt.Sprintf("\n\nразбанено администратором %s за %v",
		htmlUserLink(query.From.UserName, query.From.ID), sinceQuery(query))

	if !strings.Contains(query.Message.Text, "диагностика") &&
		!strings.Contains(query.Message.Text, "spam detection results") && userID != 0 {
		spamInfoText := []string{"\n\n<b>исходная диагностика</b>\n"}

		info, found := a.locator.Spam(ctx, userID)
		if found {
			for _, check := range info.Checks {
				spamInfoText = append(spamInfoText, "- "+htmlEscape(check.String()))
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

func (a *admin) unbanInChat(userID, chatID int64) error {
	if a.softBan {
		_, err := a.tbAPI.Request(tbapi.RestrictChatMemberConfig{
			ChatMemberConfig: tbapi.ChatMemberConfig{UserID: userID, ChatConfig: tbapi.ChatConfig{ChatID: chatID}},
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
		ChatMemberConfig: tbapi.ChatMemberConfig{UserID: userID, ChatConfig: tbapi.ChatConfig{ChatID: chatID}},
		OnlyIfBanned:     true,
	}
	_, err := a.tbAPI.Request(cfg)
	if err != nil {
		return fmt.Errorf("failed to unban user %d: %w", userID, err)
	}
	return nil
}

func (a *admin) unbanChannelInChat(channelID, chatID int64) error {
	_, err := a.tbAPI.Request(tbapi.UnbanChatSenderChatConfig{
		ChatConfig:   tbapi.ChatConfig{ChatID: chatID},
		SenderChatID: channelID,
	})
	if err != nil {
		return fmt.Errorf("failed to unban channel %d: %w", channelID, err)
	}
	return nil
}

func (a *admin) callbackShowInfo(ctx context.Context, query *tbapi.CallbackQuery) error {
	callbackData := query.Data
	spamInfoText := "<b>can't get spam info</b>"
	userID, _, err := parseCallbackData(callbackData)
	if err != nil {
		spamInfoText = fmt.Sprintf("<b>failed to parse userID from %q: %v</b>", htmlEscape(callbackData[1:]), err)
	}

	if userID != 0 {
		spamInfoText = a.spamInfoForCallback(ctx, userID, query.Message.Text)
	}

	escapedMessage := htmlEscape(query.Message.Text) + "\n\n<b>spam detection results</b>\n" + spamInfoText
	confirmationKeyboard := [][]tbapi.InlineKeyboardButton{}
	if query.Message.ReplyMarkup != nil && len(query.Message.ReplyMarkup.InlineKeyboard) > 0 {
		confirmationKeyboard = query.Message.ReplyMarkup.InlineKeyboard
		confirmationKeyboard[0] = confirmationKeyboard[0][:1]
	}
	editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, escapedMessage)
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: confirmationKeyboard}
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to send spam info, chatID:%d, msgID:%d, %w", query.Message.Chat.ID, query.Message.MessageID, err)
	}
	return nil
}

func (a *admin) deleteAndBan(ctx context.Context, query *tbapi.CallbackQuery, userID int64, msgID int, chatID int64) error {
	errs := new(multierror.Error)
	userName := a.locator.UserNameByID(ctx, userID)
	banReq := banRequest{
		duration:  bot.PermanentBanDuration,
		userID:    userID,
		channelID: channelIDFromCallback(userID),
		tbAPI:     a.tbAPI,
		dry:       a.dry,
		training:  false,
		userName:  userName,
	}

	msgFromSuper := userName != "" && a.superUsers.IsSuper(userName, userID)
	if !msgFromSuper {
		if err := a.banInAllChats(ctx, banReq); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to ban user %d: %w", userID, err))
		}
	}

	_, err := a.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
		MessageID:  msgID,
		ChatConfig: tbapi.ChatConfig{ChatID: a.chatIDOrFallback(chatID)},
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

func (a *admin) getCleanWarningMessage(msg string) (string, error) {
	msgLines := strings.Split(msg, "\n")
	if len(msgLines) < 3 {
		return "", fmt.Errorf("unexpected warning callback message: %q", msg)
	}

	endLine := len(msgLines)
	for i, line := range msgLines[2:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Причина:") || strings.HasPrefix(trimmed, "ham подтвержден") ||
			strings.HasPrefix(trimmed, "spam detection results") || strings.HasPrefix(trimmed, "**spam detection results**") {
			endLine = i + 2
			break
		}
	}

	cleanMsg := strings.TrimSpace(strings.Join(msgLines[2:endLine], "\n"))
	if cleanMsg == "" {
		return "", fmt.Errorf("no original warning message found in callback message: %q", msg)
	}
	return cleanMsg, nil
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
			tbapi.NewInlineKeyboardButtonData("⛔︎ "+action, formatCallbackData(confirmationPrefix, user.ID, msgID, chatID)),
			tbapi.NewInlineKeyboardButtonData("️⚑ info", formatCallbackData(infoPrefix, user.ID, msgID, chatID)),
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

// parseDCUserID extracts the plain user ID carried by DC-gate unban callbacks.
func parseDCUserID(query *tbapi.CallbackQuery, prefix string) (int64, error) {
	userID, err := strconv.ParseInt(strings.TrimPrefix(query.Data, prefix), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse user id from %q: %w", query.Data, err)
	}
	return userID, nil
}

// dcUnbanResolved reports whether this DC-gate notification has already been
// unbanned (terminal state), guarding against double confirmations and stale
// cancel presses on an edited message.
func dcUnbanResolved(query *tbapi.CallbackQuery) bool {
	return strings.Contains(query.Message.Text, "разбанено администратором")
}

// callbackDCUnbanAsk is step one of the two-step flow: it swaps the «Разбанить»
// button for an explicit confirmation pair, since confirming also whitelists
// the user against the DC gate.
func (a *admin) callbackDCUnbanAsk(query *tbapi.CallbackQuery) error {
	if _, err := a.tbAPI.Request(tbapi.NewCallback(query.ID, "")); err != nil {
		return fmt.Errorf("failed to send callback response: %w", err)
	}
	userID, err := parseDCUserID(query, dcUnbanAskPrefix)
	if err != nil {
		return err
	}
	keyboard := tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("Подтвердить разбан", dcUnbanConfirmPrefix+strconv.FormatInt(userID, 10)),
			tbapi.NewInlineKeyboardButtonData("Отмена", dcUnbanCancelPrefix+strconv.FormatInt(userID, 10)),
		),
	)
	editMsg := tbapi.NewEditMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, keyboard)
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to make dc-gate unban confirmation, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}
	return nil
}

// callbackDCUnbanConfirmed unbans the user in every primary chat and adds them
// to the approved list, which also exempts them from the DC join gate on
// rejoin. In dry/training modes no Telegram unban calls are made, but the
// approval still applies so the flow can be exercised end-to-end.
func (a *admin) callbackDCUnbanConfirmed(ctx context.Context, query *tbapi.CallbackQuery) error {
	userID, err := parseDCUserID(query, dcUnbanConfirmPrefix)
	if err != nil {
		return err
	}
	if dcUnbanResolved(query) {
		if _, cbErr := a.tbAPI.Request(tbapi.NewCallback(query.ID, "уже разбанено")); cbErr != nil {
			return fmt.Errorf("failed to send callback response: %w", cbErr)
		}
		return nil
	}
	if _, err := a.tbAPI.Request(tbapi.NewCallback(query.ID, "принято")); err != nil {
		return fmt.Errorf("failed to send callback response: %w", err)
	}

	if !a.trainingMode && !a.dry {
		// the DC gate always issues hard kicks, so lift them directly instead of
		// unbanInChat, whose soft-ban branch only lifts restrictions and would
		// leave a kicked user kicked
		if uerr := a.unbanHardInAllChats(userID); uerr != nil {
			return uerr
		}
	}

	if err := a.bot.AddApprovedUser(userID, dcGateUserName(query.Message.Text)); err != nil {
		return fmt.Errorf("failed to add user %d to approved list: %w", userID, err)
	}

	updText := htmlEscape(query.Message.Text) + fmt.Sprintf("\n\nразбанено администратором %s за %v",
		htmlUserLink(query.From.UserName, query.From.ID), sinceQuery(query))
	editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, updText)
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: [][]tbapi.InlineKeyboardButton{}}
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to edit message, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}
	return nil
}

// unbanHardInAllChats unbans a user in every monitored group with
// unbanChatMember, bypassing the soft-ban restrict-lifting of unbanInChat.
func (a *admin) unbanHardInAllChats(userID int64) error {
	var errs *multierror.Error
	for _, chatID := range a.primChatIDs {
		_, err := a.tbAPI.Request(tbapi.UnbanChatMemberConfig{
			ChatMemberConfig: tbapi.ChatMemberConfig{UserID: userID, ChatConfig: tbapi.ChatConfig{ChatID: chatID}},
			OnlyIfBanned:     true,
		})
		if err != nil {
			errs = multierror.Append(errs, fmt.Errorf("failed to unban user %d in chat %d: %w", userID, chatID, err))
		}
	}
	return errs.ErrorOrNil()
}

// dcGateUserName pulls the username out of a plain-text [DC GATE] notification
// ("[DC GATE] gpig_stepan (123) забанен по DC ..."). Telegram returns
// message.text entity-stripped, but if the notification ever went out through
// the plain-text fallback the raw anchor markup stays in the text, so any HTML
// tags are stripped from the capture. Returns "" when the banned user had no
// username.
func dcGateUserName(text string) string {
	m := dcGateUserRegex.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return htmlTagRegex.ReplaceAllString(m[1], "")
}

var (
	dcGateUserRegex = regexp.MustCompile(`\[DC GATE\] (.*?) \((-?\d+)\) забанен`)
	htmlTagRegex    = regexp.MustCompile(`<[^>]*>`)
)

// callbackDCUnbanCancel restores the original «Разбанить» keyboard.
func (a *admin) callbackDCUnbanCancel(query *tbapi.CallbackQuery) error {
	if _, err := a.tbAPI.Request(tbapi.NewCallback(query.ID, "")); err != nil {
		return fmt.Errorf("failed to send callback response: %w", err)
	}
	if dcUnbanResolved(query) {
		return nil // already unbanned, nothing to restore
	}
	userID, err := parseDCUserID(query, dcUnbanCancelPrefix)
	if err != nil {
		return err
	}
	keyboard := tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("Разбанить", dcUnbanAskPrefix+strconv.FormatInt(userID, 10)),
		),
	)
	editMsg := tbapi.NewEditMessageReplyMarkup(query.Message.Chat.ID, query.Message.MessageID, keyboard)
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to restore dc-gate keyboard, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}
	return nil
}

// appealResolver resolves user appeals from the admin-chat inline buttons.
type appealResolver interface {
	GetAppeal(ctx context.Context, appealID int64) (audit.Appeal, error)
	Accept(ctx context.Context, appealID int64, resolverID, resolutionText string) error
	Reject(ctx context.Context, appealID int64, resolverID, resolutionText string) error
}

// callbackAppealResolve handles the "✅ Принять" / "❌ Отклонить" admin buttons.
// a second tap on an already-resolved appeal is answered with a notice and
// performs no action.
func (a *admin) callbackAppealResolve(ctx context.Context, query *tbapi.CallbackQuery, accept bool) error {
	if a.appeals == nil {
		return fmt.Errorf("appeal resolver not configured")
	}

	prefix := appealAcceptPrefix
	if !accept {
		prefix = appealRejectPrefix
	}
	appealID, err := strconv.ParseInt(strings.TrimPrefix(query.Data, prefix), 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse appeal id from %q: %w", query.Data, err)
	}

	ap, err := a.appeals.GetAppeal(ctx, appealID)
	if err != nil {
		return fmt.Errorf("failed to load appeal %d: %w", appealID, err)
	}
	// only accepted/rejected are terminal; triaged/escalated appeals stay resolvable
	if ap.Status == audit.AppealAccepted || ap.Status == audit.AppealRejected {
		if _, rErr := a.tbAPI.Request(tbapi.NewCallback(query.ID, "Апелляция уже рассмотрена")); rErr != nil {
			return fmt.Errorf("failed to answer callback: %w", rErr)
		}
		return nil
	}

	toast := "принято"
	if !accept {
		toast = "отклонено"
	}
	if _, cbErr := a.tbAPI.Request(tbapi.NewCallback(query.ID, toast)); cbErr != nil {
		return fmt.Errorf("failed to answer callback: %w", cbErr)
	}

	resolverID := query.From.UserName
	if resolverID == "" {
		resolverID = fmt.Sprintf("%d", query.From.ID)
	}

	outcome := "✅ апелляция принята"
	if accept {
		err = a.appeals.Accept(ctx, appealID, resolverID, "")
	} else {
		outcome = "❌ апелляция отклонена"
		err = a.appeals.Reject(ctx, appealID, resolverID, "")
	}
	if err != nil {
		return fmt.Errorf("failed to resolve appeal %d: %w", appealID, err)
	}

	updText := htmlEscape(query.Message.Text) + fmt.Sprintf("\n\n%s администратором %s за %v",
		outcome, htmlUserLink(query.From.UserName, query.From.ID), sinceQuery(query))
	editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, updText)
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: [][]tbapi.InlineKeyboardButton{}}
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to edit appeal message, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}
	return nil
}
