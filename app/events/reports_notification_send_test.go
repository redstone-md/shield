package events

import (
	"context"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestUserReports_SendReportNotification(t *testing.T) {
	t.Run("successful notification with single report", func(t *testing.T) {
		var sentMsg tbapi.MessageConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.MessageConfig)
				sentMsg = msg
				return tbapi.Message{MessageID: 999}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			UpdateAdminMsgIDFunc: func(ctx context.Context, msgID int, chatID int64, adminMsgID int) error {
				assert.Equal(t, 100, msgID)
				assert.Equal(t, int64(200), chatID)
				assert.Equal(t, 999, adminMsgID)
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: "spam message"},
		}

		err := rep.sendReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Len(t, mockAPI.SendCalls(), 1, "should send message")
		assert.Len(t, mockReports.UpdateAdminMsgIDCalls(), 1, "should update admin msg ID")
		assert.Equal(t, int64(456), sentMsg.ChatID, "should send to admin chat")
		assert.Equal(t, tbapi.ModeHTML, sentMsg.ParseMode, "should use HTML")
		assert.Contains(t, sentMsg.Text, "Жалобы на спам (1)", "should contain report count")
		assert.Contains(t, sentMsg.Text, "spammer", "should contain reported user name")
		assert.Contains(t, sentMsg.Text, "spam message", "should contain message text")
		assert.Contains(t, sentMsg.Text, "reporter1", "should contain reporter name")

		keyboard, ok := sentMsg.ReplyMarkup.(tbapi.InlineKeyboardMarkup)
		require.True(t, ok, "should have inline keyboard")
		require.Len(t, keyboard.InlineKeyboard, 1, "should have 1 row")
		require.Len(t, keyboard.InlineKeyboard[0], 3, "row should have 3 buttons")
		assert.Equal(t, "✅ Забанить", keyboard.InlineKeyboard[0][0].Text)
		assert.Equal(t, "R+666:100:200", *keyboard.InlineKeyboard[0][0].CallbackData)
		assert.Equal(t, "❌ Отклонить", keyboard.InlineKeyboard[0][1].Text)
		assert.Equal(t, "R-666:100:200", *keyboard.InlineKeyboard[0][1].CallbackData)
		assert.Equal(t, "⛔️ Забанить репортера", keyboard.InlineKeyboard[0][2].Text)
		assert.Equal(t, "R?666:100:200", *keyboard.InlineKeyboard[0][2].CallbackData)
	})

	t.Run("successful notification with multiple reports", func(t *testing.T) {
		var sentMsg tbapi.MessageConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.MessageConfig)
				sentMsg = msg
				return tbapi.Message{MessageID: 999}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			UpdateAdminMsgIDFunc: func(ctx context.Context, msgID int, chatID int64, adminMsgID int) error {
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: "spam message"},
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 222, ReporterUserName: "reporter2", MsgText: "spam message"},
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 333, ReporterUserName: "reporter3", MsgText: "spam message"},
		}

		err := rep.sendReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Contains(t, sentMsg.Text, "Жалобы на спам (3)", "should contain report count")
		assert.Contains(t, sentMsg.Text, "reporter1", "should contain first reporter")
		assert.Contains(t, sentMsg.Text, "reporter2", "should contain second reporter")
		assert.Contains(t, sentMsg.Text, "reporter3", "should contain third reporter")
	})

	t.Run("long message text should be truncated", func(t *testing.T) {
		var sentMsg tbapi.MessageConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.MessageConfig)
				sentMsg = msg
				return tbapi.Message{MessageID: 999}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			UpdateAdminMsgIDFunc: func(ctx context.Context, msgID int, chatID int64, adminMsgID int) error {
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		longMsg := strings.Repeat("spam ", 100)
		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: longMsg},
		}

		err := rep.sendReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Contains(t, sentMsg.Text, "...", "should truncate long message")
	})

	t.Run("reporter without username - should use user ID", func(t *testing.T) {
		var sentMsg tbapi.MessageConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.MessageConfig)
				sentMsg = msg
				return tbapi.Message{MessageID: 999}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			UpdateAdminMsgIDFunc: func(ctx context.Context, msgID int, chatID int64, adminMsgID int) error {
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "", MsgText: "spam"},
		}

		err := rep.sendReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Contains(t, sentMsg.Text, "user111", "should use userID as fallback")
	})

	t.Run("admin chat not configured - should skip notification", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{}
		mockReports := &mocks.ReportsMock{}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  0,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: "spam"},
		}

		err := rep.sendReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Empty(t, mockAPI.SendCalls(), "should not send message")
	})

	t.Run("empty reports list - should return error", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{}
		mockReports := &mocks.ReportsMock{}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		err := rep.sendReportNotification(context.Background(), []storage.Report{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no reports provided")
	})

	t.Run("send error - should return error", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, fmt.Errorf("network error")
			},
		}

		mockReports := &mocks.ReportsMock{}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: "spam"},
		}

		err := rep.sendReportNotification(context.Background(), reports)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send notification")
		assert.Contains(t, err.Error(), "network error")
	})

	t.Run("UpdateAdminMsgID error - should not fail", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{MessageID: 999}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			UpdateAdminMsgIDFunc: func(ctx context.Context, msgID int, chatID int64, adminMsgID int) error {
				return fmt.Errorf("database error")
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: "spam"},
		}

		err := rep.sendReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Len(t, mockAPI.SendCalls(), 1, "should still send message")
		assert.Len(t, mockReports.UpdateAdminMsgIDCalls(), 1, "should attempt to update admin msg ID")
	})

	t.Run("names with html special characters should be escaped", func(t *testing.T) {
		var sentMsg tbapi.MessageConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.MessageConfig)
				sentMsg = msg
				return tbapi.Message{MessageID: 999}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			UpdateAdminMsgIDFunc: func(ctx context.Context, msgID int, chatID int64, adminMsgID int) error {
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spam<user>&bot", ReporterUserID: 111, ReporterUserName: "test_reporter", MsgText: "spam"},
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spam<user>&bot", ReporterUserID: 222, ReporterUserName: "admin\"test\"", MsgText: "spam"},
		}

		err := rep.sendReportNotification(context.Background(), reports)
		require.NoError(t, err)

		assert.Contains(t, sentMsg.Text, "spam&lt;user&gt;&amp;bot", "reported user name should be escaped")
		assert.NotContains(t, sentMsg.Text, "spam<user>&bot", "reported user name should not contain raw html specials")

		assert.Contains(t, sentMsg.Text, `<a href="tg://user?id=111">test_reporter</a>`, "first reporter link must stay well-formed")
		assert.Contains(t, sentMsg.Text, `admin&quot;test&quot;`, "second reporter name should have escaped quotes")
	})

	t.Run("notification should include padding for full-width buttons", func(t *testing.T) {
		var sentMsg tbapi.MessageConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.MessageConfig)
				sentMsg = msg
				return tbapi.Message{MessageID: 999}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			UpdateAdminMsgIDFunc: func(ctx context.Context, msgID int, chatID int64, adminMsgID int) error {
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: "short spam"},
		}

		err := rep.sendReportNotification(context.Background(), reports)
		require.NoError(t, err)

		assert.Contains(t, sentMsg.Text, "\u2800", "should contain U+2800 padding for full-width buttons")

		assert.True(t, strings.HasSuffix(sentMsg.Text, "\n\n"+strings.Repeat("\u2800", 30)), "padding should be at the end")
	})
}
