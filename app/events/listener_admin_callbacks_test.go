package events

import (
	"context"
	"errors"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestTelegramListener_DoWithAdminUnbanDecline(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if mc, ok := c.(tbapi.MessageConfig); ok {
				return tbapi.Message{Text: mc.Text, From: &tbapi.User{UserName: "user"}}, nil
			}
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	b := &mocks.BotMock{
		UpdateSpamFunc: func(msg string) error {
			return nil
		},
		AddApprovedUserFunc: func(id int64, name string) error { return nil },
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        b,
		SuperUsers: SuperUsers{"admin"},
		Group:      "gr",
		Locator:    locator,
		AdminGroup: "123",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		CallbackQuery: &tbapi.CallbackQuery{
			Data: "+999:987654",
			Message: &tbapi.Message{
				MessageID:     987654,
				Chat:          tbapi.Chat{ID: 123},
				Text:          "unban user blah\n\nthis was the ham, not spam",
				From:          &tbapi.User{UserName: "user", ID: 999},
				ForwardOrigin: &tbapi.MessageOrigin{Date: time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()},
			},
			From: &tbapi.User{UserName: "admin", ID: 1000},
		},
	}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, "unban user blah")
	kb := mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).ReplyMarkup.InlineKeyboard
	assert.Empty(t, kb, "buttons cleared")
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, `бан подтвержден администратором <a href="tg://user?id=1000">admin</a>`)
	assert.Empty(t, mockAPI.RequestCalls())
	assert.Len(t, b.UpdateSpamCalls(), 1)
	assert.Empty(t, b.UpdateHamCalls())
	require.Empty(t, b.AddApprovedUserCalls())
}

func TestTelegramListener_DoWithAdminBanConfirmedTraining(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if mc, ok := c.(tbapi.MessageConfig); ok {
				return tbapi.Message{Text: mc.Text, From: &tbapi.User{UserName: "user"}}, nil
			}
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	b := &mocks.BotMock{
		UpdateSpamFunc: func(msg string) error {
			return nil
		},
		AddApprovedUserFunc: func(id int64, name string) error { return nil },
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger:   mockLogger,
		TbAPI:        mockAPI,
		Bot:          b,
		SuperUsers:   SuperUsers{"admin"},
		Group:        "gr",
		Locator:      locator,
		AdminGroup:   "123",
		TrainingMode: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		CallbackQuery: &tbapi.CallbackQuery{
			Data: "+999:987654",
			Message: &tbapi.Message{
				MessageID:     987654,
				Chat:          tbapi.Chat{ID: 123},
				Text:          "unban user blah\n\nthis was the ham, not spam",
				From:          &tbapi.User{UserName: "user", ID: 999},
				ForwardOrigin: &tbapi.MessageOrigin{Date: time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()},
			},
			From: &tbapi.User{UserName: "admin", ID: 1000},
		},
	}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, "unban user blah")
	kb := mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).ReplyMarkup.InlineKeyboard
	assert.Empty(t, kb, "buttons cleared")
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, `бан подтвержден администратором <a href="tg://user?id=1000">admin</a>`)
	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, int64(999), mockAPI.RequestCalls()[0].C.(tbapi.BanChatMemberConfig).UserID, "user banned")
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.BanChatMemberConfig).ChatID, "chat id")
	assert.Equal(t, 987654, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID, "message deleted")

	assert.Len(t, b.UpdateSpamCalls(), 1)
	assert.Empty(t, b.UpdateHamCalls())
	require.Empty(t, b.AddApprovedUserCalls())
}

func TestTelegramListener_DoWithAdminShowInfo(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if mc, ok := c.(tbapi.MessageConfig); ok {
				return tbapi.Message{Text: mc.Text, From: &tbapi.User{UserName: "user"}}, nil
			}
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	b := &mocks.BotMock{}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        b,
		SuperUsers: SuperUsers{"admin"},
		Group:      "gr",
		Locator:    locator,
		AdminGroup: "123",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		CallbackQuery: &tbapi.CallbackQuery{
			Data: "!999:987654",
			Message: &tbapi.Message{
				MessageID:     987654,
				Chat:          tbapi.Chat{ID: 123},
				Text:          "unban user blah\n\nthis was the spam",
				From:          &tbapi.User{UserName: "user", ID: 999},
				ForwardOrigin: &tbapi.MessageOrigin{Date: time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()},
			},
			From: &tbapi.User{UserName: "admin", ID: 1000},
		},
	}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Locator.AddSpam(ctx, 999, []spamcheck.Response{{Name: "rule1", Spam: true, Details: "details1"},
		{Name: "rule2", Spam: true, Details: "details2"}})
	require.NoError(t, err)

	err = l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, "unban user blah")
	kb := mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).ReplyMarkup.InlineKeyboard
	assert.Empty(t, kb, "buttons cleared")
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, "results</b>\n- rule1: spam, details1\n- rule2: spam, details2")
	assert.Empty(t, mockAPI.RequestCalls())
	assert.Empty(t, b.UpdateSpamCalls())
	assert.Empty(t, b.UpdateHamCalls())
	require.Empty(t, b.AddApprovedUserCalls())
}

func TestTelegramListener_DoWithProcNewChatMemberMessage(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "user"}}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
	}
	b := &mocks.BotMock{}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		TbAPI:      mockAPI,
		Bot:        b,
		SuperUsers: SuperUsers{"admin"},
		Group:      "gr",
		Locator:    locator,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat:           tbapi.Chat{ID: 123},
			From:           &tbapi.User{UserName: "new_user", ID: 321},
			NewChatMembers: []tbapi.User{{UserName: "new_user", ID: 321}},
			MessageID:      22,
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	meta, found := l.Locator.Message(ctx, "new_123_321")
	assert.True(t, found)
	assert.Equal(t, int64(321), meta.UserID)
	assert.Equal(t, 22, meta.MsgID)
	assert.Equal(t, "new_user", meta.UserName)
	assert.Equal(t, int64(123), meta.ChatID)
	assert.Equal(t, int64(321), l.Locator.UserIDByName(ctx, "new_user"))

}

func TestTelegramListener_DoWithProcLeftChatMemberMessage(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "user"}}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	b := &mocks.BotMock{}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		TbAPI:               mockAPI,
		Bot:                 b,
		SuperUsers:          SuperUsers{"admin"},
		Group:               "gr",
		Locator:             locator,
		SuppressJoinMessage: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	t.Run("user has left the chat by himself", func(t *testing.T) {
		mockAPI.ResetCalls()
		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat:           tbapi.Chat{ID: 123},
				From:           &tbapi.User{UserName: "new_user", ID: 321},
				LeftChatMember: &tbapi.User{UserName: "new_user", ID: 321},
				MessageID:      22,
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.Empty(t, mockAPI.SendCalls())
		require.Empty(t, mockAPI.RequestCalls())
	})

	t.Run("user has left the chat by admin, we don't have a message about joining", func(t *testing.T) {
		mockAPI.ResetCalls()
		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat:           tbapi.Chat{ID: 123},
				From:           &tbapi.User{UserName: "new_user", ID: 999},
				LeftChatMember: &tbapi.User{UserName: "new_user", ID: 321},
				MessageID:      22,
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.Empty(t, mockAPI.SendCalls())
		require.Empty(t, mockAPI.RequestCalls())
	})

	t.Run("user has left the chat by admin, we have a message about joining", func(t *testing.T) {
		mockAPI.ResetCalls()
		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat:           tbapi.Chat{ID: 123},
				From:           &tbapi.User{UserName: "new_user", ID: 999},
				LeftChatMember: &tbapi.User{UserName: "new_user", ID: 321},
				MessageID:      22,
			},
		}

		err := locator.AddMessage(ctx, "new_123_321", 123, 321, "", 21)
		require.NoError(t, err)

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err = l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.Empty(t, mockAPI.SendCalls())
		require.Len(t, mockAPI.RequestCalls(), 1)
		assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
		assert.Equal(t, 21, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	})

	t.Run("user has left the chat by admin, we have a message about joining, SuppressJoinMessage = false", func(t *testing.T) {
		mockAPI.ResetCalls()
		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat:           tbapi.Chat{ID: 123},
				From:           &tbapi.User{UserName: "new_user", ID: 999},
				LeftChatMember: &tbapi.User{UserName: "new_user", ID: 321},
				MessageID:      22,
			},
		}

		suppressJoinMessage := l.SuppressJoinMessage
		defer func() {
			l.SuppressJoinMessage = suppressJoinMessage
		}()
		l.SuppressJoinMessage = false

		err := locator.AddMessage(ctx, "new_123_321", 123, 321, "", 21)
		require.NoError(t, err)

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err = l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.Empty(t, mockAPI.SendCalls())
		require.Empty(t, mockAPI.RequestCalls())
	})

	t.Run("error from procLeftChatMemberMessage", func(t *testing.T) {
		mockAPI.ResetCalls()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat:           tbapi.Chat{ID: 123},
				From:           &tbapi.User{UserName: "new_user", ID: 999},
				LeftChatMember: &tbapi.User{UserName: "new_user", ID: 321},
				MessageID:      22,
			},
		}

		err := locator.AddMessage(ctx, "new_123_321", 123, 321, "", 21)
		require.NoError(t, err)

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }
		oldRequestFunc := mockAPI.RequestFunc
		mockAPI.RequestFunc = func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return nil, errors.New("some error")
		}
		defer func() {
			mockAPI.RequestFunc = oldRequestFunc
		}()

		err = l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.Empty(t, mockAPI.SendCalls())
		require.Len(t, mockAPI.RequestCalls(), 1)
		assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
		assert.Equal(t, 21, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	})

}
