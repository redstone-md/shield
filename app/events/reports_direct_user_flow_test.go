package events

import (
	"context"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestUserReports_DirectUserReport_SuccessfulReportFromRegularUser(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: okSend,
	}

	mockReports := &mocks.ReportsMock{
		GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
			return 5, nil
		},
		AddFunc: func(ctx context.Context, report storage.Report) error {
			assert.Equal(t, int(999), report.MsgID)
			assert.Equal(t, int64(123), report.ChatID)
			assert.Equal(t, int64(111), report.ReporterUserID)
			assert.Equal(t, "reporter", report.ReporterUserName)
			assert.Equal(t, int64(666), report.ReportedUserID)
			assert.Equal(t, "spammer", report.ReportedUserName)
			assert.Equal(t, "spam message", report.MsgText)
			return nil
		},
		GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
			return []storage.Report{}, nil
		},
	}

	mockBot := &mocks.BotMock{
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			assert.True(t, checkOnly)
			assert.True(t, msg.ForceLLM)
			assert.Equal(t, reportLLMContext, msg.LLMContext)
			return bot.Response{}
		},
	}

	rep := &userReports{
		tbAPI:       mockAPI,
		bot:         mockBot,
		primChatID:  123,
		adminChatID: 456,
		superUsers:  SuperUsers{"superuser"},
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
	assert.Len(t, mockReports.AddCalls(), 1, "should add report to storage")
	assert.Len(t, mockReports.GetReporterCountSinceCalls(), 1, "should check rate limit")
	assert.Len(t, mockBot.OnMessageCalls(), 1, "should force llm review before storing report")

}

func TestUserReports_DirectUserReport_LlmConfirmedReportTriggersImmediateModeration(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{MessageID: 1234}, nil
		},
	}

	mockReports := &mocks.ReportsMock{
		GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
			return 0, nil
		},
	}

	mockBot := &mocks.BotMock{
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			assert.True(t, checkOnly)
			assert.True(t, msg.ForceLLM)
			assert.Equal(t, reportLLMContext, msg.LLMContext)
			return bot.Response{
				Send: true,
				CheckResults: []spamcheck.Response{
					{Name: "openai", Spam: true, Details: "priority violation, confidence: 96%"},
				},
			}
		},
		RemoveApprovedUserFunc: func(id int64) error {
			assert.Equal(t, int64(666), id)
			return nil
		},
		UpdateSpamFunc: func(msg string) error {
			assert.Equal(t, "spam message", msg)
			return nil
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
			RatePeriod: time.Hour,
			Threshold:  2,
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
	assert.Len(t, mockBot.OnMessageCalls(), 1)
	assert.Len(t, mockBot.RemoveApprovedUserCalls(), 1)
	assert.Len(t, mockBot.UpdateSpamCalls(), 1)
	assert.Empty(t, mockReports.AddCalls(), "should not store report when llm already confirmed spam")
	assert.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 2, "should delete command and original message")
	assert.Len(t, mockAPI.SendCalls(), 2, "should notify admin chat and send confirmation")

}

func TestUserReports_DirectUserReport_ReporterIsSuperuserShouldReturnError(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{}
	mockReports := &mocks.ReportsMock{}

	rep := &userReports{
		tbAPI:        mockAPI,
		primChatID:   123,
		adminChatID:  456,
		superUsers:   SuperUsers{"superuser"},
		ReportConfig: ReportConfig{Storage: mockReports},
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
	assert.Empty(t, mockAPI.RequestCalls(), "should not delete message")
	assert.Empty(t, mockReports.AddCalls(), "should not add report")

}

func TestUserReports_DirectUserReport_ReportedUserIsSuperuserShouldReturnError(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{}
	mockReports := &mocks.ReportsMock{}

	rep := &userReports{
		tbAPI:        mockAPI,
		primChatID:   123,
		adminChatID:  456,
		superUsers:   SuperUsers{"superuser"},
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
				From:      &tbapi.User{ID: 666, UserName: "superuser"},
				Text:      "some message",
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "from super-user")
	assert.Empty(t, mockAPI.RequestCalls(), "should not delete message")
	assert.Empty(t, mockReports.AddCalls(), "should not add report")

}

func TestUserReports_DirectUserReport_ForumTopicCreationMessageShouldReturnError(t *testing.T) {
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
				MessageID:         1,
				From:              &tbapi.User{ID: 666, UserName: "topic_creator"},
				ForumTopicCreated: &tbapi.ForumTopicCreated{Name: "General"},
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot report forum topic creation messages")
	assert.Empty(t, mockAPI.RequestCalls(), "should not delete message")
	assert.Empty(t, mockReports.AddCalls(), "should not add report")

}

func TestUserReports_DirectUserReport_RateLimitExceededShouldDeleteCommandAndReturnError(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: okSend,
	}

	mockBot := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response { return bot.Response{} }}

	mockReports := &mocks.ReportsMock{
		GetReporterCountSinceFunc: func(ctx context.Context, reporterID int64, since time.Time) (int, error) {
			return 10, nil
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
	assert.Contains(t, err.Error(), "rate limit exceeded")
	assert.Len(t, mockAPI.RequestCalls(), 1, "should still delete /report message")
	assert.Empty(t, mockReports.AddCalls(), "should not add report when rate limited")

}

func TestUserReports_DirectUserReport_ReportsStorageAddError(t *testing.T) {
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
			return fmt.Errorf("database error")
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
	assert.Contains(t, err.Error(), "failed to add report")
	assert.Len(t, mockAPI.RequestCalls(), 1, "should delete /report message")
	assert.Len(t, mockReports.AddCalls(), 1, "should attempt to add report")

}

func TestUserReports_DirectUserReport_EmptyMessageTextShouldUseTransformedMessage(t *testing.T) {
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
			assert.Contains(t, report.MsgText, "caption from image")
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
			From:      &tbapi.User{UserName: "reporter", ID: 111},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999,
				From:      &tbapi.User{ID: 666, UserName: "spammer"},
				Text:      "",
				Caption:   "caption from image",
				Photo:     []tbapi.PhotoSize{{FileID: "photo123"}},
			},
		},
	}

	err := rep.DirectUserReport(context.Background(), update)
	require.NoError(t, err)
	assert.Len(t, mockReports.AddCalls(), 1, "should add report with transformed text")

}
