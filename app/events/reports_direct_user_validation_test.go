package events

import (
	"context"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestUserReports_DirectUserReport_ReportedMessageFromChannelShouldReturnError(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{}
	mockReports := &mocks.ReportsMock{}

	rep := &userReports{
		tbAPI:        mockAPI,
		primChatID:   123,
		adminChatID:  456,
		superUsers:   SuperUsers{},
		ReportConfig: ReportConfig{Storage: mockReports},
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "/report",
			From:      &tbapi.User{UserName: "reporter", ID: 111},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999,
				From:      nil,
				SenderChat: &tbapi.Chat{
					ID:   -100123456789,
					Type: "channel",
				},
				Text: "channel message",
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot report messages from channels or anonymous admins")
	assert.Empty(t, mockAPI.RequestCalls(), "should not delete message")
	assert.Empty(t, mockReports.AddCalls(), "should not add report")

}

func TestUserReports_DirectUserReport_ReportsStorageNotInitializedShouldReturnError(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: okSend,
	}

	mockBot := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response { return bot.Response{} }}

	rep := &userReports{
		tbAPI:       mockAPI,
		bot:         mockBot,
		primChatID:  123,
		adminChatID: 456,
		superUsers:  SuperUsers{},
		ReportConfig: ReportConfig{
			Storage:    nil,
			RateLimit:  0,
			RatePeriod: 1 * time.Hour,
		},
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "/report",
			From:      &tbapi.User{UserName: "reporter", ID: 111},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999,
				From:      &tbapi.User{ID: 666, UserName: "spammer"},
				Text:      "spam message",
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reports storage not initialized")
	assert.Len(t, mockAPI.RequestCalls(), 1, "should still delete /report message")

}

func TestUserReports_DirectUserReport_AllowsNonApprovedUsers(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: okSend,
	}

	mockBot := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response { return bot.Response{} }}

	mockReports := &mocks.ReportsMock{
		GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
			return 0, nil
		},
		AddFunc: func(ctx context.Context, report storage.Report) error {
			assert.Equal(t, int64(111), report.ReporterUserID)
			return nil
		},
		GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
			return []storage.Report{}, nil
		},
	}

	rep := &userReports{
		tbAPI:       mockAPI,
		bot:         mockBot,
		primChatID:  123,
		adminChatID: 456,
		superUsers:  SuperUsers{},
		ReportConfig: ReportConfig{
			Storage: mockReports,
		},
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "/report",
			From:      &tbapi.User{UserName: "reporter", ID: 111},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999,
				From:      &tbapi.User{ID: 666, UserName: "spammer"},
				Text:      "spam message",
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.NoError(t, err)
	assert.Len(t, mockAPI.RequestCalls(), 1, "should delete /report message")
	assert.Len(t, mockReports.AddCalls(), 1, "should add report")

}

func TestUserReports_DirectUserReport_AllowsAnyUsersWithoutApprovedLookup(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: okSend,
	}

	mockBot := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response { return bot.Response{} }}

	mockReports := &mocks.ReportsMock{
		GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
			return 5, nil
		},
		AddFunc: func(ctx context.Context, report storage.Report) error {
			return nil
		},
		GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
			return []storage.Report{}, nil
		},
	}

	rep := &userReports{
		tbAPI:       mockAPI,
		bot:         mockBot,
		primChatID:  123,
		adminChatID: 456,
		superUsers:  SuperUsers{},
		ReportConfig: ReportConfig{
			Storage:    mockReports,
			RateLimit:  10,
			RatePeriod: 1 * time.Hour,
			Threshold:  2,
		},
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "/report",
			From:      &tbapi.User{UserName: "random_user", ID: 222},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999,
				From:      &tbapi.User{ID: 666, UserName: "spammer"},
				Text:      "spam message",
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.NoError(t, err)
	assert.Len(t, mockAPI.RequestCalls(), 1, "should delete /report message")
	assert.Len(t, mockReports.AddCalls(), 1, "should add report")

}

func TestUserReports_DirectUserReport_MessageWithQuoteTextIncludesQuoteInStoredReport(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: okSend,
	}

	mockReports := &mocks.ReportsMock{
		GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
			return 0, nil
		},
		AddFunc: func(ctx context.Context, report storage.Report) error {
			assert.Equal(t, "Thank you\nBuy cheap stuff at spam.com", report.MsgText)
			return nil
		},
		GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
			return []storage.Report{}, nil
		},
	}

	mockBot := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response { return bot.Response{} }}

	rep := &userReports{
		tbAPI: mockAPI, bot: mockBot, primChatID: 123, adminChatID: 456,
		superUsers: SuperUsers{},
		ReportConfig: ReportConfig{
			Storage: mockReports, RateLimit: 10, RatePeriod: 1 * time.Hour, Threshold: 2,
		},
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 123}, Text: "/report",
			From: &tbapi.User{UserName: "reporter", ID: 111},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999, From: &tbapi.User{ID: 666, UserName: "spammer"},
				Text:  "Thank you",
				Quote: &tbapi.TextQuote{Text: "Buy cheap stuff at spam.com"},
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.NoError(t, err)
	require.Len(t, mockReports.AddCalls(), 1)

}

func TestUserReports_DirectUserReport_MessageWithoutQuoteStoresOnlyText(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: okSend,
	}

	mockReports := &mocks.ReportsMock{
		GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
			return 0, nil
		},
		AddFunc: func(ctx context.Context, report storage.Report) error {
			assert.Equal(t, "plain spam text", report.MsgText)
			return nil
		},
		GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
			return []storage.Report{}, nil
		},
	}

	mockBot := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response { return bot.Response{} }}

	rep := &userReports{
		tbAPI: mockAPI, bot: mockBot, primChatID: 123, adminChatID: 456,
		superUsers: SuperUsers{},
		ReportConfig: ReportConfig{
			Storage: mockReports, RateLimit: 10, RatePeriod: 1 * time.Hour, Threshold: 2,
		},
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 123}, Text: "/report",
			From: &tbapi.User{UserName: "reporter", ID: 111},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999, From: &tbapi.User{ID: 666, UserName: "spammer"},
				Text: "plain spam text",
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.NoError(t, err)
	require.Len(t, mockReports.AddCalls(), 1)

}

func TestUserReports_DirectUserReport_MessageWithEmptyQuoteTextStoresOnlyText(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: okSend,
	}

	mockReports := &mocks.ReportsMock{
		GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
			return 0, nil
		},
		AddFunc: func(ctx context.Context, report storage.Report) error {
			assert.Equal(t, "some text", report.MsgText)
			return nil
		},
		GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
			return []storage.Report{}, nil
		},
	}

	mockBot := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response { return bot.Response{} }}

	rep := &userReports{
		tbAPI: mockAPI, bot: mockBot, primChatID: 123, adminChatID: 456,
		superUsers: SuperUsers{},
		ReportConfig: ReportConfig{
			Storage: mockReports, RateLimit: 10, RatePeriod: 1 * time.Hour, Threshold: 2,
		},
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 123}, Text: "/report",
			From: &tbapi.User{UserName: "reporter", ID: 111},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999, From: &tbapi.User{ID: 666, UserName: "spammer"},
				Text:  "some text",
				Quote: &tbapi.TextQuote{Text: ""},
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.NoError(t, err)
	require.Len(t, mockReports.AddCalls(), 1)

}

func TestUserReports_DirectUserReport_EmptyTextWithQuotePresentUsesTransformFallbackPlusQuote(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: okSend,
	}

	mockReports := &mocks.ReportsMock{
		GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
			return 0, nil
		},
		AddFunc: func(ctx context.Context, report storage.Report) error {
			assert.Equal(t, "image caption\nquoted spam content", report.MsgText)
			return nil
		},
		GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
			return []storage.Report{}, nil
		},
	}

	mockBot := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response { return bot.Response{} }}

	rep := &userReports{
		tbAPI: mockAPI, bot: mockBot, primChatID: 123, adminChatID: 456,
		superUsers: SuperUsers{},
		ReportConfig: ReportConfig{
			Storage: mockReports, RateLimit: 10, RatePeriod: 1 * time.Hour, Threshold: 2,
		},
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 123}, Text: "/report",
			From: &tbapi.User{UserName: "reporter", ID: 111},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999, From: &tbapi.User{ID: 666, UserName: "spammer"},
				Text:    "",
				Caption: "image caption",
				Photo:   []tbapi.PhotoSize{{FileID: "photo123"}},
				Quote:   &tbapi.TextQuote{Text: "quoted spam content"},
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.NoError(t, err)
	require.Len(t, mockReports.AddCalls(), 1)

}

func TestUserReports_DirectUserReport_SuperUserShouldUseSpamInsteadOfReport(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{}
	mockBot := &mocks.BotMock{}
	mockReports := &mocks.ReportsMock{}

	rep := &userReports{
		tbAPI:       mockAPI,
		bot:         mockBot,
		primChatID:  123,
		adminChatID: 456,
		superUsers:  SuperUsers{"superuser"},
		ReportConfig: ReportConfig{
			Storage: mockReports,
		},
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "/report",
			From:      &tbapi.User{UserName: "superuser", ID: 111},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999,
				From:      &tbapi.User{ID: 666, UserName: "spammer"},
				Text:      "spam message",
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use /spam instead")

}
