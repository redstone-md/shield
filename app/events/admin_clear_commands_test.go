package events

import (
	"context"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdmin_DirectClearCommands(t *testing.T) {
	newAdmin := func(mockAPI *mocks.TbAPIMock, locator Locator) *admin {
		return &admin{
			tbAPI:                  mockAPI,
			primChatIDs:            []int64{123},
			adminChatID:            456,
			locator:                locator,
			superUsers:             SuperUsers{"superuser"},
			aggressiveCleanupLimit: 2,
		}
	}

	newAPI := func() *mocks.TbAPIMock {
		return &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{}, nil
			},
		}
	}

	newCommandUpdate := func(text string) tbapi.Update {
		return tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 456},
				From:      &tbapi.User{UserName: "admin", ID: 111},
				Text:      text,
			},
		}
	}

	t.Run("target by id", func(t *testing.T) {
		mockAPI := newAPI()
		locator := &mocks.LocatorMock{
			UserNameByIDFunc: func(ctx context.Context, userID int64) string {
				assert.Equal(t, int64(222), userID)
				return "spammer"
			},
			UserIDByNameFunc: func(ctx context.Context, userName string) int64 { return 0 },
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				assert.Equal(t, int64(222), userID)
				assert.Equal(t, 2, limit)
				return userMsgs(301, 302), nil
			},
		}
		adm := newAdmin(mockAPI, locator)

		err := adm.DirectClearTarget(context.Background(), newCommandUpdate("/clear 222"), "222")
		require.NoError(t, err)

		require.Len(t, locator.GetUserMessagesCalls(), 1)
		require.Len(t, mockAPI.RequestCalls(), 3)
		assert.Equal(t, 301, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
		assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
		assert.Equal(t, 302, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
		assert.Equal(t, int64(123), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)
		assert.Equal(t, 789, mockAPI.RequestCalls()[2].C.(tbapi.DeleteMessageConfig).MessageID)
		assert.Equal(t, int64(456), mockAPI.RequestCalls()[2].C.(tbapi.DeleteMessageConfig).ChatID)

		require.Len(t, mockAPI.SendCalls(), 1)
		notification := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text
		assert.Contains(t, notification, `удалено 2 сообщений пользователя <a href="tg://user?id=222">spammer</a> (222)`)
		assert.Contains(t, notification, `<a href="tg://user?id=111">admin</a>`)
	})

	t.Run("reply target", func(t *testing.T) {
		mockAPI := newAPI()
		locator := &mocks.LocatorMock{
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				assert.Equal(t, int64(222), userID)
				assert.Equal(t, 2, limit)
				return userMsgs(401), nil
			},
		}
		adm := newAdmin(mockAPI, locator)
		update := newCommandUpdate("/clear")
		update.Message.ReplyToMessage = &tbapi.Message{
			MessageID: 202,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{ID: 222, UserName: "spammer"},
			Text:      "message to clear",
		}

		err := adm.DirectClearReply(context.Background(), update)
		require.NoError(t, err)

		require.Len(t, locator.GetUserMessagesCalls(), 1)
		require.Len(t, mockAPI.RequestCalls(), 2)
		assert.Equal(t, 401, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
		assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
		assert.Equal(t, 789, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
		assert.Equal(t, int64(456), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)

		require.Len(t, mockAPI.SendCalls(), 1)
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text,
			`удалено 1 сообщений пользователя <a href="tg://user?id=222">spammer</a> (222)`)
	})

	t.Run("skips superuser target", func(t *testing.T) {
		mockAPI := newAPI()
		locator := &mocks.LocatorMock{
			GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
				return userMsgs(401), nil
			},
		}
		adm := newAdmin(mockAPI, locator)
		adm.superUsers = SuperUsers{"spammer"}
		update := newCommandUpdate("/clear")
		update.Message.ReplyToMessage = &tbapi.Message{
			MessageID: 202,
			From:      &tbapi.User{ID: 222, UserName: "spammer"},
		}

		err := adm.DirectClearReply(context.Background(), update)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "clear target is super-user")
		assert.Empty(t, locator.GetUserMessagesCalls())
		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, mockAPI.SendCalls())
	})
}

func TestTelegramListener_DoWithDirectClearByID(t *testing.T) {
	mockAPI, locator := newClearListenerMocks(t, []int{301, 302}, 2)
	l := newClearListener(mockAPI, locator)

	updMsg := tbapi.Update{Message: &tbapi.Message{
		MessageID: 101,
		Chat:      tbapi.Chat{ID: 456},
		Text:      "/clear 222",
		From:      &tbapi.User{UserName: "superuser1", ID: 77},
	}}

	err := runClearListener(t, l, updMsg)
	require.EqualError(t, err, "telegram update chan closed")

	require.Len(t, locator.GetUserMessagesCalls(), 1)
	require.Len(t, mockAPI.RequestCalls(), 3)
	assert.Equal(t, 301, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Equal(t, 302, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Equal(t, 101, mockAPI.RequestCalls()[2].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(456), mockAPI.RequestCalls()[2].C.(tbapi.DeleteMessageConfig).ChatID)
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "удалено 2 сообщений пользователя")
}

func TestTelegramListener_DoWithDirectClearReply(t *testing.T) {
	mockAPI, locator := newClearListenerMocks(t, []int{401}, 1)
	l := newClearListener(mockAPI, locator)
	l.AggressiveCleanupLimit = 1

	updMsg := tbapi.Update{Message: &tbapi.Message{
		MessageID: 101,
		Chat:      tbapi.Chat{ID: 456},
		Text:      "/clear",
		From:      &tbapi.User{UserName: "superuser1", ID: 77},
		ReplyToMessage: &tbapi.Message{
			MessageID: 202,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{ID: 222, UserName: "spammer"},
			Text:      "message to clear",
		},
	}}

	err := runClearListener(t, l, updMsg)
	require.EqualError(t, err, "telegram update chan closed")

	require.Len(t, locator.GetUserMessagesCalls(), 1)
	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, 401, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Equal(t, 101, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(456), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "удалено 1 сообщений пользователя")
}

func newClearListenerMocks(t *testing.T, messageIDs []int, wantLimit int) (*mocks.TbAPIMock, *mocks.LocatorMock) {
	t.Helper()

	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			if config.SuperGroupUsername == "@admin" || config.ChatID == 456 {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 456}}, nil
			}
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
	}
	locator := &mocks.LocatorMock{
		UserNameByIDFunc: func(ctx context.Context, userID int64) string {
			assert.Equal(t, int64(222), userID)
			return "spammer"
		},
		UserIDByNameFunc: func(ctx context.Context, userName string) int64 { return 0 },
		GetUserMessagesFunc: func(ctx context.Context, userID int64, limit int) ([]storage.UserMessage, error) {
			assert.Equal(t, int64(222), userID)
			assert.Equal(t, wantLimit, limit)
			return userMsgs(messageIDs...), nil
		},
	}
	return mockAPI, locator
}

func newClearListener(mockAPI *mocks.TbAPIMock, locator Locator) *TelegramListener {
	return &TelegramListener{
		SpamLogger:             &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}},
		TbAPI:                  mockAPI,
		Bot:                    &mocks.BotMock{},
		Group:                  "gr",
		AdminGroup:             "admin",
		SuperUsers:             SuperUsers{"superuser1"},
		Locator:                locator,
		AggressiveCleanupLimit: 2,
	}
}

func runClearListener(t *testing.T, l *TelegramListener, update tbapi.Update) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updChan := make(chan tbapi.Update, 1)
	updChan <- update
	close(updChan)
	l.TbAPI.(*mocks.TbAPIMock).GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel {
		return updChan
	}
	return l.Do(ctx)
}
