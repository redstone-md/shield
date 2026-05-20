package events

import (
	"context"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"testing"
	"time"
)

func TestTelegramListener_DoWithBotBan(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "user"}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		t.Logf("on-message: %+v", msg)
		if msg.Text == "text 123" && msg.From.Username == "user" {
			return bot.Response{Send: true, Text: "bot's answer", BanInterval: 2 * time.Minute,
				User: bot.User{Username: "user", ID: 1}, CheckResults: []spamcheck.Response{
					{Name: "Check1", Spam: true, Details: "Details 1"}}}
		}
		if msg.From.Username == "ChannelBot" {
			return bot.Response{Send: true, Text: "bot's answer for channel", BanInterval: 2 * time.Minute, User: bot.User{Username: "user", ID: 1}, ChannelID: msg.SenderChat.ID}
		}
		if msg.From.Username == "admin" {
			return bot.Response{Send: true, Text: "bot's answer for admin", BanInterval: 2 * time.Minute, User: bot.User{Username: "user", ID: 1}, ChannelID: msg.ReplyTo.SenderChat.ID}
		}
		return bot.Response{}
	}}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        botMock,
		SuperUsers: SuperUsers{"admin"},
		Group:      "gr",
		Locator:    locator,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	t.Run("test ban of the user", func(t *testing.T) {
		mockLogger.ResetCalls()
		botMock.ResetCalls()
		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123},
				Text: "text 123",
				From: &tbapi.User{UserName: "user", ID: 123},
				Date: int(time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.Len(t, mockLogger.SaveCalls(), 1)
		assert.Equal(t, "text 123", mockLogger.SaveCalls()[0].Msg.Text)
		assert.Equal(t, "user", mockLogger.SaveCalls()[0].Msg.From.Username)
		assert.Len(t, mockAPI.SendCalls(), 1, "ban group message should be sent after a real user ban")
		assert.Equal(t, banGroupMessageText, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
		assert.Len(t, mockAPI.RequestCalls(), 1)
		assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.BanChatMemberConfig).ChatID)

		require.Len(t, botMock.OnMessageCalls(), 1)
		assert.Equal(t, "text 123", botMock.OnMessageCalls()[0].Msg.Text)
		assert.Equal(t, "user", botMock.OnMessageCalls()[0].Msg.From.Username)
		assert.False(t, botMock.OnMessageCalls()[0].CheckOnly)
	})

	t.Run("test ban of the channel", func(t *testing.T) {
		mockLogger.ResetCalls()
		botMock.ResetCalls()
		mockAPI.ResetCalls()
		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: 123},
				Text: "text 321",
				From: &tbapi.User{UserName: "ChannelBot", ID: 136817688},
				SenderChat: &tbapi.Chat{
					ID:       12345,
					UserName: "test_bot",
				},
				Date: int(time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.Len(t, mockLogger.SaveCalls(), 1)
		assert.Equal(t, "text 321", mockLogger.SaveCalls()[0].Msg.Text)
		assert.Equal(t, "ChannelBot", mockLogger.SaveCalls()[0].Msg.From.Username)
		assert.Equal(t, "user", mockLogger.SaveCalls()[0].Response.User.Username)
		assert.Equal(t, "bot's answer for channel", mockLogger.SaveCalls()[0].Response.Text)
		assert.Empty(t, mockAPI.SendCalls())
		assert.Len(t, mockAPI.RequestCalls(), 1)
		assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.BanChatSenderChatConfig).ChatID)
		assert.Equal(t, int64(12345), mockAPI.RequestCalls()[0].C.(tbapi.BanChatSenderChatConfig).SenderChatID)

		require.Len(t, botMock.OnMessageCalls(), 1)
		assert.Equal(t, "text 321", botMock.OnMessageCalls()[0].Msg.Text)
		assert.Equal(t, "ChannelBot", botMock.OnMessageCalls()[0].Msg.From.Username)
		assert.False(t, botMock.OnMessageCalls()[0].CheckOnly)
	})

	t.Run("test ban of the channel on behalf of the superuser", func(t *testing.T) {
		mockLogger.ResetCalls()
		mockAPI.ResetCalls()
		botMock.ResetCalls()
		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				ReplyToMessage: &tbapi.Message{
					SenderChat: &tbapi.Chat{
						ID:       54321,
						UserName: "original_bot",
					},
				},
				Chat: tbapi.Chat{ID: 123},
				Text: "text 543",
				From: &tbapi.User{UserName: "admin", ID: 555},
				Date: int(time.Date(2020, 2, 11, 19, 37, 55, 9, time.UTC).Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		require.Len(t, mockLogger.SaveCalls(), 1)
		assert.Equal(t, "text 543", mockLogger.SaveCalls()[0].Msg.Text)
		assert.Equal(t, "admin", mockLogger.SaveCalls()[0].Msg.From.Username)
		assert.Equal(t, "bot's answer for admin", mockLogger.SaveCalls()[0].Response.Text)
		assert.Empty(t, mockAPI.SendCalls())
		require.Empty(t, mockAPI.RequestCalls())

		require.Len(t, botMock.OnMessageCalls(), 1)
		assert.Equal(t, "text 543", botMock.OnMessageCalls()[0].Msg.Text)
		assert.Equal(t, "admin", botMock.OnMessageCalls()[0].Msg.From.Username)
		assert.False(t, botMock.OnMessageCalls()[0].CheckOnly)
	})

	t.Run("test spam check for forwarded message", func(t *testing.T) {
		mockLogger.ResetCalls()
		botMock.ResetCalls()
		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat:          tbapi.Chat{ID: 123},
				Text:          "text 123",
				From:          &tbapi.User{UserName: "user", ID: 123},
				Date:          int(time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()),
				ForwardOrigin: &tbapi.MessageOrigin{Date: time.Now().Unix()},
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.Len(t, mockLogger.SaveCalls(), 1)
		assert.Equal(t, "text 123", mockLogger.SaveCalls()[0].Msg.Text)
		assert.True(t, mockLogger.SaveCalls()[0].Msg.WithForward)
		require.Len(t, botMock.OnMessageCalls(), 1)
		assert.Equal(t, "text 123", botMock.OnMessageCalls()[0].Msg.Text)
		assert.True(t, botMock.OnMessageCalls()[0].Msg.WithForward)
	})

	t.Run("test spam check for edited message", func(t *testing.T) {
		mockLogger.ResetCalls()
		botMock.ResetCalls()
		updMsg := tbapi.Update{
			EditedMessage: &tbapi.Message{
				Chat:     tbapi.Chat{ID: 123},
				Text:     "edited spam message",
				From:     &tbapi.User{UserName: "edited_user", ID: 456},
				Date:     int(time.Now().Unix()),
				EditDate: int(time.Now().Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		botMock.OnMessageFunc = func(msg bot.Message, checkOnly bool) bot.Response {
			if msg.Text == "edited spam message" && msg.From.Username == "edited_user" {
				return bot.Response{Send: true, Text: "edited message is spam", BanInterval: 1 * time.Minute,
					User: bot.User{Username: "edited_user", ID: 456}, CheckResults: []spamcheck.Response{
						{Name: "EditedCheck", Spam: true, Details: "Edited message detected as spam"}}}
			}
			return bot.Response{}
		}

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.Len(t, mockLogger.SaveCalls(), 1)
		assert.Equal(t, "edited spam message", mockLogger.SaveCalls()[0].Msg.Text)
		assert.Equal(t, "edited_user", mockLogger.SaveCalls()[0].Msg.From.Username)
		assert.Equal(t, "edited message is spam", mockLogger.SaveCalls()[0].Response.Text)

		require.Len(t, botMock.OnMessageCalls(), 1)
		assert.Equal(t, "edited spam message", botMock.OnMessageCalls()[0].Msg.Text)
		assert.Equal(t, "edited_user", botMock.OnMessageCalls()[0].Msg.From.Username)
		assert.False(t, botMock.OnMessageCalls()[0].CheckOnly)
	})
}

func TestTelegramListener_DoWithBotSoftBan(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "user"}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		t.Logf("on-message: %+v", msg)
		if msg.Text == "text 123" && msg.From.Username == "user" {
			return bot.Response{Send: true, Text: "bot's answer", BanInterval: 2 * time.Minute,
				User: bot.User{Username: "user", ID: 1}, CheckResults: []spamcheck.Response{
					{Name: "Check1", Spam: true, Details: "Details 1"}}}
		}
		if msg.From.Username == "ChannelBot" {
			return bot.Response{Send: true, Text: "bot's answer for channel", BanInterval: 2 * time.Minute, User: bot.User{Username: "user", ID: 1}, ChannelID: msg.SenderChat.ID}
		}
		if msg.From.Username == "admin" {
			return bot.Response{Send: true, Text: "bot's answer for admin", BanInterval: 2 * time.Minute, User: bot.User{Username: "user", ID: 1}, ChannelID: msg.ReplyTo.SenderChat.ID}
		}
		return bot.Response{}
	}}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger:  mockLogger,
		TbAPI:       mockAPI,
		Bot:         botMock,
		SuperUsers:  SuperUsers{"admin"},
		Group:       "gr",
		Locator:     locator,
		SoftBanMode: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat: tbapi.Chat{ID: 123},
			Text: "text 123",
			From: &tbapi.User{UserName: "user", ID: 123},
			Date: int(time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()),
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	assert.Len(t, mockLogger.SaveCalls(), 1)
	assert.Equal(t, "text 123", mockLogger.SaveCalls()[0].Msg.Text)
	assert.Equal(t, "user", mockLogger.SaveCalls()[0].Msg.From.Username)
	assert.Len(t, mockAPI.SendCalls(), 1, "restriction group message should be sent after a soft-ban")
	assert.Equal(t, restrictGroupMessageText, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
	assert.Len(t, mockAPI.RequestCalls(), 1)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.RestrictChatMemberConfig).ChatID)
	assert.Equal(t, int64(1), mockAPI.RequestCalls()[0].C.(tbapi.RestrictChatMemberConfig).UserID)
	assert.Equal(t, &tbapi.ChatPermissions{}, mockAPI.RequestCalls()[0].C.(tbapi.RestrictChatMemberConfig).Permissions)

	require.Len(t, botMock.OnMessageCalls(), 1)
	assert.Equal(t, "text 123", botMock.OnMessageCalls()[0].Msg.Text)
	assert.Equal(t, "user", botMock.OnMessageCalls()[0].Msg.From.Username)
	assert.False(t, botMock.OnMessageCalls()[0].CheckOnly)
}

func TestTelegramListener_DoWithTraining(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "ChannelBot"}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		t.Logf("on-message: %+v", msg)
		return bot.Response{DeleteReplyTo: true, ReplyTo: msg.ID, ChannelID: msg.ChatID, BanInterval: time.Hour,
			Send: true, Text: "bot's answer", User: bot.User{Username: "user", ID: 1, DisplayName: "First Last"}}
	}}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger:   mockLogger,
		TbAPI:        mockAPI,
		Bot:          botMock,
		Group:        "gr",
		Locator:      locator,
		TrainingMode: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat: tbapi.Chat{ID: 123},
			Text: "text 321",
			From: &tbapi.User{UserName: "user", ID: 136817688},
			SenderChat: &tbapi.Chat{
				ID:       12345,
				UserName: "test_bot",
			},
			Date: int(time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()),
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	assert.Len(t, mockLogger.SaveCalls(), 1)
	assert.Equal(t, "text 321", mockLogger.SaveCalls()[0].Msg.Text)
	assert.Equal(t, "user", mockLogger.SaveCalls()[0].Msg.From.Username)
	assert.Empty(t, mockAPI.SendCalls(), "no messages should be sent in training mode")
	assert.Empty(t, mockAPI.RequestCalls())

	require.Len(t, botMock.OnMessageCalls(), 1)
	assert.Equal(t, "text 321", botMock.OnMessageCalls()[0].Msg.Text)
	assert.Equal(t, "user", botMock.OnMessageCalls()[0].Msg.From.Username)
	assert.False(t, botMock.OnMessageCalls()[0].CheckOnly)
}

func TestTelegramListener_DoDeleteMessages(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "ChannelBot"}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		t.Logf("on-message: %+v", msg)
		if msg.Text == "text 123" && msg.From.Username == "user" {
			return bot.Response{DeleteReplyTo: true, ReplyTo: msg.ID, ChannelID: msg.ChatID, BanInterval: time.Hour,
				Send: true, Text: "bot's answer", User: bot.User{Username: "user", ID: 1, DisplayName: "First Last"}}
		}
		return bot.Response{}
	}}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        botMock,
		Group:      "gr",
		Locator:    locator,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 321,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "text 123",
			From:      &tbapi.User{UserName: "user"},
			Date:      int(time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()),
			ReplyToMessage: &tbapi.Message{
				SenderChat: &tbapi.Chat{
					ID: 54321,
				},
			},
			SenderChat: &tbapi.Chat{ID: 54321},
		},
	}

	updChan := make(chan tbapi.Update, 2)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	require.Len(t, mockLogger.SaveCalls(), 1)
	assert.Equal(t, "text 123", mockLogger.SaveCalls()[0].Msg.Text)
	assert.Equal(t, "user", mockLogger.SaveCalls()[0].Msg.From.Username)

	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, 321, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)
}
