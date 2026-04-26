package events

import (
	"context"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/app/rules"
	"testing"
	"time"
)

func TestTelegramListener_PrivateChatStoresUser(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return []tbapi.ChatMember{}, nil
		},
	}

	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		t.Fatal("bot.OnMessage should not be called for private chat messages")
		return bot.Response{}
	}}

	locatorMock := &mocks.LocatorMock{
		AddMessageFunc: func(ctx context.Context, msg string, chatID, userID int64, userName string, msgID int) error {
			t.Fatal("locator.AddMessage should not be called for private chat messages")
			return nil
		},
	}

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        botMock,
		Group:      "123",
		Locator:    locatorMock,
	}

	t.Run("private chat message stores user and returns nil", func(t *testing.T) {
		update := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: 999, Type: "private"},
				Text: "hello bot",
				From: &tbapi.User{
					ID:        42,
					UserName:  "testuser",
					FirstName: "Test",
					LastName:  "User",
				},
				Date: int(time.Now().Unix()),
			},
		}

		err := l.procEvents(update)
		require.NoError(t, err)

		users := l.GetDMUsers()
		require.Len(t, users, 1)
		assert.Equal(t, int64(42), users[0].UserID)
		assert.Equal(t, "testuser", users[0].UserName)
		assert.Equal(t, "Test User", users[0].DisplayName)
	})

	t.Run("private chat does not reach spam checking", func(t *testing.T) {
		update := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: 888, Type: "private"},
				Text: "buy crypto now!!!",
				From: &tbapi.User{
					ID:        100,
					UserName:  "spammer",
					FirstName: "Spammer",
				},
				Date: int(time.Now().Unix()),
			},
		}

		err := l.procEvents(update)
		require.NoError(t, err)

		assert.Empty(t, botMock.OnMessageCalls())

		assert.Empty(t, locatorMock.AddMessageCalls())
	})

	t.Run("first name only display name", func(t *testing.T) {

		l2 := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			Group:      "123",
			Locator:    locatorMock,
		}

		update := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: 777, Type: "private"},
				Text: "hi",
				From: &tbapi.User{
					ID:        200,
					UserName:  "alice",
					FirstName: "Alice",
				},
				Date: int(time.Now().Unix()),
			},
		}

		err := l2.procEvents(update)
		require.NoError(t, err)

		users := l2.GetDMUsers()
		require.Len(t, users, 1)
		assert.Equal(t, "Alice", users[0].DisplayName)
	})

	t.Run("nil From in private chat does not panic", func(t *testing.T) {
		l3 := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			Group:      "123",
			Locator:    locatorMock,
		}

		update := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: 666, Type: "private"},
				Text: "hello",
				From: nil,
				Date: int(time.Now().Unix()),
			},
		}

		err := l3.procEvents(update)
		require.NoError(t, err)
		assert.Empty(t, l3.GetDMUsers())
	})
}

func TestTelegramListener_PrivateChatViaDoLoop(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return []tbapi.ChatMember{}, nil
		},
	}

	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		if msg.Text == "idle" {
			return bot.Response{}
		}
		t.Fatal("bot.OnMessage should not be called for private chat messages via Do loop")
		return bot.Response{}
	}}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        botMock,
		Group:      "123",
		Locator:    locator,
	}

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat: tbapi.Chat{ID: 555, Type: "private"},
			Text: "hello from DM",
			From: &tbapi.User{
				ID:        77,
				UserName:  "dmuser",
				FirstName: "DM",
				LastName:  "User",
			},
			Date: int(time.Now().Unix()),
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(context.Background())
	require.EqualError(t, err, "telegram update chan closed")

	users := l.GetDMUsers()
	require.Len(t, users, 1)
	assert.Equal(t, int64(77), users[0].UserID)
	assert.Equal(t, "dmuser", users[0].UserName)
	assert.Equal(t, "DM User", users[0].DisplayName)
}

func TestTelegramListener_DMUsersMethods(t *testing.T) {
	l := TelegramListener{}
	users := l.GetDMUsers()
	assert.Empty(t, users)
	assert.NotNil(t, users)
}

func TestTelegramListener_ApplyRuleSet(t *testing.T) {
	l := &TelegramListener{
		RuleSetVersion: 1,
		ModerationConfig: ModerationConfig{
			FirstStrike:  10 * time.Minute,
			SecondStrike: time.Hour,
		},
		ReportConfig: ReportConfig{
			Storage:   nil,
			Enabled:   true,
			Threshold: 2,
		},
		SoftBanMode:  false,
		Dry:          false,
		TrainingMode: false,
	}

	rs := rules.RuleSet{
		Version: 3,
		Moderation: rules.ModerationRules{
			FirstStrike:  5 * time.Minute,
			SecondStrike: 30 * time.Minute,
			SoftBan:      true,
			DryRun:       true,
		},
		Reports: rules.ReportRules{
			Enabled:          false,
			Threshold:        5,
			AutoBanThreshold: 10,
			RateLimit:        3,
			RatePeriod:       2 * time.Minute,
		},
	}

	l.ApplyRuleSet(rs)

	assert.Equal(t, 3, l.RuleSetVersion)
	assert.Equal(t, 5*time.Minute, l.ModerationConfig.FirstStrike)
	assert.Equal(t, 30*time.Minute, l.ModerationConfig.SecondStrike)
	assert.False(t, l.ReportConfig.Enabled)
	assert.Equal(t, 5, l.ReportConfig.Threshold)
	assert.True(t, l.SoftBanMode)
	assert.True(t, l.Dry)
}

func TestTelegramListener_ApplyRuleSet_propagatesToSubHandlers(t *testing.T) {
	l := &TelegramListener{
		RuleSetVersion: 1,
		ModerationConfig: ModerationConfig{
			FirstStrike:  10 * time.Minute,
			SecondStrike: time.Hour,
		},
		ReportConfig: ReportConfig{
			Storage:   nil,
			Enabled:   true,
			Threshold: 2,
		},
		SoftBanMode:  false,
		Dry:          false,
		TrainingMode: false,
	}

	l.adminHandler = &admin{
		softBan: false,
		dry:     false,
	}
	l.reportsHandler = &userReports{
		ReportConfig: ReportConfig{Enabled: true, Threshold: 2},
		moderation:   ModerationConfig{FirstStrike: 10 * time.Minute, SecondStrike: time.Hour},
		softBanMode:  false,
		dry:          false,
	}

	rs := rules.RuleSet{
		Version: 2,
		Moderation: rules.ModerationRules{
			FirstStrike:  1 * time.Minute,
			SecondStrike: 5 * time.Minute,
			SoftBan:      true,
			DryRun:       true,
		},
		Reports: rules.ReportRules{
			Enabled:   false,
			Threshold: 7,
		},
	}

	l.ApplyRuleSet(rs)

	assert.True(t, l.adminHandler.softBan)
	assert.True(t, l.adminHandler.dry)
	assert.False(t, l.reportsHandler.ReportConfig.Enabled)
	assert.Equal(t, 7, l.reportsHandler.ReportConfig.Threshold)
	assert.Equal(t, 1*time.Minute, l.reportsHandler.moderation.FirstStrike)
	assert.True(t, l.reportsHandler.softBanMode)
	assert.True(t, l.reportsHandler.dry)
}
