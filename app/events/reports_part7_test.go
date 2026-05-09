package events

import (
	"context"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/app/storage"
	"testing"
	"time"
)

func TestUserReports_CallbackReportBanReporterConfirm(t *testing.T) {
	t.Run("ban reporter with remaining reporters", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		var callCount int
		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				callCount++
				if callCount == 1 {
					return []storage.Report{
						{MsgID: 100, ChatID: 200, ReporterUserID: 111, ReporterUserName: "reporter_1", ReportedUserID: 666, ReportedUserName: "spammer"},
						{MsgID: 100, ChatID: 200, ReporterUserID: 222, ReporterUserName: "reporter2", ReportedUserID: 666, ReportedUserName: "spammer"},
					}, nil
				}
				return []storage.Report{
					{MsgID: 100, ChatID: 200, ReporterUserID: 222, ReporterUserName: "reporter2", ReportedUserID: 666, ReportedUserName: "spammer"},
				}, nil
			},
			DeleteReporterFunc: func(ctx context.Context, reporterID int64, msgID int, chatID int64) error {
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		query := &tbapi.CallbackQuery{
			Data:    "R!111:100",
			From:    &tbapi.User{UserName: "admin", ID: 333},
			Message: &tbapi.Message{Chat: tbapi.Chat{ID: 456}, MessageID: 999, Text: "Test", Date: int(time.Now().Unix())},
		}

		err := rep.callbackReportBanReporterConfirm(context.Background(), query)
		require.NoError(t, err)
		assert.Len(t, mockReports.GetByMessageCalls(), 2)
		assert.Len(t, mockReports.DeleteReporterCalls(), 1)
		require.Len(t, mockAPI.SendCalls(), 1)
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, "репортер [reporter\\_1](tg://user?id=111) забанен администратором [admin](tg://user?id=333)")
	})

	t.Run("ban last reporter", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		var callCount int
		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				callCount++
				if callCount == 1 {
					return []storage.Report{
						{MsgID: 100, ChatID: 200, ReporterUserID: 111, ReporterUserName: "reporter1", ReportedUserID: 666, ReportedUserName: "spammer"},
					}, nil
				}
				return []storage.Report{}, nil
			},
			DeleteReporterFunc: func(ctx context.Context, reporterID int64, msgID int, chatID int64) error {
				return nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error {
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		query := &tbapi.CallbackQuery{
			Data:    "R!111:100",
			From:    &tbapi.User{UserName: "admin"},
			Message: &tbapi.Message{Chat: tbapi.Chat{ID: 456}, MessageID: 999, Text: "Test", Date: int(time.Now().Unix())},
		}

		err := rep.callbackReportBanReporterConfirm(context.Background(), query)
		require.NoError(t, err)
		assert.Len(t, mockReports.GetByMessageCalls(), 2)
		assert.Len(t, mockReports.DeleteReporterCalls(), 1)
		assert.Len(t, mockReports.DeleteByMessageCalls(), 1)
	})

	t.Run("uses shared action executor", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		var callCount int
		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				callCount++
				if callCount == 1 {
					return []storage.Report{
						{MsgID: 100, ChatID: 200, ReporterUserID: 111, ReporterUserName: "reporter1", ReportedUserID: 666, ReportedUserName: "spammer"},
						{MsgID: 100, ChatID: 200, ReporterUserID: 222, ReporterUserName: "reporter2", ReportedUserID: 666, ReportedUserName: "spammer"},
					}, nil
				}
				return []storage.Report{
					{MsgID: 100, ChatID: 200, ReporterUserID: 222, ReporterUserName: "reporter2", ReportedUserID: 666, ReportedUserName: "spammer"},
				}, nil
			},
			DeleteReporterFunc: func(ctx context.Context, reporterID int64, msgID int, chatID int64) error {
				return nil
			},
		}
		actions := &actionExecutorSpy{}

		rep := &userReports{
			tbAPI:        mockAPI,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
			actions:      actions,
		}

		query := &tbapi.CallbackQuery{
			Data:    "R!111:100",
			From:    &tbapi.User{UserName: "admin"},
			Message: &tbapi.Message{Chat: tbapi.Chat{ID: 456}, MessageID: 999, Text: "Test", Date: int(time.Now().Unix())},
		}

		err := rep.callbackReportBanReporterConfirm(context.Background(), query)
		require.NoError(t, err)
		require.Len(t, actions.banCalls, 1)
		assert.Equal(t, int64(111), actions.banCalls[0].userID)
		assert.Equal(t, int64(200), actions.banCalls[0].chatID)
		assert.Len(t, mockReports.DeleteReporterCalls(), 1)
	})
}

func TestUserReports_CallbackReportCancel(t *testing.T) {
	t.Run("restore original buttons", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				if editMarkup, ok := c.(tbapi.EditMessageReplyMarkupConfig); ok {
					assert.Equal(t, int64(456), editMarkup.ChatID)
					assert.Equal(t, 999, editMarkup.MessageID)
					require.Len(t, editMarkup.ReplyMarkup.InlineKeyboard, 1, "should have 1 row of buttons")
					require.Len(t, editMarkup.ReplyMarkup.InlineKeyboard[0], 3, "row should have 3 buttons")
					assert.Equal(t, "✅ Забанить", editMarkup.ReplyMarkup.InlineKeyboard[0][0].Text)
					assert.Equal(t, "R+666:100", *editMarkup.ReplyMarkup.InlineKeyboard[0][0].CallbackData)
					assert.Equal(t, "❌ Отклонить", editMarkup.ReplyMarkup.InlineKeyboard[0][1].Text)
					assert.Equal(t, "R-666:100", *editMarkup.ReplyMarkup.InlineKeyboard[0][1].CallbackData)
					assert.Equal(t, "⛔️ Забанить репортера", editMarkup.ReplyMarkup.InlineKeyboard[0][2].Text)
					assert.Equal(t, "R?666:100", *editMarkup.ReplyMarkup.InlineKeyboard[0][2].CallbackData)
				}
				return tbapi.Message{}, nil
			},
		}

		rep := &userReports{tbAPI: mockAPI}

		query := &tbapi.CallbackQuery{
			Data:    "RX666:100",
			From:    &tbapi.User{UserName: "admin"},
			Message: &tbapi.Message{Chat: tbapi.Chat{ID: 456}, MessageID: 999},
		}

		err := rep.callbackReportCancel(context.Background(), query)
		require.NoError(t, err)
		assert.Len(t, mockAPI.SendCalls(), 1)
	})
}

func TestUserReports_HandleReportCallback_SecurityValidation(t *testing.T) {
	t.Run("callback from admin chat should be processed", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer"},
				}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error {
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		query := &tbapi.CallbackQuery{
			Data: "R-666:100",
			From: &tbapi.User{UserName: "admin"},
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 999,
				Text:      "User spam reported",
				Date:      int(time.Now().Unix()),
			},
		}

		err := rep.HandleReportCallback(context.Background(), query)
		require.NoError(t, err)

		assert.Len(t, mockReports.GetByMessageCalls(), 1)
		assert.Len(t, mockReports.DeleteByMessageCalls(), 1)
	})

	t.Run("callback from non-admin chat should be rejected", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				t.Fatal("should not call Send for unauthorized callback")
				return tbapi.Message{}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				t.Fatal("should not call GetByMessage for unauthorized callback")
				return nil, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error {
				t.Fatal("should not call DeleteByMessage for unauthorized callback")
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		query := &tbapi.CallbackQuery{
			Data: "R-666:100",
			From: &tbapi.User{UserName: "attacker"},
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 789},
				MessageID: 999,
				Text:      "User spam reported",
				Date:      int(time.Now().Unix()),
			},
		}

		err := rep.HandleReportCallback(context.Background(), query)
		require.NoError(t, err)

		assert.Empty(t, mockReports.GetByMessageCalls())
		assert.Empty(t, mockReports.DeleteByMessageCalls())
		assert.Empty(t, mockAPI.SendCalls())
	})

	t.Run("R+ callback from non-admin chat should be rejected", func(t *testing.T) {
		mockBot := &mocks.BotMock{
			RemoveApprovedUserFunc: func(userID int64) error {
				t.Fatal("should not call RemoveApprovedUser for unauthorized callback")
				return nil
			},
			UpdateSpamFunc: func(msg string) error {
				t.Fatal("should not call UpdateSpam for unauthorized callback")
				return nil
			},
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				t.Fatal("should not call GetByMessage for unauthorized callback")
				return nil, nil
			},
		}

		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				t.Fatal("should not call Request (ban) for unauthorized callback")
				return nil, nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			bot:          mockBot,
			adminChatID:  456,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		query := &tbapi.CallbackQuery{
			Data: "R+666:100",
			From: &tbapi.User{UserName: "attacker"},
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 123},
				MessageID: 999,
				Text:      "fake report",
				Date:      int(time.Now().Unix()),
			},
		}

		err := rep.HandleReportCallback(context.Background(), query)
		require.NoError(t, err)

		assert.Empty(t, mockBot.RemoveApprovedUserCalls())
		assert.Empty(t, mockBot.UpdateSpamCalls())
		assert.Empty(t, mockReports.GetByMessageCalls())
		assert.Empty(t, mockAPI.RequestCalls())
	})

	t.Run("R! callback from non-admin chat should be rejected", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				t.Fatal("should not ban reporter for unauthorized callback")
				return nil, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				t.Fatal("should not call GetByMessage for unauthorized callback")
				return nil, nil
			},
			DeleteReporterFunc: func(ctx context.Context, reporterID int64, msgID int, chatID int64) error {
				t.Fatal("should not call DeleteReporter for unauthorized callback")
				return nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		query := &tbapi.CallbackQuery{
			Data: "R!111:100",
			From: &tbapi.User{UserName: "attacker"},
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 999},
				MessageID: 888,
				Text:      "fake report",
				Date:      int(time.Now().Unix()),
			},
		}

		err := rep.HandleReportCallback(context.Background(), query)
		require.NoError(t, err)

		assert.Empty(t, mockReports.GetByMessageCalls())
		assert.Empty(t, mockReports.DeleteReporterCalls())
		assert.Empty(t, mockAPI.RequestCalls())
	})

	t.Run("callback with invalid data format should return error", func(t *testing.T) {
		rep := &userReports{
			adminChatID: 456,
			primChatID:  200,
		}

		query := &tbapi.CallbackQuery{
			Data: "R+",
			From: &tbapi.User{UserName: "admin"},
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 999,
				Date:      int(time.Now().Unix()),
			},
		}

		err := rep.HandleReportCallback(context.Background(), query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid callback data")
	})

	t.Run("callback with unknown prefix should return error", func(t *testing.T) {
		rep := &userReports{
			adminChatID: 456,
			primChatID:  200,
		}

		query := &tbapi.CallbackQuery{
			Data: "RZ666:100",
			From: &tbapi.User{UserName: "admin"},
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 999,
				Date:      int(time.Now().Unix()),
			},
		}

		err := rep.HandleReportCallback(context.Background(), query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown report callback")
	})
}
