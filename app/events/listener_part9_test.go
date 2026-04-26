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

func TestTelegramListener_CallbackRouting(t *testing.T) {
	t.Run("R+ callback routes to reportsHandler", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{{User: &tbapi.User{UserName: "admin", ID: 1}}}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{MessageID: 100}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		reportsMock := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{{MsgID: 42, ChatID: 123, ReportedUserID: 999, ReportedUserName: "spammer"}}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error {
				return nil
			},
		}

		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			UpdateSpamFunc:         func(msg string) error { return nil },
		}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			TbAPI:      mockAPI,
			Bot:        botMock,
			SuperUsers: SuperUsers{"admin"},
			Group:      "123",
			AdminGroup: "456",
			Locator:    locator,
			ReportConfig: ReportConfig{
				Storage: reportsMock,
				Enabled: true,
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		callbackQuery := tbapi.CallbackQuery{
			ID:   "callback123",
			Data: "R+999:42",
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 100,
				Text:      "User spam reported",
				Date:      int(time.Now().Unix()),
			},
			From: &tbapi.User{UserName: "admin", ID: 1},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- tbapi.Update{CallbackQuery: &callbackQuery}
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")

		assert.Len(t, reportsMock.GetByMessageCalls(), 1)
		assert.Equal(t, 42, reportsMock.GetByMessageCalls()[0].MsgID)
	})

	t.Run("? callback routes to adminHandler", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{{User: &tbapi.User{UserName: "admin", ID: 1}}}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{MessageID: 100}, nil
			},
		}

		botMock := &mocks.BotMock{}
		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			TbAPI:      mockAPI,
			Bot:        botMock,
			SuperUsers: SuperUsers{"admin"},
			Group:      "123",
			AdminGroup: "456",
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		callbackQuery := tbapi.CallbackQuery{
			ID:   "callback123",
			Data: "?999:42",
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 100,
				Text:      "permanently banned user",
				Date:      int(time.Now().Unix()),
			},
			From: &tbapi.User{UserName: "admin", ID: 1},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- tbapi.Update{CallbackQuery: &callbackQuery}
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")

		sendCalls := mockAPI.SendCalls()
		found := false
		for _, call := range sendCalls {
			if _, ok := call.C.(tbapi.EditMessageReplyMarkupConfig); ok {
				found = true
				break
			}
		}
		assert.True(t, found, "adminHandler should edit message with confirmation buttons")
	})

	t.Run("R- callback routes to reportsHandler", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{{User: &tbapi.User{UserName: "admin", ID: 1}}}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{MessageID: 100}, nil
			},
		}

		reportsMock := &mocks.ReportsMock{
			GetByMessageFunc: func(ctx context.Context, msgID int, chatID int64) ([]storage.Report, error) {
				return []storage.Report{{MsgID: 42, ChatID: 123}}, nil
			},
			DeleteByMessageFunc: func(ctx context.Context, msgID int, chatID int64) error {
				return nil
			},
		}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			TbAPI:      mockAPI,
			Bot:        &mocks.BotMock{},
			SuperUsers: SuperUsers{"admin"},
			Group:      "123",
			AdminGroup: "456",
			Locator:    locator,
			ReportConfig: ReportConfig{
				Storage: reportsMock,
				Enabled: true,
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		callbackQuery := tbapi.CallbackQuery{
			ID:   "callback123",
			Data: "R-999:42",
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 100,
				Text:      "User spam reported",
				Date:      int(time.Now().Unix()),
			},
			From: &tbapi.User{UserName: "admin", ID: 1},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- tbapi.Update{CallbackQuery: &callbackQuery}
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")

		assert.Len(t, reportsMock.GetByMessageCalls(), 1)
		assert.Len(t, reportsMock.DeleteByMessageCalls(), 1)
	})

	t.Run("+ callback routes to adminHandler", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{{User: &tbapi.User{UserName: "admin", ID: 1}}}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{MessageID: 100}, nil
			},
		}

		botMock := &mocks.BotMock{
			UpdateSpamFunc: func(msg string) error { return nil },
		}
		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			TbAPI:      mockAPI,
			Bot:        botMock,
			SuperUsers: SuperUsers{"admin"},
			Group:      "123",
			AdminGroup: "456",
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		callbackQuery := tbapi.CallbackQuery{
			ID:   "callback123",
			Data: "+999:42",
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 100,
				Text:      "permanently banned user",
				Date:      int(time.Now().Unix()),
			},
			From: &tbapi.User{UserName: "admin", ID: 1},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- tbapi.Update{CallbackQuery: &callbackQuery}
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")

		sendCalls := mockAPI.SendCalls()
		found := false
		for _, call := range sendCalls {
			if editConfig, ok := call.C.(tbapi.EditMessageTextConfig); ok {
				if editConfig.ChatID == 456 && editConfig.MessageID == 100 {
					found = true
					break
				}
			}
		}
		assert.True(t, found, "adminHandler should edit message with confirmation")
	})

	t.Run("no prefix callback routes to adminHandler", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{{User: &tbapi.User{UserName: "admin", ID: 1}}}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{MessageID: 100}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		botMock := &mocks.BotMock{
			UpdateHamFunc:       func(msg string) error { return nil },
			AddApprovedUserFunc: func(id int64, name string) error { return nil },
		}
		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			TbAPI:      mockAPI,
			Bot:        botMock,
			SuperUsers: SuperUsers{"admin"},
			Group:      "123",
			AdminGroup: "456",
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		callbackQuery := tbapi.CallbackQuery{
			ID:   "callback123",
			Data: "999:42",
			Message: &tbapi.Message{
				Chat:      tbapi.Chat{ID: 456},
				MessageID: 100,
				Text:      "permanently banned [user](tg://user?id=999)\n\nspam message",
				Date:      int(time.Now().Unix()),
			},
			From: &tbapi.User{UserName: "admin", ID: 1},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- tbapi.Update{CallbackQuery: &callbackQuery}
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")

		assert.Len(t, botMock.UpdateHamCalls(), 1)
		assert.Len(t, botMock.AddApprovedUserCalls(), 1)
	})
}
