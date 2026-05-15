package events

import (
	"context"
	"fmt"
	"log"
	"strings"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/storage"
)

func (r *userReports) sendAutoBanNotification(reports []storage.Report) error {
	if len(reports) == 0 {
		return fmt.Errorf("no reports provided")
	}

	firstReport := reports[0]
	reportedUserID := firstReport.ReportedUserID
	reportedUserName := firstReport.ReportedUserName

	msgText := strings.ReplaceAll(escapeMarkDownV1Text(firstReport.MsgText), "\n", " ")
	msgText = truncateString(msgText, 200, "...")

	reporterList := make([]string, 0, len(reports))
	for _, report := range reports {
		reporterName := report.ReporterUserName
		if reporterName == "" {
			reporterName = fmt.Sprintf("user%d", report.ReporterUserID)
		}
		reporterList = append(reporterList, fmt.Sprintf("- [%s](tg://user?id=%d)",
			escapeMarkDownV1Text(reporterName), report.ReporterUserID))
	}

	actionType := "забанен"
	if r.softBanMode {
		actionType = "ограничен"
	}

	notificationText := fmt.Sprintf(
		"**Пользователь автоматически %s после %d репортов**\n\n[%s](tg://user?id=%d)\n\n"+
			"%s\n\n**Кто пожаловался:**\n%s",
		actionType,
		len(reports),
		escapeMarkDownV1Text(reportedUserName),
		reportedUserID,
		msgText,
		strings.Join(reporterList, "\n"))

	tbMsg := tbapi.NewMessage(r.adminChatID, notificationText)
	tbMsg.ParseMode = tbapi.ModeMarkdown
	tbMsg.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}

	if _, err := r.tbAPI.Send(tbMsg); err != nil {
		return fmt.Errorf("failed to send auto-ban notification: %w", err)
	}

	log.Printf("[INFO] auto-ban notification sent to admin chat for user %d", reportedUserID)
	return nil
}

func (r *userReports) updateNotificationForAutoBan(reports []storage.Report) error {
	if len(reports) == 0 {
		return fmt.Errorf("reports list is empty")
	}

	firstReport := reports[0]
	adminMsgID := firstReport.AdminMsgID
	if adminMsgID == 0 {
		return fmt.Errorf("admin message ID is 0, cannot update")
	}

	reportedUserID := firstReport.ReportedUserID
	reportedUserName := firstReport.ReportedUserName

	msgText := strings.ReplaceAll(escapeMarkDownV1Text(firstReport.MsgText), "\n", " ")
	msgText = truncateString(msgText, 200, "...")

	reporterList := make([]string, 0, len(reports))
	for _, report := range reports {
		reporterName := report.ReporterUserName
		if reporterName == "" {
			reporterName = fmt.Sprintf("user%d", report.ReporterUserID)
		}
		reporterList = append(reporterList, fmt.Sprintf("- [%s](tg://user?id=%d)",
			escapeMarkDownV1Text(reporterName), report.ReporterUserID))
	}

	actionType := "забанен"
	if r.softBanMode {
		actionType = "ограничен"
	}

	updatedText := fmt.Sprintf("**Жалобы на спам (%d)**\n\n[%s](tg://user?id=%d)\n\n%s\n\n"+
		"**Кто пожаловался:**\n%s\n\n_автоматически %s после %d репортов_",
		len(reports),
		escapeMarkDownV1Text(reportedUserName),
		reportedUserID,
		msgText,
		strings.Join(reporterList, "\n"),
		actionType,
		len(reports))

	editMsg := tbapi.NewEditMessageText(r.adminChatID, adminMsgID, updatedText)
	editMsg.ParseMode = "Markdown"
	editMsg.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: [][]tbapi.InlineKeyboardButton{}}

	if _, err := r.tbAPI.Send(editMsg); err != nil {
		return fmt.Errorf("failed to update notification for auto-ban (msgID:%d, adminMsgID:%d): %w",
			firstReport.MsgID, adminMsgID, err)
	}

	log.Printf("[INFO] updated admin notification %d for auto-ban of user %d", adminMsgID, reportedUserID)
	return nil
}

func (r *userReports) sendReportNotification(ctx context.Context, reports []storage.Report) error {
	if len(reports) == 0 {
		return fmt.Errorf("no reports provided")
	}
	if r.adminChatID == 0 {
		log.Printf("[DEBUG] admin chat not configured, skipping notification")
		return nil
	}

	firstReport := reports[0]
	msgID := firstReport.MsgID
	chatID := firstReport.ChatID
	reportedUserID := firstReport.ReportedUserID
	reportedUserName := firstReport.ReportedUserName

	msgText := strings.ReplaceAll(escapeMarkDownV1Text(firstReport.MsgText), "\n", " ")
	msgText = truncateString(msgText, 200, "...")

	reporterList := make([]string, 0, len(reports))
	for _, report := range reports {
		reporterName := report.ReporterUserName
		if reporterName == "" {
			reporterName = fmt.Sprintf("user%d", report.ReporterUserID)
		}
		reporterList = append(reporterList, fmt.Sprintf("- [%s](tg://user?id=%d)",
			escapeMarkDownV1Text(reporterName), report.ReporterUserID))
	}

	notificationText := fmt.Sprintf("**Жалобы на спам (%d)**\n\n[%s](tg://user?id=%d)\n\n%s\n\n**Кто пожаловался:**\n%s",
		len(reports),
		escapeMarkDownV1Text(reportedUserName),
		reportedUserID,
		msgText,
		strings.Join(reporterList, "\n"))

	padding := strings.Repeat("\u2800", 30)
	notificationText += "\n\n" + padding

	keyboard := tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("✅ Забанить", fmt.Sprintf("R+%d:%d", reportedUserID, msgID)),
			tbapi.NewInlineKeyboardButtonData("❌ Отклонить", fmt.Sprintf("R-%d:%d", reportedUserID, msgID)),
			tbapi.NewInlineKeyboardButtonData("⛔️ Забанить репортера", fmt.Sprintf("R?%d:%d", reportedUserID, msgID)),
		),
	)

	tbMsg := tbapi.NewMessage(r.adminChatID, notificationText)
	tbMsg.ParseMode = tbapi.ModeMarkdown
	tbMsg.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	tbMsg.ReplyMarkup = keyboard

	resp, err := r.tbAPI.Send(tbMsg)
	if err != nil {
		return fmt.Errorf("failed to send notification to admin chat: %w", err)
	}

	if err := r.Storage.UpdateAdminMsgID(ctx, msgID, chatID, resp.MessageID); err != nil {
		log.Printf("[WARN] failed to update admin message ID for msgID:%d: %v", msgID, err)
	}

	log.Printf("[INFO] user report notification sent to admin chat: msgID:%d, reported:%s (%d), %d reports",
		msgID, reportedUserName, reportedUserID, len(reports))
	return nil
}

func (r *userReports) updateReportNotification(_ context.Context, reports []storage.Report) error {
	if len(reports) == 0 {
		return fmt.Errorf("reports list is empty")
	}

	if r.adminChatID == 0 {
		log.Printf("[DEBUG] admin chat not configured, skipping report notification update")
		return nil
	}

	firstReport := reports[0]
	adminMsgID := firstReport.AdminMsgID
	msgID := firstReport.MsgID
	chatID := firstReport.ChatID
	reportedUserID := firstReport.ReportedUserID
	reportedUserName := firstReport.ReportedUserName

	msgText := strings.ReplaceAll(escapeMarkDownV1Text(firstReport.MsgText), "\n", " ")
	msgText = truncateString(msgText, 200, "...")

	reporterList := make([]string, 0, len(reports))
	for _, report := range reports {
		reporterName := report.ReporterUserName
		if reporterName == "" {
			reporterName = fmt.Sprintf("user%d", report.ReporterUserID)
		}
		reporterList = append(reporterList, fmt.Sprintf("- [%s](tg://user?id=%d)",
			escapeMarkDownV1Text(reporterName), report.ReporterUserID))
	}

	notification := fmt.Sprintf("**Жалобы на спам (%d)**\n\n", len(reports)) +
		fmt.Sprintf("[%s](tg://user?id=%d)\n\n", escapeMarkDownV1Text(reportedUserName), reportedUserID) +
		fmt.Sprintf("%s\n\n", msgText) +
		fmt.Sprintf("**Кто пожаловался:**\n%s", strings.Join(reporterList, "\n"))

	padding := strings.Repeat("\u2800", 30)
	notification += "\n\n" + padding

	keyboard := tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("✅ Забанить", fmt.Sprintf("R+%d:%d", reportedUserID, msgID)),
			tbapi.NewInlineKeyboardButtonData("❌ Отклонить", fmt.Sprintf("R-%d:%d", reportedUserID, msgID)),
			tbapi.NewInlineKeyboardButtonData("⛔️ Забанить репортера", fmt.Sprintf("R?%d:%d", reportedUserID, msgID)),
		),
	)

	editMsg := tbapi.NewEditMessageText(r.adminChatID, adminMsgID, notification)
	editMsg.ParseMode = "Markdown"
	editMsg.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	editMsg.ReplyMarkup = &keyboard

	if _, err := r.tbAPI.Send(editMsg); err != nil {
		return fmt.Errorf("failed to edit admin notification for msgID:%d, chatID:%d: %w", msgID, chatID, err)
	}

	log.Printf("[INFO] updated report notification for msgID:%d (reported user:%d, %d reports total, admin_msg_id:%d)",
		msgID, reportedUserID, len(reports), adminMsgID)
	return nil
}
