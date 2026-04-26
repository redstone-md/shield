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

func TestUserReports_CallbackReportBan(t *testing.T) {
	t.Run("uses shared action executor for delete and ban", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam msg"},
				}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error {
				return nil
			},
		}

		mockBot := &mocks.BotMock{
			RemoveApprovedUserFunc: func(userID int64) error { return nil },
			UpdateSpamFunc:         func(msg string) error { return nil },
		}
		actions := &actionExecutorSpy{}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
			bot:          mockBot,
			actions:      actions,
		}

		query := &tbapi.CallbackQuery{
			Data: "R+666:100",
			From: &tbapi.User{UserName: "admin"},
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 999,
				Text:      "**User spam reported (1 reports)**",
				Date:      int(time.Now().Unix()),
			},
		}

		err := rep.callbackReportBan(context.Background(), query)
		require.NoError(t, err)
		require.Len(t, actions.deleteMessageCalls, 1)
		require.Len(t, actions.banCalls, 1)
		assert.Equal(t, int64(200), actions.deleteMessageCalls[0].ChatID)
		assert.Equal(t, 100, actions.deleteMessageCalls[0].MsgID)
		assert.Equal(t, int64(666), actions.banCalls[0].userID)
		assert.Equal(t, int64(200), actions.banCalls[0].chatID)
	})

	t.Run("successful ban approval", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam msg"},
				}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error {
				return nil
			},
		}

		mockBot := &mocks.BotMock{
			RemoveApprovedUserFunc: func(userID int64) error { return nil },
			UpdateSpamFunc:         func(msg string) error { return nil },
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
			bot:          mockBot,
		}

		query := &tbapi.CallbackQuery{
			Data: "R+666:100",
			From: &tbapi.User{UserName: "admin"},
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 999,
				Text:      "**User spam reported (1 reports)**",
				Date:      int(time.Now().Unix()),
			},
		}

		err := rep.callbackReportBan(context.Background(), query)
		require.NoError(t, err)
		assert.Len(t, mockReports.GetByMessageCalls(), 1)
		assert.Len(t, mockReports.DeleteByMessageCalls(), 1)
		assert.Len(t, mockBot.UpdateSpamCalls(), 1)
	})

	t.Run("no reports found", func(t *testing.T) {
		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{}, nil
			},
		}

		rep := &userReports{
			adminChatID:  456,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		query := &tbapi.CallbackQuery{
			Data: "R+666:100",
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: 456},
			},
		}

		err := rep.callbackReportBan(context.Background(), query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no reports found")
	})

	t.Run("soft-ban mode uses restrict instead of ban", func(t *testing.T) {
		var banReqReceived banRequest
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {

				switch req := c.(type) {
				case tbapi.RestrictChatMemberConfig:
					banReqReceived.restrict = true
					banReqReceived.userID = req.UserID
				case tbapi.BanChatMemberConfig:
					banReqReceived.restrict = false
					banReqReceived.userID = req.UserID
				}
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		mockBot := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			UpdateSpamFunc:         func(msg string) error { return nil },
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
				}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error { return nil },
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			bot:         mockBot,
			primChatID:  200,
			softBanMode: true,
			ReportConfig: ReportConfig{
				Storage: mockReports,
			},
		}

		query := &tbapi.CallbackQuery{
			Data: "R+666:100",
			From: &tbapi.User{UserName: "admin"},
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 999,
				Text:      "User spam reported",
			},
		}

		err := rep.callbackReportBan(context.Background(), query)
		require.NoError(t, err)
		assert.True(t, banReqReceived.restrict, "should use restrict mode in soft-ban")
		assert.Equal(t, int64(666), banReqReceived.userID, "should ban correct user")
	})

	t.Run("normal mode uses permanent ban", func(t *testing.T) {
		var banReqReceived banRequest
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {

				switch req := c.(type) {
				case tbapi.RestrictChatMemberConfig:
					banReqReceived.restrict = true
					banReqReceived.userID = req.UserID
				case tbapi.BanChatMemberConfig:
					banReqReceived.restrict = false
					banReqReceived.userID = req.UserID
				}
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		mockBot := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			UpdateSpamFunc:         func(msg string) error { return nil },
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: 100, ChatID: 200, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
				}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error { return nil },
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			bot:         mockBot,
			primChatID:  200,
			softBanMode: false,
			ReportConfig: ReportConfig{
				Storage: mockReports,
			},
		}

		query := &tbapi.CallbackQuery{
			Data: "R+666:100",
			From: &tbapi.User{UserName: "admin"},
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 999,
				Text:      "User spam reported",
			},
		}

		err := rep.callbackReportBan(context.Background(), query)
		require.NoError(t, err)
		assert.False(t, banReqReceived.restrict, "should use ban mode when soft-ban disabled")
		assert.Equal(t, int64(666), banReqReceived.userID, "should ban correct user")
	})
}

func TestUserReports_ExecuteAutoBanUsesSharedActionExecutor(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{MessageID: 123}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	mockBot := &mocks.BotMock{
		RemoveApprovedUserFunc: func(id int64) error { return nil },
		UpdateSpamFunc:         func(msg string) error { return nil },
	}
	mockReports := &mocks.ReportsMock{
		DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error { return nil },
	}
	actions := &actionExecutorSpy{}

	rep := &userReports{
		tbAPI:       mockAPI,
		bot:         mockBot,
		actions:     actions,
		primChatID:  200,
		adminChatID: 456,
		softBanMode: true,
		ReportConfig: ReportConfig{
			Storage:          mockReports,
			Threshold:        2,
			AutoBanThreshold: 3,
		},
	}

	err := rep.executeAutoBan(context.Background(), []storage.Report{
		{MsgID: 100, ChatID: 200, ReporterUserID: 111, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
		{MsgID: 100, ChatID: 200, ReporterUserID: 222, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
		{MsgID: 100, ChatID: 200, ReporterUserID: 333, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
	})
	require.NoError(t, err)
	require.Len(t, actions.deleteMessageCalls, 1)
	require.Len(t, actions.banCalls, 1)
	assert.Equal(t, int64(200), actions.deleteMessageCalls[0].ChatID)
	assert.Equal(t, 100, actions.deleteMessageCalls[0].MsgID)
	assert.True(t, actions.banCalls[0].restrict)
	assert.Equal(t, int64(666), actions.banCalls[0].userID)
}

func TestUserReports_CallbackReportReject(t *testing.T) {
	t.Run("successful reject", func(t *testing.T) {
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
				Text:      "**User spam reported (1 reports)**",
				Date:      int(time.Now().Unix()),
			},
		}

		err := rep.callbackReportReject(context.Background(), query)
		require.NoError(t, err)
		assert.Len(t, mockReports.GetByMessageCalls(), 1)
		assert.Len(t, mockReports.DeleteByMessageCalls(), 1)
	})

	t.Run("no reports found", func(t *testing.T) {
		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{}, nil
			},
		}

		rep := &userReports{
			adminChatID:  456,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		query := &tbapi.CallbackQuery{
			Data: "R-666:100",
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: 456},
			},
		}

		err := rep.callbackReportReject(context.Background(), query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no reports found")
	})
}

func TestUserReports_CallbackReportBanReporterAsk(t *testing.T) {
	t.Run("show confirmation with multiple reporters", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				if editMarkup, ok := c.(tbapi.EditMessageReplyMarkupConfig); ok {
					assert.Equal(t, int64(456), editMarkup.ChatID)
					assert.Equal(t, 999, editMarkup.MessageID)
					require.Len(t, editMarkup.ReplyMarkup.InlineKeyboard, 3)
					assert.Equal(t, "Ban reporter1", editMarkup.ReplyMarkup.InlineKeyboard[0][0].Text)
					assert.Equal(t, "R!111:100", *editMarkup.ReplyMarkup.InlineKeyboard[0][0].CallbackData)
					assert.Equal(t, "Ban reporter2", editMarkup.ReplyMarkup.InlineKeyboard[1][0].Text)
					assert.Equal(t, "R!222:100", *editMarkup.ReplyMarkup.InlineKeyboard[1][0].CallbackData)
					assert.Equal(t, "Cancel", editMarkup.ReplyMarkup.InlineKeyboard[2][0].Text)
					assert.Equal(t, "RX666:100", *editMarkup.ReplyMarkup.InlineKeyboard[2][0].CallbackData)
				}
				return tbapi.Message{}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: 100, ChatID: 200, ReporterUserID: 111, ReporterUserName: "reporter1"},
					{MsgID: 100, ChatID: 200, ReporterUserID: 222, ReporterUserName: "reporter2"},
				}, nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			primChatID:   200,
			ReportConfig: ReportConfig{Storage: mockReports},
		}

		query := &tbapi.CallbackQuery{
			Data:    "R?666:100",
			Message: &tbapi.Message{Chat: tbapi.Chat{ID: 456}, MessageID: 999},
		}

		err := rep.callbackReportBanReporterAsk(context.Background(), query)
		require.NoError(t, err)
		assert.Len(t, mockReports.GetByMessageCalls(), 1)
		assert.Len(t, mockAPI.SendCalls(), 1)
	})

	t.Run("no reports found", func(t *testing.T) {
		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{}, nil
			},
		}

		rep := &userReports{primChatID: 200, ReportConfig: ReportConfig{Storage: mockReports}}
		query := &tbapi.CallbackQuery{Data: "R?666:100", Message: &tbapi.Message{Chat: tbapi.Chat{ID: 456}}}

		err := rep.callbackReportBanReporterAsk(context.Background(), query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no reports found")
	})
}
