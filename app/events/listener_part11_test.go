package events

import (
	"context"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"testing"
	"time"
)

func TestTelegramListener_OrphanedReportDeletion(t *testing.T) {
	t.Run("deletes orphaned /report from regular user", func(t *testing.T) {
		mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
		deleteCalled := false
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				if deleteMsg, ok := c.(tbapi.DeleteMessageConfig); ok {
					assert.Equal(t, int64(123), deleteMsg.ChatID)
					assert.Equal(t, 100, deleteMsg.MessageID)
					deleteCalled = true
				}
				return &tbapi.APIResponse{Ok: true}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{}, nil
			},
		}
		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{}
		}}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			SuperUsers: SuperUsers{"super"},
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 100,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "/report",
				From:      &tbapi.User{UserName: "regular_user", ID: 999},
				Date:      int(time.Now().Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.True(t, deleteCalled, "orphaned /report should be deleted")
		assert.Empty(t, botMock.OnMessageCalls(), "bot.OnMessage should not be called for orphaned /report")
	})

	t.Run("deletes orphaned report (without slash)", func(t *testing.T) {
		mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
		deleteCalled := false
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				if _, ok := c.(tbapi.DeleteMessageConfig); ok {
					deleteCalled = true
				}
				return &tbapi.APIResponse{Ok: true}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{}, nil
			},
		}
		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{}
		}}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			SuperUsers: SuperUsers{"super"},
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 100,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "report",
				From:      &tbapi.User{UserName: "regular_user", ID: 999},
				Date:      int(time.Now().Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.True(t, deleteCalled, "orphaned report should be deleted")
	})

	t.Run("deletes /report@botname syntax", func(t *testing.T) {
		mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
		deleteCalled := false
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				if _, ok := c.(tbapi.DeleteMessageConfig); ok {
					deleteCalled = true
				}
				return &tbapi.APIResponse{Ok: true}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{}, nil
			},
		}
		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{}
		}}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger:  mockLogger,
			TbAPI:       mockAPI,
			Bot:         botMock,
			BotUsername: "some_bot",
			SuperUsers:  SuperUsers{"super"},
			Locator:     locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 100,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "/report@some_bot",
				From:      &tbapi.User{UserName: "regular_user", ID: 999},
				Date:      int(time.Now().Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.True(t, deleteCalled, "/report@botname should be deleted")
	})

	t.Run("does NOT delete messages that start with 'report' but are not commands", func(t *testing.T) {
		testCases := []string{
			"reporter arrived",
			"report on the meeting",
			"reporting the issue",
			"reports are ready",
			"/report_spam",
		}

		for _, text := range testCases {
			t.Run(text, func(t *testing.T) {
				mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
				deleteCalled := false
				mockAPI := &mocks.TbAPIMock{
					GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
						return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
					},
					RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
						if _, ok := c.(tbapi.DeleteMessageConfig); ok {
							deleteCalled = true
						}
						return &tbapi.APIResponse{Ok: true}, nil
					},
					GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
						return []tbapi.ChatMember{}, nil
					},
				}
				botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
					return bot.Response{}
				}}

				locator, teardown := prepTestLocator(t)
				defer teardown()

				l := TelegramListener{
					SpamLogger: mockLogger,
					TbAPI:      mockAPI,
					Bot:        botMock,
					SuperUsers: SuperUsers{"super"},
					Locator:    locator,
				}

				ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				defer cancel()

				updMsg := tbapi.Update{
					Message: &tbapi.Message{
						MessageID: 100,
						Chat:      tbapi.Chat{ID: 123},
						Text:      text,
						From:      &tbapi.User{UserName: "regular_user", ID: 999},
						Date:      int(time.Now().Unix()),
					},
				}

				updChan := make(chan tbapi.Update, 1)
				updChan <- updMsg
				close(updChan)
				mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

				err := l.Do(ctx)
				require.EqualError(t, err, "telegram update chan closed")
				assert.False(t, deleteCalled, "message starting with 'report' but not a command should NOT be deleted")
				assert.Len(t, botMock.OnMessageCalls(), 1, "bot.OnMessage should be called for non-command messages")
			})
		}
	})

	t.Run("does NOT delete orphaned /report from superuser", func(t *testing.T) {
		mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
		deleteCalled := false
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				if _, ok := c.(tbapi.DeleteMessageConfig); ok {
					deleteCalled = true
				}
				return &tbapi.APIResponse{Ok: true}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{}, nil
			},
		}
		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{}
		}}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			SuperUsers: SuperUsers{"superuser"},
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 100,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "/report",
				From:      &tbapi.User{UserName: "superuser", ID: 888},
				Date:      int(time.Now().Unix()),
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.False(t, deleteCalled, "superuser orphaned /report should NOT be auto-deleted")
	})

	t.Run("reply /report from superuser is handled before spam check", func(t *testing.T) {
		mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{}, nil
			},
		}
		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{Send: true, Text: "should not run"}
		}}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			SuperUsers: SuperUsers{"superuser"},
			Locator:    locator,
			ReportConfig: ReportConfig{
				Enabled: true,
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 100,
				Chat:      tbapi.Chat{ID: 123},
				Text:      "/report",
				From:      &tbapi.User{UserName: "superuser", ID: 888},
				Date:      int(time.Now().Unix()),
				ReplyToMessage: &tbapi.Message{
					MessageID: 99,
					Chat:      tbapi.Chat{ID: 123},
					Text:      "reported spam text",
					From:      &tbapi.User{UserName: "spammer", ID: 999},
					Date:      int(time.Now().Unix()),
				},
			},
		}

		updChan := make(chan tbapi.Update, 1)
		updChan <- updMsg
		close(updChan)
		mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

		err := l.Do(ctx)
		require.EqualError(t, err, "telegram update chan closed")
		assert.Empty(t, botMock.OnMessageCalls(), "superuser reply /report should not reach spam check")
	})
}

func TestTelegramListener_isReportCommand(t *testing.T) {
	tests := []struct {
		name        string
		botUsername string
		text        string
		want        bool
	}{

		{name: "plain report", botUsername: "mybot", text: "report", want: true},
		{name: "plain /report", botUsername: "mybot", text: "/report", want: true},
		{name: "plain REPORT uppercase", botUsername: "mybot", text: "REPORT", want: true},
		{name: "plain /REPORT uppercase", botUsername: "mybot", text: "/REPORT", want: true},
		{name: "plain report with trailing space", botUsername: "mybot", text: "report ", want: true},
		{name: "plain /report with leading space", botUsername: "mybot", text: " /report", want: true},
		{name: "plain /report with both spaces", botUsername: "mybot", text: " /report ", want: true},

		{name: "plain /report with empty botUsername", botUsername: "", text: "/report", want: true},
		{name: "plain report with empty botUsername", botUsername: "", text: "report", want: true},

		{name: "exact match /report@mybot", botUsername: "mybot", text: "/report@mybot", want: true},
		{name: "case insensitive /report@MyBot", botUsername: "mybot", text: "/report@MyBot", want: true},
		{name: "case insensitive /report@MYBOT", botUsername: "mybot", text: "/report@MYBOT", want: true},
		{name: "mixed case bot /report@mybot", botUsername: "MyBot", text: "/report@mybot", want: true},

		{name: "with extra text /report@mybot spam", botUsername: "mybot", text: "/report@mybot this is spam", want: true},
		{name: "with extra text case insensitive", botUsername: "mybot", text: "/report@MyBot extra text here", want: true},

		{name: "wrong bot /report@wrongbot", botUsername: "mybot", text: "/report@wrongbot", want: false},
		{name: "wrong bot /report@anotherbot", botUsername: "mybot", text: "/report@anotherbot", want: false},
		{name: "wrong bot with extra text", botUsername: "mybot", text: "/report@wrongbot spam", want: false},

		{name: "empty username /report@", botUsername: "mybot", text: "/report@", want: false},
		{name: "space after @ /report@ ", botUsername: "mybot", text: "/report@ ", want: false},
		{name: "tab after @ /report@\\t", botUsername: "mybot", text: "/report@\t", want: false},
		{name: "newline after @ /report@\\n", botUsername: "mybot", text: "/report@\n", want: false},
		{name: "space after @ with text /report@ text", botUsername: "mybot", text: "/report@ text", want: false},
		{name: "double @@ /report@@mybot", botUsername: "mybot", text: "/report@@mybot", want: false},

		{name: "@ command with empty botUsername", botUsername: "", text: "/report@somebot", want: false},
		{name: "@ command exact match but empty botUsername", botUsername: "", text: "/report@", want: false},

		{name: "not a report command /spam", botUsername: "mybot", text: "/spam", want: false},
		{name: "report substring reportthis", botUsername: "mybot", text: "reportthis", want: false},
		{name: "empty string", botUsername: "mybot", text: "", want: false},
		{name: "just spaces", botUsername: "mybot", text: "   ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &TelegramListener{
				BotUsername: tt.botUsername,
			}
			got := l.isReportCommand(tt.text)
			assert.Equal(t, tt.want, got, "isReportCommand(%q) with botUsername=%q", tt.text, tt.botUsername)
		})
	}
}

func TestTelegramListener_isBotMention(t *testing.T) {
	tests := []struct {
		name        string
		botUsername string
		text        string
		want        bool
	}{
		{name: "simple mention", botUsername: "mybot", text: "@mybot проверь", want: true},
		{name: "mention with punctuation", botUsername: "mybot", text: "@mybot, проверь это", want: true},
		{name: "case insensitive", botUsername: "MyBot", text: "эй @mybot глянь", want: true},
		{name: "no bot configured", botUsername: "", text: "@mybot help", want: false},
		{name: "different bot", botUsername: "mybot", text: "@otherbot help", want: false},
		{name: "empty text", botUsername: "mybot", text: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &TelegramListener{BotUsername: tt.botUsername}
			assert.Equal(t, tt.want, l.isBotMention(tt.text))
		})
	}
}

func TestTelegramListener_IsLinkedChannel(t *testing.T) {
	tests := []struct {
		name            string
		linkedChannelID int64
		msg             *tbapi.Message
		expected        bool
	}{
		{
			name:            "matching sender chat",
			linkedChannelID: -1001234567890,
			msg: &tbapi.Message{
				SenderChat: &tbapi.Chat{ID: -1001234567890},
			},
			expected: true,
		},
		{
			name:            "non-matching sender chat",
			linkedChannelID: -1001234567890,
			msg: &tbapi.Message{
				SenderChat: &tbapi.Chat{ID: -1009999999999},
			},
			expected: false,
		},
		{
			name:            "nil sender chat",
			linkedChannelID: -1001234567890,
			msg:             &tbapi.Message{},
			expected:        false,
		},
		{
			name:            "zero linked channel ID",
			linkedChannelID: 0,
			msg: &tbapi.Message{
				SenderChat: &tbapi.Chat{ID: -1001234567890},
			},
			expected: false,
		},
		{
			name:            "zero linked channel ID and nil sender chat",
			linkedChannelID: 0,
			msg:             &tbapi.Message{},
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &TelegramListener{linkedChannelID: tt.linkedChannelID}
			assert.Equal(t, tt.expected, l.isLinkedChannel(tt.msg))
		})
	}
}
