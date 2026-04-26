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

func TestTelegramListener_AnonymousAdminPostSkipsSpamCheck(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: -1001688024850}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "user"}}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return []tbapi.ChatMember{}, nil
		},
	}

	t.Run("anonymous admin post from group itself should skip spam check", func(t *testing.T) {
		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			t.Fatalf("bot.OnMessage should not be called for anonymous admin post from group itself")
			return bot.Response{}
		}}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			Group:      "-1001688024850",
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: -1001688024850},
				Text: "test for spam please ignore this message",
				From: &tbapi.User{ID: 1087968824, UserName: "GroupAnonymousBot", FirstName: "Group"},
				SenderChat: &tbapi.Chat{
					ID:   -1001688024850,
					Type: "supergroup",
				},
				Date: int(time.Now().Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")

		assert.Empty(t, botMock.OnMessageCalls())

		assert.Empty(t, mockLogger.SaveCalls())
	})

	t.Run("channel auto-forward should run spam check", func(t *testing.T) {
		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{
				Send:          true,
				Text:          "this is spam",
				BanInterval:   2 * time.Minute,
				User:          bot.User{ID: 777000, Username: "Telegram"},
				ChannelID:     0,
				ReplyTo:       msg.ID,
				DeleteReplyTo: true,
			}
		}}

		mockAPI.RequestFunc = func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			Group:      "-1001688024850",
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: -1001688024850},
				Text: "event announcement with lots of emojis",
				From: &tbapi.User{ID: 777000, FirstName: "Telegram"},
				SenderChat: &tbapi.Chat{
					ID:       -1001261918100,
					Type:     "channel",
					UserName: "esnlausanne",
				},
				Date: int(time.Now().Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")

		assert.Len(t, botMock.OnMessageCalls(), 1)
		assert.Equal(t, "event announcement with lots of emojis", botMock.OnMessageCalls()[0].Msg.Text)
	})

	t.Run("channel spam uses channel ID for locator", func(t *testing.T) {
		locatorMock := &mocks.LocatorMock{
			AddMessageFunc: func(ctx context.Context, msg string, chatID, userID int64, userName string, msgID int) error {
				return nil
			},
			AddSpamFunc: func(ctx context.Context, userID int64, checks []spamcheck.Response) error {
				return nil
			},
		}

		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{
				Send: true, Text: "this is spam", BanInterval: 2 * time.Minute,
				User:          bot.User{ID: 777000, Username: "Telegram"},
				ChannelID:     -1001261918100,
				ReplyTo:       msg.ID,
				DeleteReplyTo: true,
				CheckResults:  []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}},
			}
		}}

		mockAPI.RequestFunc = func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		}

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			Group:      "-1001688024850",
			Locator:    locatorMock,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: -1001688024850},
				Text: "channel spam message",
				From: &tbapi.User{ID: 777000, FirstName: "Telegram"},
				SenderChat: &tbapi.Chat{
					ID:       -1001261918100,
					Type:     "channel",
					UserName: "esnlausanne",
				},
				Date: int(time.Now().Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")

		require.Len(t, locatorMock.AddMessageCalls(), 1)
		assert.Equal(t, int64(-1001261918100), locatorMock.AddMessageCalls()[0].UserID)
		assert.Equal(t, "esnlausanne", locatorMock.AddMessageCalls()[0].UserName)

		require.Len(t, locatorMock.AddSpamCalls(), 1)
		assert.Equal(t, int64(-1001261918100), locatorMock.AddSpamCalls()[0].UserID)
	})

	t.Run("regular user message should run spam check", func(t *testing.T) {
		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{
				Send:          true,
				Text:          "this is spam",
				BanInterval:   2 * time.Minute,
				User:          bot.User{ID: 12345, Username: "spammer"},
				ReplyTo:       msg.ID,
				DeleteReplyTo: true,
			}
		}}

		mockAPI.RequestFunc = func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			Group:      "-1001688024850",
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: -1001688024850},
				Text: "buy cheap products here",
				From: &tbapi.User{ID: 12345, UserName: "spammer"},
				Date: int(time.Now().Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")

		assert.Len(t, botMock.OnMessageCalls(), 1)
		assert.Equal(t, "buy cheap products here", botMock.OnMessageCalls()[0].Msg.Text)
	})

	t.Run("anonymous admin post in testing chat should skip spam check", func(t *testing.T) {
		mockLogger.ResetCalls()

		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			t.Fatalf("bot.OnMessage should not be called for anonymous admin post from testing chat")
			return bot.Response{}
		}}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		testingChatID := int64(-1002345678901)

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			Group:      "-1001688024850",
			TestingIDs: []int64{testingChatID},
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: testingChatID},
				Text: "test message in testing chat",
				From: &tbapi.User{ID: 1087968824, UserName: "GroupAnonymousBot", FirstName: "Group"},
				SenderChat: &tbapi.Chat{
					ID:   testingChatID,
					Type: "supergroup",
				},
				Date: int(time.Now().Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")

		assert.Empty(t, botMock.OnMessageCalls())

		assert.Empty(t, mockLogger.SaveCalls())
	})
}
