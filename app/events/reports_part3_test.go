package events

import (
	"context"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/app/storage"
	"testing"
)

func TestUserReports_CheckReportThreshold(t *testing.T) {
	t.Run("threshold not reached returns accepted outcome", func(t *testing.T) {
		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{{MsgID: msgID, ChatID: chatID, ReporterUserID: 111}}, nil
			},
		}

		rep := &userReports{ReportConfig: ReportConfig{Storage: mockReports, Threshold: 3}}

		outcome, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.NoError(t, err)
		assert.Equal(t, reportOutcomeAccepted, outcome)
		assert.Len(t, mockReports.GetByMessageCalls(), 1)
	})

	t.Run("threshold reached returns review outcome and sends notification", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{SendFunc: okSend}
		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 111, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 222, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
				}, nil
			},
			UpdateAdminMsgIDFunc: func(ctx context.Context, msgID int, chatID int64, adminMsgID int) error { return nil },
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports, Threshold: 2},
		}

		outcome, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.NoError(t, err)
		assert.Equal(t, reportOutcomeReview, outcome)
		assert.Len(t, mockAPI.SendCalls(), 1)
	})

	t.Run("existing notification update returns review outcome", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{SendFunc: okSend}
		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 111, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam", NotificationSent: true, AdminMsgID: 999},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 222, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam", NotificationSent: true, AdminMsgID: 999},
				}, nil
			},
		}

		rep := &userReports{
			tbAPI:        mockAPI,
			adminChatID:  456,
			ReportConfig: ReportConfig{Storage: mockReports, Threshold: 2},
		}

		outcome, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.NoError(t, err)
		assert.Equal(t, reportOutcomeReview, outcome)
		assert.Len(t, mockAPI.SendCalls(), 1)
	})

	t.Run("reports storage not initialized returns error", func(t *testing.T) {
		rep := &userReports{ReportConfig: ReportConfig{Storage: nil, Threshold: 2}}

		outcome, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.Error(t, err)
		assert.Empty(t, outcome)
		assert.Contains(t, err.Error(), "reports storage not initialized")
	})

	t.Run("GetByMessage error returns error", func(t *testing.T) {
		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return nil, fmt.Errorf("database error")
			},
		}

		rep := &userReports{ReportConfig: ReportConfig{Storage: mockReports, Threshold: 2}}

		outcome, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.Error(t, err)
		assert.Empty(t, outcome)
		assert.Contains(t, err.Error(), "failed to get reports")
		assert.Contains(t, err.Error(), "database error")
	})
}

func TestUserReports_AutoBan(t *testing.T) {
	t.Run("auto-ban triggers at correct threshold", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{MessageID: 123}, nil
			},
		}

		mockBot := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error {
				return nil
			},
			UpdateSpamFunc: func(msg string) error {
				return nil
			},
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {

				return []storage.Report{
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 111, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 222, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 333, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 444, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 555, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
				}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error {
				return nil
			},
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			bot:         mockBot,
			primChatID:  200,
			adminChatID: 456,
			ReportConfig: ReportConfig{
				Storage:          mockReports,
				Threshold:        2,
				AutoBanThreshold: 5,
			},
		}

		_, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.NoError(t, err)
		assert.Len(t, mockBot.RemoveApprovedUserCalls(), 1, "should remove from approved list")
		assert.Len(t, mockBot.UpdateSpamCalls(), 1, "should update spam samples")
		assert.Len(t, mockReports.DeleteByMessageCalls(), 1, "should delete reports")
		assert.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 1, "should delete message")
		assert.Len(t, mockAPI.SendCalls(), 1, "should send auto-ban notification")
	})

	t.Run("auto-ban respects soft-ban mode", func(t *testing.T) {
		var banReqReceived banRequest
		mockAPI := &mocks.TbAPIMock{
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
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{MessageID: 123}, nil
			},
		}

		mockBot := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			UpdateSpamFunc:         func(msg string) error { return nil },
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 111, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 222, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 333, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
				}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error { return nil },
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			bot:         mockBot,
			primChatID:  200,
			adminChatID: 456,
			softBanMode: true,
			ReportConfig: ReportConfig{
				Storage:          mockReports,
				Threshold:        2,
				AutoBanThreshold: 3,
			},
		}

		_, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.NoError(t, err)
		assert.True(t, banReqReceived.restrict, "should use restrict mode in soft-ban")
		assert.Equal(t, int64(666), banReqReceived.userID, "should ban correct user")
	})

	t.Run("auto-ban updates existing notification when manual threshold was reached first", func(t *testing.T) {
		var editedMsgID int
		var buttonsRemoved bool

		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {

				if editMsg, ok := c.(tbapi.EditMessageTextConfig); ok {
					editedMsgID = editMsg.MessageID
					if editMsg.ReplyMarkup != nil && len(editMsg.ReplyMarkup.InlineKeyboard) == 0 {
						buttonsRemoved = true
					}
					assert.Contains(t, editMsg.Text, "автоматически забанен", "should mention auto-ban in updated text")
					assert.Contains(t, editMsg.Text, "5 репортов", "should show report count")
				}
				return tbapi.Message{MessageID: 123}, nil
			},
		}

		mockBot := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			UpdateSpamFunc:         func(msg string) error { return nil },
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {

				return []storage.Report{
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 111, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam", NotificationSent: true, AdminMsgID: 999},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 222, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam", NotificationSent: true, AdminMsgID: 999},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 333, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam", NotificationSent: true, AdminMsgID: 999},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 444, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam", NotificationSent: true, AdminMsgID: 999},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 555, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam", NotificationSent: true, AdminMsgID: 999},
				}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error { return nil },
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			bot:         mockBot,
			primChatID:  200,
			adminChatID: 456,
			ReportConfig: ReportConfig{
				Storage:          mockReports,
				Threshold:        2,
				AutoBanThreshold: 5,
			},
		}

		_, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.NoError(t, err)
		assert.Equal(t, 999, editedMsgID, "should edit existing notification")
		assert.True(t, buttonsRemoved, "should remove buttons from notification")
	})

	t.Run("manual approval still works when count < auto-ban threshold", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {

				if msg, ok := c.(tbapi.MessageConfig); ok {
					assert.NotNil(t, msg.ReplyMarkup, "should have buttons for manual approval")
				}
				return tbapi.Message{MessageID: 123}, nil
			},
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {

				return []storage.Report{
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 111, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 222, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 333, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
				}, nil
			},
			UpdateAdminMsgIDFunc: func(ctx context.Context, msgID int, chatID int64, adminMsgID int) error { return nil },
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			primChatID:  200,
			adminChatID: 456,
			ReportConfig: ReportConfig{
				Storage:          mockReports,
				Threshold:        2,
				AutoBanThreshold: 5,
			},
		}

		_, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.NoError(t, err)
		assert.Len(t, mockAPI.SendCalls(), 1, "should send manual approval notification")
	})

	t.Run("dry mode - no actual bans or deletions", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				t.Error("should not make any Request calls in dry mode")
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{MessageID: 123}, nil
			},
		}

		mockBot := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			UpdateSpamFunc: func(msg string) error {
				t.Error("should not update spam samples in dry mode")
				return nil
			},
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 111, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 222, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 333, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
				}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error { return nil },
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			bot:         mockBot,
			primChatID:  200,
			adminChatID: 456,
			dry:         true,
			ReportConfig: ReportConfig{
				Storage:          mockReports,
				Threshold:        2,
				AutoBanThreshold: 3,
			},
		}

		_, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.NoError(t, err)
		assert.Empty(t, mockAPI.RequestCalls(), "should not make API requests in dry mode")
		assert.Empty(t, mockBot.UpdateSpamCalls(), "should not update spam in dry mode")
	})

	t.Run("notification failure handling - reports not deleted", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, fmt.Errorf("telegram API error")
			},
		}

		mockBot := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			UpdateSpamFunc:         func(msg string) error { return nil },
		}

		mockReports := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 111, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 222, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
					{MsgID: msgID, ChatID: chatID, ReporterUserID: 333, ReportedUserID: 666, ReportedUserName: "spammer", MsgText: "spam"},
				}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error {
				t.Error("should not delete reports when notification fails")
				return nil
			},
		}

		rep := &userReports{
			tbAPI:       mockAPI,
			bot:         mockBot,
			primChatID:  200,
			adminChatID: 456,
			ReportConfig: ReportConfig{
				Storage:          mockReports,
				Threshold:        2,
				AutoBanThreshold: 3,
			},
		}

		_, err := rep.checkReportThreshold(context.Background(), 100, 200)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "notification failed", "should indicate notification failure")
		assert.Empty(t, mockReports.DeleteByMessageCalls(), "should not delete reports when notification fails")

		assert.Len(t, mockBot.RemoveApprovedUserCalls(), 1, "should still ban user")
		assert.Len(t, mockBot.UpdateSpamCalls(), 1, "should still update spam")
	})
}
