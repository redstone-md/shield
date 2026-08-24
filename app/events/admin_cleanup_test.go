package events

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// userMsgs builds locator UserMessage results from bare msg IDs (ChatID 0 falls
// back to the primary chat in deleteUserMessages).
func userMsgs(ids ...int) []storage.UserMessage {
	msgs := make([]storage.UserMessage, len(ids))
	for i, id := range ids {
		msgs[i] = storage.UserMessage{MsgID: id}
	}
	return msgs
}

func TestAdmin_DirectReportCleansUpUserMessages(t *testing.T) {

	setupCleanupTest := func(dry bool, messageIDs []int) (*mocks.TbAPIMock, *mocks.LocatorMock, *admin) {
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
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				return userMsgs(messageIDs...), nil
			},
			DeleteUserMessageFunc: func(ctx context.Context, chatID int64, msgID int) error {
				return nil
			},
		}

		adm := &admin{
			tbAPI:       mockAPI,
			bot:         botMock,
			primChatIDs: []int64{123},
			adminChatID: 456,
			locator:     locatorMock,
			superUsers:  SuperUsers{},
			dry:         dry,
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

	t.Run("cleans up all seen user messages", func(t *testing.T) {
		mockAPI, locatorMock, adm := setupCleanupTest(false, []int{100, 101, 102})
		update := createSpamReportUpdate()

		err := adm.directReport(context.Background(), update, true)
		require.NoError(t, err)

		// the cleanup notification is the final side effect of the async worker:
		// waiting for it makes every earlier assertion race-free
		require.Eventually(t, func() bool {
			for _, call := range mockAPI.SendCalls() {
				if msg, ok := call.C.(tbapi.MessageConfig); ok &&
					strings.Contains(msg.Text, "удалено 3 сообщений спамера") {
					return true
				}
			}
			return false
		}, time.Second, 10*time.Millisecond)

		assert.Equal(t, int64(666), locatorMock.GetUserMessagesCalls()[0].UserID)
		assert.Equal(t, cleanupBatchSize, locatorMock.GetUserMessagesCalls()[0].Limit)

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
				if strings.Contains(msg.Text, "удалено 3 сообщений спамера") {
					foundNotification = true
					notificationMsg = msg.Text
					break
				}
			}
		}
		assert.True(t, foundNotification, "Should send notification about deleted messages")
		assert.Contains(t, notificationMsg, "удалено 3 сообщений спамера \"spammer\" (666)", "Notification should include username and ID")
	})

	t.Run("no stored messages to clean", func(t *testing.T) {
		mockAPI, locatorMock, adm := setupCleanupTest(false, []int{})
		update := createSpamReportUpdate()

		err := adm.directReport(context.Background(), update, true)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			return len(locatorMock.GetUserMessagesCalls()) == 1
		}, time.Second, 10*time.Millisecond)

		requestCalls := mockAPI.RequestCalls()
		deleteCount := 0
		for _, call := range requestCalls {
			if _, ok := call.C.(tbapi.DeleteMessageConfig); ok {
				deleteCount++
			}
		}
		assert.Equal(t, 2, deleteCount, "Should only delete original and admin messages")
	})

	t.Run("skips cleanup in dry mode", func(t *testing.T) {
		_, locatorMock, adm := setupCleanupTest(true, []int{})
		update := createSpamReportUpdate()

		err := adm.directReport(context.Background(), update, false)
		require.NoError(t, err)

		assert.Empty(t, locatorMock.GetUserMessagesCalls())
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
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				assert.Equal(t, int64(666), userID)
				assert.Equal(t, cleanupBatchSize, limit)
				return userMsgs(101, 102, 103), nil
			},
			DeleteUserMessageFunc: func(ctx context.Context, chatID int64, msgID int) error {
				assert.Equal(t, int64(123456789), chatID)
				return nil
			},
		}

		adm := &admin{
			tbAPI:       mockAPI,
			locator:     locatorMock,
			primChatIDs: []int64{123456789},
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
		require.Len(t, locatorMock.DeleteUserMessageCalls(), 3)
	})

	t.Run("pages through more than one batch", func(t *testing.T) {
		oldInterval := cleanupRateInterval
		cleanupRateInterval = time.Millisecond
		defer func() { cleanupRateInterval = oldInterval }()

		// pool models the user_messages table: GetUserMessages serves the newest
		// rows up to the batch size, DeleteUserMessage drops them once deleted
		var poolMu sync.Mutex
		pool := make([]storage.UserMessage, 0, cleanupBatchSize+7)
		for i := range cleanupBatchSize + 7 {
			pool = append(pool, storage.UserMessage{MsgID: 1000 + i})
		}
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}
		locatorMock := &mocks.LocatorMock{
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				assert.Equal(t, int64(666), userID)
				assert.Equal(t, cleanupBatchSize, limit)
				poolMu.Lock()
				defer poolMu.Unlock()
				n := min(limit, len(pool))
				return append([]storage.UserMessage(nil), pool[:n]...), nil
			},
			DeleteUserMessageFunc: func(ctx context.Context, chatID int64, msgID int) error {
				poolMu.Lock()
				defer poolMu.Unlock()
				for i, m := range pool {
					if m.MsgID == msgID {
						pool = append(pool[:i], pool[i+1:]...)
						break
					}
				}
				return nil
			},
		}

		adm := &admin{
			tbAPI:       mockAPI,
			locator:     locatorMock,
			primChatIDs: []int64{123456789},
		}

		deleted, err := adm.deleteUserMessages(context.Background(), 666)
		require.NoError(t, err)
		assert.Equal(t, cleanupBatchSize+7, deleted)

		poolMu.Lock()
		defer poolMu.Unlock()
		assert.Empty(t, pool, "every stored message row should be consumed")
	})

	t.Run("locator error", func(t *testing.T) {
		locatorMock := &mocks.LocatorMock{
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				return nil, fmt.Errorf("database error")
			},
		}

		adm := &admin{
			locator: locatorMock,
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
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				return userMsgs(201, 202, 203), nil
			},
			DeleteUserMessageFunc: func(ctx context.Context, chatID int64, msgID int) error {
				return nil
			},
		}

		adm := &admin{
			tbAPI:       mockAPI,
			locator:     locatorMock,
			primChatIDs: []int64{123456789},
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
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				return userMsgs(301, 302, 303, 304, 305, 306, 307), nil
			},
			DeleteUserMessageFunc: func(ctx context.Context, chatID int64, msgID int) error {
				return nil
			},
		}

		adm := &admin{
			tbAPI:       mockAPI,
			locator:     locatorMock,
			primChatIDs: []int64{123456789},
		}

		deleted, err := adm.deleteUserMessages(context.Background(), 666)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stopped after 5 consecutive failures")
		assert.Equal(t, 0, deleted)
		assert.Len(t, mockAPI.RequestCalls(), 5)
	})

	t.Run("permanent telegram errors are skipped and purged from locator", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return nil, fmt.Errorf("Bad Request: message to delete not found")
			},
		}
		var purgedIDs []int
		locatorMock := &mocks.LocatorMock{
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				return userMsgs(401, 402, 403, 404, 405, 406, 407), nil
			},
			DeleteUserMessageFunc: func(ctx context.Context, chatID int64, msgID int) error {
				purgedIDs = append(purgedIDs, msgID)
				return nil
			},
		}

		adm := &admin{
			tbAPI:       mockAPI,
			locator:     locatorMock,
			primChatIDs: []int64{123456789},
		}

		// more than maxConsecutiveFailures rows: permanent errors must not trip the guard
		deleted, err := adm.deleteUserMessages(context.Background(), 666)
		require.NoError(t, err)
		assert.Equal(t, 0, deleted)
		assert.Len(t, mockAPI.RequestCalls(), 7)
		assert.ElementsMatch(t, []int{401, 402, 403, 404, 405, 406, 407}, purgedIDs)
	})

	t.Run("empty message list", func(t *testing.T) {
		locatorMock := &mocks.LocatorMock{
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				return userMsgs(), nil
			},
		}

		adm := &admin{
			locator: locatorMock,
		}

		deleted, err := adm.deleteUserMessages(context.Background(), 666)
		require.NoError(t, err)
		assert.Equal(t, 0, deleted)
	})

	t.Run("deletes each message in its own chat", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}
		locatorMock := &mocks.LocatorMock{
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				return []storage.UserMessage{
					{ChatID: 111, MsgID: 501},
					{ChatID: 222, MsgID: 502},
					{ChatID: 0, MsgID: 503}, // legacy row without chat_id falls back to primary chat
				}, nil
			},
			DeleteUserMessageFunc: func(ctx context.Context, chatID int64, msgID int) error {
				return nil
			},
		}
		adm := &admin{
			tbAPI:       mockAPI,
			locator:     locatorMock,
			primChatIDs: []int64{999},
		}

		deleted, err := adm.deleteUserMessages(context.Background(), 666)
		require.NoError(t, err)
		assert.Equal(t, 3, deleted)

		calls := mockAPI.RequestCalls()
		require.Len(t, calls, 3)
		assert.Equal(t, int64(111), calls[0].C.(tbapi.DeleteMessageConfig).ChatID)
		assert.Equal(t, int64(222), calls[1].C.(tbapi.DeleteMessageConfig).ChatID)
		assert.Equal(t, int64(999), calls[2].C.(tbapi.DeleteMessageConfig).ChatID)
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
