package events

import (
	"context"
	"fmt"
	"log"
	"strings"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/redstone-md/shield/app/bot"
)

func (r *userReports) callbackReportBan(ctx context.Context, query *tbapi.CallbackQuery) error {
	reportedUserID, msgID, callbackChatID, err := parseCallbackDataWithChat(query.Data)
	if err != nil {
		return fmt.Errorf("failed to parse callback data: %w", err)
	}
	lookupChatID := resolveCallbackChatID(callbackChatID, r.primChatIDs)

	reports, err := r.Storage.GetByMessage(ctx, msgID, lookupChatID)
	if err != nil {
		return fmt.Errorf("failed to get reports for msgID:%d: %w", msgID, err)
	}
	if len(reports) == 0 {
		return fmt.Errorf("no reports found for msgID:%d", msgID)
	}

	chatID := reports[0].ChatID
	msgText := reports[0].MsgText
	reportedUserName := reports[0].ReportedUserName
	ctx = r.reportActionContext(ctx, "admin_ban", msgID, reportedUserID)

	if remErr := r.bot.RemoveApprovedUser(reportedUserID); remErr != nil {
		log.Printf("[DEBUG] can't remove user %d from approved list: %v", reportedUserID, remErr)
	}

	if !r.dry && msgText != "" {
		if spamErr := r.bot.UpdateSpam(msgText); spamErr != nil {
			log.Printf("[WARN] failed to update spam samples: %v", spamErr)
		}
		if r.autoLearner != nil {
			r.autoLearner.LearnSpam(ctx, msgText, query.From.UserName)
		}
	}

	if r.actions != nil {
		if delErr := r.actions.DeleteMessage(ctx, chatID, msgID); delErr != nil {
			log.Printf("[WARN] failed to delete reported message %d: %v", msgID, delErr)
		} else {
			log.Printf("[INFO] reported message %d deleted", msgID)
		}
	} else if !r.dry {
		_, err = r.tbAPI.Request(tbapi.DeleteMessageConfig{BaseChatMessage: tbapi.BaseChatMessage{
			MessageID:  msgID,
			ChatConfig: tbapi.ChatConfig{ChatID: chatID},
		}})
		if err != nil {
			log.Printf("[WARN] failed to delete reported message %d: %v", msgID, err)
		} else {
			log.Printf("[INFO] reported message %d deleted", msgID)
		}
	}

	req := banRequest{
		duration: bot.PermanentBanDuration,
		userID:   reportedUserID,
		chatID:   chatID,
		dry:      r.dry,
		training: r.trainingMode,
		userName: reportedUserName,
		restrict: r.softBanMode,
	}
	if r.actions != nil {
		if err := r.actions.ApplyBan(ctx, req); err != nil {
			log.Printf("[WARN] failed to ban user %d: %v", reportedUserID, err)
		}
	} else {
		req.tbAPI = r.tbAPI
		if err := banUserOrChannel(ctx, req); err != nil {
			log.Printf("[WARN] failed to ban user %d: %v", reportedUserID, err)
		}
	}

	if err := r.Storage.DeleteByMessage(ctx, msgID, chatID); err != nil {
		log.Printf("[WARN] failed to delete reports for msgID:%d: %v", msgID, err)
	}

	updText := htmlEscape(query.Message.Text) + fmt.Sprintf("\n\nзабанено администратором %s за %v",
		htmlUserLink(query.From.UserName, query.From.ID), sinceQuery(query))
	editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, updText)
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: [][]tbapi.InlineKeyboardButton{}}
	if err := send(editMsg, r.tbAPI); err != nil {
		return fmt.Errorf("failed to update notification, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}

	log.Printf("[INFO] report ban approved for user %d by admin %s", reportedUserID, query.From.UserName)
	return nil
}

func (r *userReports) callbackReportReject(ctx context.Context, query *tbapi.CallbackQuery) error {
	_, msgID, callbackChatID, err := parseCallbackDataWithChat(query.Data)
	if err != nil {
		return fmt.Errorf("failed to parse callback data: %w", err)
	}
	lookupChatID := resolveCallbackChatID(callbackChatID, r.primChatIDs)

	reports, err := r.Storage.GetByMessage(ctx, msgID, lookupChatID)
	if err != nil {
		return fmt.Errorf("failed to get reports for msgID:%d: %w", msgID, err)
	}
	if len(reports) == 0 {
		return fmt.Errorf("no reports found for msgID:%d", msgID)
	}

	chatID := reports[0].ChatID

	if err := r.Storage.DeleteByMessage(ctx, msgID, chatID); err != nil {
		log.Printf("[WARN] failed to delete reports for msgID:%d: %v", msgID, err)
	}

	updText := htmlEscape(query.Message.Text) + fmt.Sprintf("\n\nотклонено администратором %s за %v",
		htmlUserLink(query.From.UserName, query.From.ID), sinceQuery(query))
	editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, updText)
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: [][]tbapi.InlineKeyboardButton{}}
	if err := send(editMsg, r.tbAPI); err != nil {
		return fmt.Errorf("failed to update notification, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}

	log.Printf("[INFO] report rejected by admin %s for msgID:%d", query.From.UserName, msgID)
	return nil
}

func (r *userReports) callbackReportBanReporterAsk(ctx context.Context, query *tbapi.CallbackQuery) error {
	reportedUserID, msgID, callbackChatID, err := parseCallbackDataWithChat(query.Data)
	if err != nil {
		return fmt.Errorf("failed to parse callback data: %w", err)
	}
	lookupChatID := resolveCallbackChatID(callbackChatID, r.primChatIDs)

	reports, err := r.Storage.GetByMessage(ctx, msgID, lookupChatID)
	if err != nil {
		return fmt.Errorf("failed to get reports for msgID:%d: %w", msgID, err)
	}
	if len(reports) == 0 {
		return fmt.Errorf("no reports found for msgID:%d", msgID)
	}

	keyboard := make([][]tbapi.InlineKeyboardButton, 0, len(reports)+1)
	for _, report := range reports {
		reporterName := report.ReporterUserName
		if reporterName == "" {
			reporterName = fmt.Sprintf("user_%d", report.ReporterUserID)
		}
		button := tbapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("Забанить %s", reporterName),
			fmt.Sprintf("R!%d:%d:%d", report.ReporterUserID, msgID, lookupChatID),
		)
		keyboard = append(keyboard, []tbapi.InlineKeyboardButton{button})
	}

	cancelButton := tbapi.NewInlineKeyboardButtonData(
		"Отмена",
		fmt.Sprintf("RX%d:%d:%d", reportedUserID, msgID, lookupChatID),
	)
	keyboard = append(keyboard, []tbapi.InlineKeyboardButton{cancelButton})

	editMsg := tbapi.NewEditMessageReplyMarkup(
		query.Message.Chat.ID,
		query.Message.MessageID,
		tbapi.InlineKeyboardMarkup{InlineKeyboard: keyboard},
	)
	if _, err := r.tbAPI.Send(editMsg); err != nil {
		return fmt.Errorf("failed to update keyboard, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}

	log.Printf("[INFO] ban reporter confirmation shown for msgID:%d", msgID)
	return nil
}

func (r *userReports) callbackReportBanReporterConfirm(ctx context.Context, query *tbapi.CallbackQuery) error {
	reporterID, msgID, callbackChatID, err := parseCallbackDataWithChat(query.Data)
	if err != nil {
		return fmt.Errorf("failed to parse callback data: %w", err)
	}
	lookupChatID := resolveCallbackChatID(callbackChatID, r.primChatIDs)

	reports, err := r.Storage.GetByMessage(ctx, msgID, lookupChatID)
	if err != nil {
		return fmt.Errorf("failed to get reports for msgID:%d: %w", msgID, err)
	}
	if len(reports) == 0 {
		return fmt.Errorf("no reports found for msgID:%d", msgID)
	}

	chatID := reports[0].ChatID

	var reporterName string
	for _, report := range reports {
		if report.ReporterUserID == reporterID {
			reporterName = report.ReporterUserName
			break
		}
	}
	if reporterName == "" {
		reporterName = fmt.Sprintf("user_%d", reporterID)
	}
	ctx = r.reportActionContext(ctx, "ban_reporter", msgID, reporterID)

	req := banRequest{
		duration: bot.PermanentBanDuration,
		userID:   reporterID,
		chatID:   chatID,
		dry:      r.dry,
		training: r.trainingMode,
		userName: reporterName,
	}
	if r.actions != nil {
		if banErr := r.actions.ApplyBan(ctx, req); banErr != nil {
			log.Printf("[WARN] failed to ban reporter %d: %v", reporterID, banErr)
		}
	} else {
		req.tbAPI = r.tbAPI
		if banErr := banUserOrChannel(ctx, req); banErr != nil {
			log.Printf("[WARN] failed to ban reporter %d: %v", reporterID, banErr)
		}
	}

	if delErr := r.Storage.DeleteReporter(ctx, reporterID, msgID, chatID); delErr != nil {
		log.Printf("[WARN] failed to delete reporter %d from database: %v", reporterID, delErr)
	}

	remainingReports, err := r.Storage.GetByMessage(ctx, msgID, lookupChatID)
	if err != nil {
		log.Printf("[WARN] failed to get remaining reports for msgID:%d: %v", msgID, err)
	}

	if len(remainingReports) == 0 {
		if delErr := r.Storage.DeleteByMessage(ctx, msgID, chatID); delErr != nil {
			log.Printf("[WARN] failed to delete reports for msgID:%d: %v", msgID, delErr)
		}

		updText := htmlEscape(query.Message.Text) + fmt.Sprintf("\n\nвсе репортеры забанены администратором %s за %v",
			htmlUserLink(query.From.UserName, query.From.ID), sinceQuery(query))
		editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, updText)
		editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: [][]tbapi.InlineKeyboardButton{}}
		if err := send(editMsg, r.tbAPI); err != nil {
			return fmt.Errorf("failed to update notification, chatID:%d, msgID:%d, %w",
				query.Message.Chat.ID, query.Message.MessageID, err)
		}
	} else {
		reportedUserID := reports[0].ReportedUserID
		reportedUserName := reports[0].ReportedUserName

		msgText := htmlEscape(truncateString(strings.ReplaceAll(remainingReports[0].MsgText, "\n", " "), 200, "..."))

		var reporterList []string
		for _, report := range remainingReports {
			rName := report.ReporterUserName
			if rName == "" {
				rName = fmt.Sprintf("user%d", report.ReporterUserID)
			}
			reporterList = append(reporterList, "- "+htmlUserLink(rName, report.ReporterUserID))
		}

		updText := fmt.Sprintf("<b>Жалобы на спам (%d)</b>\n\n%s\n\n%s\n\n<b>Кто пожаловался:</b>\n%s",
			len(remainingReports),
			htmlUserLink(reportedUserName, reportedUserID),
			msgText,
			strings.Join(reporterList, "\n"))
		updText += fmt.Sprintf("\n\nрепортер %s забанен администратором %s",
			htmlUserLink(reporterName, reporterID), htmlUserLink(query.From.UserName, query.From.ID))

		padding := strings.Repeat("\u2800", 30)
		updText += "\n\n" + padding

		keyboard := tbapi.NewInlineKeyboardMarkup(
			tbapi.NewInlineKeyboardRow(
				tbapi.NewInlineKeyboardButtonData("✅ Забанить", fmt.Sprintf("R+%d:%d:%d", reportedUserID, msgID, chatID)),
				tbapi.NewInlineKeyboardButtonData("❌ Отклонить", fmt.Sprintf("R-%d:%d:%d", reportedUserID, msgID, chatID)),
				tbapi.NewInlineKeyboardButtonData("⛔️ Забанить репортера", fmt.Sprintf("R?%d:%d:%d", reportedUserID, msgID, chatID)),
			),
		)

		editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, updText)
		editMsg.ReplyMarkup = &keyboard
		if err := send(editMsg, r.tbAPI); err != nil {
			return fmt.Errorf("failed to update notification, chatID:%d, msgID:%d, %w",
				query.Message.Chat.ID, query.Message.MessageID, err)
		}
	}

	log.Printf("[INFO] reporter %s banned by admin %s for msgID:%d", reporterName, query.From.UserName, msgID)
	return nil
}

func (r *userReports) callbackReportCancel(_ context.Context, query *tbapi.CallbackQuery) error {
	reportedUserID, msgID, callbackChatID, err := parseCallbackDataWithChat(query.Data)
	if err != nil {
		return fmt.Errorf("failed to parse callback data: %w", err)
	}
	chatID := resolveCallbackChatID(callbackChatID, r.primChatIDs)

	keyboard := tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("✅ Забанить", fmt.Sprintf("R+%d:%d:%d", reportedUserID, msgID, chatID)),
			tbapi.NewInlineKeyboardButtonData("❌ Отклонить", fmt.Sprintf("R-%d:%d:%d", reportedUserID, msgID, chatID)),
			tbapi.NewInlineKeyboardButtonData("⛔️ Забанить репортера", fmt.Sprintf("R?%d:%d:%d", reportedUserID, msgID, chatID)),
		),
	)

	editMsg := tbapi.NewEditMessageReplyMarkup(
		query.Message.Chat.ID,
		query.Message.MessageID,
		keyboard,
	)
	if _, err := r.tbAPI.Send(editMsg); err != nil {
		return fmt.Errorf("failed to restore keyboard, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}

	log.Printf("[INFO] ban reporter canceled by admin %s for msgID:%d", query.From.UserName, msgID)
	return nil
}

func (r *userReports) HandleReportCallback(ctx context.Context, query *tbapi.CallbackQuery) error {
	chatID := query.Message.Chat.ID
	if chatID != r.adminChatID {
		return nil
	}

	callbackData := query.Data
	if len(callbackData) < 3 {
		return fmt.Errorf("invalid callback data: %s", callbackData)
	}

	switch callbackData[:2] {
	case "R+":
		return r.callbackReportBan(ctx, query)
	case "R-":
		return r.callbackReportReject(ctx, query)
	case "R?":
		return r.callbackReportBanReporterAsk(ctx, query)
	case "R!":
		return r.callbackReportBanReporterConfirm(ctx, query)
	case "RX":
		return r.callbackReportCancel(ctx, query)
	default:
		return fmt.Errorf("unknown report callback: %s", callbackData)
	}
}
