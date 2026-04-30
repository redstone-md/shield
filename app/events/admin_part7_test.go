package events

import (
	"context"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"strings"
	"testing"
	"time"
)

func TestAdmin_DirectReportWithAggressiveCleanup(t *testing.T) {

	setupAggressiveCleanupTest := func(aggressiveCleanup bool, dry bool, messageIDs []int) (*mocks.TbAPIMock, *mocks.LocatorMock, *admin) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		botMock := &mocks.BotMock{
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{
					Send:        true,
					BanInterval: bot.PermanentBanDuration,
					CheckResults: []spamcheck.Response{
						{Name: "test", Spam: true, Details: "spam detected"},
					},
				}
			},
			RemoveApprovedUserFunc: func(id int64) error {
				return nil
			},
			UpdateSpamFunc: func(msg string) error {
				return nil
			},
		}

		locatorMock := &mocks.LocatorMock{
			GetUserMessageIDsFunc: func(ctx context.Context, userID int64, limit int) ([]int, error) {
				return messageIDs, nil
			},
		}

		adm := &admin{
			tbAPI:                  mockAPI,
			bot:                    botMock,
			primChatID:             123,
			adminChatID:            456,
			locator:                locatorMock,
			superUsers:             SuperUsers{},
			aggressiveCleanup:      aggressiveCleanup,
			aggressiveCleanupLimit: 100,
			dry:                    dry,
		}

		return mockAPI, locatorMock, adm
	}

	createSpamReportUpdate := func() tbapi.Update {
		return tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "/spam",
				From:      &tbapi.User{UserName: "admin", ID: 111},
				ReplyToMessage: &tbapi.Message{
					MessageID: 999,
					From:      &tbapi.User{ID: 666, UserName: "spammer"},
					Text:      "spam message",
				},
			},
		}
	}

	t.Run("aggressive cleanup enabled", func(t *testing.T) {
		mockAPI, locatorMock, adm := setupAggressiveCleanupTest(true, false, []int{100, 101, 102})
		update := createSpamReportUpdate()

		err := adm.directReport(context.Background(), update, true)
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond)

		assert.Len(t, locatorMock.GetUserMessageIDsCalls(), 1)
		assert.Equal(t, int64(666), locatorMock.GetUserMessageIDsCalls()[0].UserID)
		assert.Equal(t, 100, locatorMock.GetUserMessageIDsCalls()[0].Limit)

		requestCalls := mockAPI.RequestCalls()
		deleteCount := 0
		for _, call := range requestCalls {
			if _, ok := call.C.(tbapi.DeleteMessageConfig); ok {
				deleteCount++
			}
		}
		assert.Equal(t, 5, deleteCount, "Should delete original + admin messages and 3 additional messages")

		sendCalls := mockAPI.SendCalls()
		foundNotification := false
		var notificationMsg string
		for _, call := range sendCalls {
			if msg, ok := call.C.(tbapi.MessageConfig); ok {
				if strings.Contains(msg.Text, "deleted 3 messages from spammer") {
					foundNotification = true
					notificationMsg = msg.Text
					break
				}
			}
		}
		assert.True(t, foundNotification, "Should send notification about deleted messages")
		assert.Contains(t, notificationMsg, "deleted 3 messages from spammer \"spammer\" (666)", "Notification should include username and ID")
	})

	t.Run("aggressive cleanup disabled", func(t *testing.T) {
		mockAPI, locatorMock, adm := setupAggressiveCleanupTest(false, false, []int{})
		update := createSpamReportUpdate()

		err := adm.directReport(context.Background(), update, true)
		require.NoError(t, err)

		assert.Empty(t, locatorMock.GetUserMessageIDsCalls())

		requestCalls := mockAPI.RequestCalls()
		deleteCount := 0
		for _, call := range requestCalls {
			if _, ok := call.C.(tbapi.DeleteMessageConfig); ok {
				deleteCount++
			}
		}
		assert.Equal(t, 2, deleteCount, "Should only delete original and admin messages")
	})

	t.Run("aggressive cleanup in dry mode", func(t *testing.T) {
		_, locatorMock, adm := setupAggressiveCleanupTest(true, true, []int{})
		update := createSpamReportUpdate()

		err := adm.directReport(context.Background(), update, false)
		require.NoError(t, err)

		assert.Empty(t, locatorMock.GetUserMessageIDsCalls())
	})
}

func TestAdmin_DeleteUserMessages(t *testing.T) {
	t.Run("successful deletion", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {

				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		locatorMock := &mocks.LocatorMock{
			GetUserMessageIDsFunc: func(ctx context.Context, userID int64, limit int) ([]int, error) {
				assert.Equal(t, int64(666), userID)
				assert.Equal(t, 100, limit)
				return []int{101, 102, 103}, nil
			},
		}

		adm := &admin{
			tbAPI:                  mockAPI,
			locator:                locatorMock,
			primChatID:             123456789,
			aggressiveCleanupLimit: 100,
		}

		deleted, err := adm.deleteUserMessages(context.Background(), 666)
		require.NoError(t, err)
		assert.Equal(t, 3, deleted)

		requestCalls := mockAPI.RequestCalls()
		assert.Len(t, requestCalls, 3)
		for i, call := range requestCalls {
			deleteConfig, ok := call.C.(tbapi.DeleteMessageConfig)
			require.True(t, ok)
			assert.Equal(t, 101+i, deleteConfig.MessageID)
			assert.Equal(t, int64(123456789), deleteConfig.ChatID)
		}
	})

	t.Run("locator error", func(t *testing.T) {
		locatorMock := &mocks.LocatorMock{
			GetUserMessageIDsFunc: func(ctx context.Context, userID int64, limit int) ([]int, error) {
				return nil, fmt.Errorf("database error")
			},
		}

		adm := &admin{
			locator:                locatorMock,
			aggressiveCleanupLimit: 100,
		}

		deleted, err := adm.deleteUserMessages(context.Background(), 666)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user messages")
		assert.Equal(t, 0, deleted)
	})

	t.Run("partial deletion success", func(t *testing.T) {
		callCount := 0
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				callCount++

				if callCount == 2 {
					return nil, fmt.Errorf("message already deleted")
				}
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		locatorMock := &mocks.LocatorMock{
			GetUserMessageIDsFunc: func(ctx context.Context, userID int64, limit int) ([]int, error) {
				return []int{201, 202, 203}, nil
			},
		}

		adm := &admin{
			tbAPI:                  mockAPI,
			locator:                locatorMock,
			primChatID:             123456789,
			aggressiveCleanupLimit: 100,
		}

		deleted, err := adm.deleteUserMessages(context.Background(), 666)
		require.NoError(t, err)
		assert.Equal(t, 2, deleted)

		assert.Len(t, mockAPI.RequestCalls(), 3)
	})

	t.Run("too many consecutive failures", func(t *testing.T) {
		failCount := 0
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				failCount++
				return nil, fmt.Errorf("failed to delete")
			},
		}

		locatorMock := &mocks.LocatorMock{
			GetUserMessageIDsFunc: func(ctx context.Context, userID int64, limit int) ([]int, error) {
				return []int{301, 302, 303, 304, 305, 306, 307}, nil
			},
		}

		adm := &admin{
			tbAPI:                  mockAPI,
			locator:                locatorMock,
			primChatID:             123456789,
			aggressiveCleanupLimit: 100,
		}

		deleted, err := adm.deleteUserMessages(context.Background(), 666)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stopped after 5 consecutive failures")
		assert.Equal(t, 0, deleted)
		assert.Len(t, mockAPI.RequestCalls(), 5)
	})

	t.Run("empty message list", func(t *testing.T) {
		locatorMock := &mocks.LocatorMock{
			GetUserMessageIDsFunc: func(ctx context.Context, userID int64, limit int) ([]int, error) {
				return []int{}, nil
			},
		}

		adm := &admin{
			locator:                locatorMock,
			aggressiveCleanupLimit: 100,
		}

		deleted, err := adm.deleteUserMessages(context.Background(), 666)
		require.NoError(t, err)
		assert.Equal(t, 0, deleted)
	})
}

func TestAdmin_channelDisplayName(t *testing.T) {
	adm := &admin{}

	tests := []struct {
		name     string
		chat     *tbapi.Chat
		expected string
	}{
		{name: "nil chat", chat: nil, expected: ""},
		{name: "username set", chat: &tbapi.Chat{ID: 123, UserName: "mychannel", Title: "My Channel"}, expected: "mychannel"},
		{name: "title only", chat: &tbapi.Chat{ID: 123, Title: "My Channel"}, expected: "My Channel"},
		{name: "neither username nor title", chat: &tbapi.Chat{ID: 123}, expected: "channel_123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, adm.channelDisplayName(tt.chat))
		})
	}
}
