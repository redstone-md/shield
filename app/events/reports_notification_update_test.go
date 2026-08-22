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

func TestUserReports_UpdateReportNotification(t *testing.T) {
	t.Run("successful update with multiple reporters", func(t *testing.T) {
		var editedMsg tbapi.EditMessageTextConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.EditMessageTextConfig)
				editedMsg = msg
				return tbapi.Message{MessageID: 888}, nil
			},
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			adminChatID: 456,
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: "spam message", AdminMsgID: 888},
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 222, ReporterUserName: "reporter2", MsgText: "spam message", AdminMsgID: 888},
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 333, ReporterUserName: "reporter3", MsgText: "spam message", AdminMsgID: 888},
		}

		err := rep.updateReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Len(t, mockAPI.SendCalls(), 1, "should edit message")
		assert.Equal(t, int64(456), editedMsg.ChatID, "should edit in admin chat")
		assert.Equal(t, 888, editedMsg.MessageID, "should edit correct message")
		assert.Equal(t, tbapi.ModeHTML, editedMsg.ParseMode, "should use HTML")
		assert.Contains(t, editedMsg.Text, "Жалобы на спам (3)", "should contain updated report count")
		assert.Contains(t, editedMsg.Text, "spammer", "should contain reported user name")
		assert.Contains(t, editedMsg.Text, "spam message", "should contain message text")
		assert.Contains(t, editedMsg.Text, "reporter1", "should contain first reporter")
		assert.Contains(t, editedMsg.Text, "reporter2", "should contain second reporter")
		assert.Contains(t, editedMsg.Text, "reporter3", "should contain third reporter")

		require.NotNil(t, editedMsg.ReplyMarkup, "should have inline keyboard")
		require.Len(t, editedMsg.ReplyMarkup.InlineKeyboard, 1, "should have 1 row")
		require.Len(t, editedMsg.ReplyMarkup.InlineKeyboard[0], 3, "row should have 3 buttons")
		assert.Equal(t, "✅ Забанить", editedMsg.ReplyMarkup.InlineKeyboard[0][0].Text)
		assert.Equal(t, "R+666:100:200", *editedMsg.ReplyMarkup.InlineKeyboard[0][0].CallbackData)
		assert.Equal(t, "❌ Отклонить", editedMsg.ReplyMarkup.InlineKeyboard[0][1].Text)
		assert.Equal(t, "R-666:100:200", *editedMsg.ReplyMarkup.InlineKeyboard[0][1].CallbackData)
		assert.Equal(t, "⛔️ Забанить репортера", editedMsg.ReplyMarkup.InlineKeyboard[0][2].Text)
		assert.Equal(t, "R?666:100:200", *editedMsg.ReplyMarkup.InlineKeyboard[0][2].CallbackData)
	})

	t.Run("successful update adding new reporter to existing notification", func(t *testing.T) {
		var editedMsg tbapi.EditMessageTextConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.EditMessageTextConfig)
				editedMsg = msg
				return tbapi.Message{MessageID: 888}, nil
			},
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			adminChatID: 456,
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: "spam message", AdminMsgID: 888},
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 222, ReporterUserName: "reporter2", MsgText: "spam message", AdminMsgID: 888},
		}

		err := rep.updateReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Contains(t, editedMsg.Text, "Жалобы на спам (2)", "should update to 2 reports")
		assert.Contains(t, editedMsg.Text, "reporter1", "should still contain first reporter")
		assert.Contains(t, editedMsg.Text, "reporter2", "should add second reporter")
	})

	t.Run("long message text should be truncated", func(t *testing.T) {
		var editedMsg tbapi.EditMessageTextConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.EditMessageTextConfig)
				editedMsg = msg
				return tbapi.Message{MessageID: 888}, nil
			},
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			adminChatID: 456,
		}

		longMsg := strings.Repeat("spam ", 100)
		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: longMsg, AdminMsgID: 888},
		}

		err := rep.updateReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Contains(t, editedMsg.Text, "...", "should truncate long message")
		msgStart := strings.Index(editedMsg.Text, "spam")
		msgEnd := strings.Index(editedMsg.Text[msgStart:], "<b>Кто пожаловался:</b>")
		msgTextInNotif := editedMsg.Text[msgStart : msgStart+msgEnd]
		assert.Less(t, len(msgTextInNotif), len(longMsg), "message should be shorter than original")
	})

	t.Run("reporter without username should use fallback", func(t *testing.T) {
		var editedMsg tbapi.EditMessageTextConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.EditMessageTextConfig)
				editedMsg = msg
				return tbapi.Message{MessageID: 888}, nil
			},
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			adminChatID: 456,
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "", MsgText: "spam", AdminMsgID: 888},
		}

		err := rep.updateReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Contains(t, editedMsg.Text, "user111", "should use fallback username")
	})

	t.Run("admin chat not configured should skip gracefully", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				t.Fatal("should not send message")
				return tbapi.Message{}, nil
			},
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			adminChatID: 0,
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: "spam", AdminMsgID: 888},
		}

		err := rep.updateReportNotification(context.Background(), reports)
		require.NoError(t, err)
		assert.Empty(t, mockAPI.SendCalls(), "should not send message")
	})

	t.Run("empty reports list should return error", func(t *testing.T) {
		rep := &userReports{
			adminChatID: 456,
		}

		err := rep.updateReportNotification(context.Background(), []storage.Report{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reports list is empty")
	})

	t.Run("send error should return error", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, fmt.Errorf("telegram api error")
			},
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			adminChatID: 456,
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", ReporterUserID: 111, ReporterUserName: "reporter1", MsgText: "spam", AdminMsgID: 888},
		}

		err := rep.updateReportNotification(context.Background(), reports)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to edit admin notification")
	})

	t.Run("names with html special characters should be escaped", func(t *testing.T) {
		var editedMsg tbapi.EditMessageTextConfig
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				msg := c.(tbapi.EditMessageTextConfig)
				editedMsg = msg
				return tbapi.Message{MessageID: 888}, nil
			},
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			adminChatID: 456,
		}

		reports := []storage.Report{
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spam<user>&bot", ReporterUserID: 111, ReporterUserName: "test_reporter", MsgText: "spam", AdminMsgID: 888},
			{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spam<user>&bot", ReporterUserID: 222, ReporterUserName: "admin\"test\"", MsgText: "spam", AdminMsgID: 888},
		}

		err := rep.updateReportNotification(context.Background(), reports)
		require.NoError(t, err)

		assert.Contains(t, editedMsg.Text, "spam&lt;user&gt;&amp;bot", "reported user name should be escaped")
		assert.NotContains(t, editedMsg.Text, "spam<user>&bot", "reported user name should not contain raw html specials")

		assert.Contains(t, editedMsg.Text, `<a href="tg://user?id=111">test_reporter</a>`, "first reporter link must stay well-formed")
		assert.Contains(t, editedMsg.Text, "admin&quot;test&quot;", "second reporter name should have escaped quotes")
	})
}
