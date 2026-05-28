package events

import (
	"context"
	"errors"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/slowpath"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			Group:      "gr",
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
			Group:      "gr",
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
			Group:      "gr",
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
					Group:      "gr",
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
			Group:      "gr",
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

	t.Run("reply /report from superuser is direct spam report alias", func(t *testing.T) {
		mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "bot"}}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return []tbapi.ChatMember{}, nil
			},
		}
		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{Send: true, Text: "diagnostic"}
			},
			UpdateSpamFunc: func(msg string) error { return nil },
		}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger: mockLogger,
			Group:      "gr",
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
		require.Len(t, botMock.OnMessageCalls(), 1)
		assert.Equal(t, "reported spam text", botMock.OnMessageCalls()[0].Msg.Text)
		assert.True(t, botMock.OnMessageCalls()[0].CheckOnly)
		require.Len(t, botMock.UpdateSpamCalls(), 1)
		assert.Equal(t, "reported spam text", botMock.UpdateSpamCalls()[0].Msg)
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

func TestTelegramListener_isChatReplyTrigger(t *testing.T) {
	botMsg := &tbapi.Message{From: &tbapi.User{UserName: "mybot", IsBot: true}, MessageID: 10}
	tests := []struct {
		name string
		upd  tbapi.Update
		want bool
	}{
		{name: "reply to bot", upd: tbapi.Update{Message: &tbapi.Message{ReplyToMessage: botMsg}}, want: true},
		{name: "reply to non bot", upd: tbapi.Update{Message: &tbapi.Message{ReplyToMessage: &tbapi.Message{From: &tbapi.User{UserName: "user"}}}}, want: false},
		{name: "keyword", upd: tbapi.Update{Message: &tbapi.Message{Text: "эй железяка, ответь"}}, want: true},
		{name: "no trigger", upd: tbapi.Update{Message: &tbapi.Message{Text: "привет"}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &TelegramListener{BotUsername: "mybot"}
			assert.Equal(t, tt.want, l.isChatReplyTrigger(tt.upd))
		})
	}
}

func TestTelegramListener_handleChatReplyUsesSlowPathEngine(t *testing.T) {
	var deleted int
	var sent []tbapi.Chattable
	chatEngine := &slowPathChatEngineStub{replyFunc: func(ctx context.Context, req slowpath.ChatRequest) (*slowpath.ChatResult, error) {
		assert.Equal(t, "tg-spam", req.TenantID)
		assert.Equal(t, "hello bot", req.Message)
		assert.Equal(t, "alice", req.History[0].UserName)
		assert.Equal(t, "previous", req.History[0].Text)
		return &slowpath.ChatResult{Text: "answer"}, nil
	}}
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			sent = append(sent, c)
			if msg, ok := c.(tbapi.MessageConfig); ok {
				return tbapi.Message{MessageID: 99, Text: msg.Text}, nil
			}
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			if _, ok := c.(tbapi.DeleteMessageConfig); ok {
				deleted++
			}
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	l := TelegramListener{
		TbAPI:              mockAPI,
		Group:      "gr",
		SlowPathChatEngine: chatEngine,
		TenantID:           "tg-spam",
		SuperUsers:         SuperUsers{"super"},
	}

	l.handleChatReply(context.Background(), tbapi.Update{Message: &tbapi.Message{
		MessageID: 17,
		Chat:      tbapi.Chat{ID: 123},
		Text:      "hello bot",
		From:      &tbapi.User{UserName: "alice", ID: 1},
		Date:      int(time.Now().Unix()),
		ReplyToMessage: &tbapi.Message{
			MessageID: 16,
			Text:      "previous",
			From:      &tbapi.User{UserName: "alice"},
		},
	}})

	require.Len(t, sent, 1)
	require.Len(t, chatEngine.calls, 1)
	assert.Equal(t, "answer", sent[0].(tbapi.MessageConfig).Text)
	assert.Zero(t, deleted)
}

func TestTelegramListener_handleChatReplyStripsThoughtTags(t *testing.T) {
	var sent []tbapi.Chattable
	chatEngine := &slowPathChatEngineStub{replyFunc: func(ctx context.Context, req slowpath.ChatRequest) (*slowpath.ChatResult, error) {
		return &slowpath.ChatResult{Text: "<thought>private\nreasoning</thought> visible answer"}, nil
	}}
	mockAPI := &mocks.TbAPIMock{SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
		sent = append(sent, c)
		return tbapi.Message{MessageID: 99}, nil
	}}
	l := TelegramListener{TbAPI: mockAPI, SlowPathChatEngine: chatEngine, TenantID: "tg-spam", SuperUsers: SuperUsers{"super"}}

	l.handleChatReply(context.Background(), tbapi.Update{Message: &tbapi.Message{
		MessageID: 17,
		Chat:      tbapi.Chat{ID: 123},
		Text:      "hello bot",
		From:      &tbapi.User{UserName: "alice", ID: 1},
		Date:      int(time.Now().Unix()),
	}})

	require.Len(t, sent, 1)
	assert.Equal(t, "visible answer", sent[0].(tbapi.MessageConfig).Text)
}

func TestTelegramListener_handleChatReplyRateLimits(t *testing.T) {
	var sent []string
	var deleted int
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if msg, ok := c.(tbapi.MessageConfig); ok {
				sent = append(sent, msg.Text)
				return tbapi.Message{MessageID: 77, Text: msg.Text}, nil
			}
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			if _, ok := c.(tbapi.DeleteMessageConfig); ok {
				deleted++
			}
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}
	l := TelegramListener{
		TbAPI: mockAPI,
		Group:      "gr",
		SlowPathChatEngine: &slowPathChatEngineStub{replyFunc: func(ctx context.Context, req slowpath.ChatRequest) (*slowpath.ChatResult, error) {
			return &slowpath.ChatResult{Text: "answer"}, nil
		}},
		TenantID:   "tg-spam",
		SuperUsers: SuperUsers{"super"},
		chatLimiter: &chatRateLimiter{entries: map[int64][]time.Time{42: {
			time.Now().UTC().Add(-10 * time.Second),
			time.Now().UTC().Add(-20 * time.Second),
			time.Now().UTC().Add(-30 * time.Second),
			time.Now().UTC().Add(-40 * time.Second),
			time.Now().UTC().Add(-50 * time.Second),
		}}},
	}

	l.handleChatReply(context.Background(), tbapi.Update{Message: &tbapi.Message{
		MessageID: 18,
		Chat:      tbapi.Chat{ID: 123},
		Text:      "hello bot",
		From:      &tbapi.User{UserName: "alice", ID: 42},
		Date:      int(time.Now().Unix()),
	}})

	assert.Equal(t, []string{chatLimitWarningText}, sent)
	assert.Equal(t, 1, deleted)
}

func TestTelegramListener_handleChatReplySuperuserBypassesLimit(t *testing.T) {
	var calls int
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if msg, ok := c.(tbapi.MessageConfig); ok {
				return tbapi.Message{MessageID: 55, Text: msg.Text}, nil
			}
			return tbapi.Message{}, nil
		},
	}
	l := TelegramListener{
		TbAPI:      mockAPI,
		Group:      "gr",
		TenantID:   "tg-spam",
		SuperUsers: SuperUsers{"super"},
		SlowPathChatEngine: &slowPathChatEngineStub{replyFunc: func(ctx context.Context, req slowpath.ChatRequest) (*slowpath.ChatResult, error) {
			calls++
			return &slowpath.ChatResult{Text: fmt.Sprintf("reply-%d", calls)}, nil
		}},
		chatLimiter: &chatRateLimiter{entries: map[int64][]time.Time{42: {
			time.Now().UTC().Add(-10 * time.Second),
			time.Now().UTC().Add(-20 * time.Second),
			time.Now().UTC().Add(-30 * time.Second),
			time.Now().UTC().Add(-40 * time.Second),
			time.Now().UTC().Add(-50 * time.Second),
		}}},
	}

	l.handleChatReply(context.Background(), tbapi.Update{Message: &tbapi.Message{
		MessageID: 19,
		Chat:      tbapi.Chat{ID: 123},
		Text:      "hello bot",
		From:      &tbapi.User{UserName: "super", ID: 42},
		Date:      int(time.Now().Unix()),
	}})

	assert.Equal(t, 1, calls)
}

func TestReplyChatWithRetryRetriesTransientErrors(t *testing.T) {
	var attempts int
	var sleeps []time.Duration
	chatEngine := &slowPathChatEngineStub{replyFunc: func(ctx context.Context, req slowpath.ChatRequest) (*slowpath.ChatResult, error) {
		attempts++
		if attempts < 3 {
			return nil, fmt.Errorf("openai chat reply: %w", &openai.APIError{HTTPStatusCode: 502, HTTPStatus: "502 Bad Gateway"})
		}
		return &slowpath.ChatResult{Text: "answer"}, nil
	}}

	result, err := replyChatWithRetry(context.Background(), chatEngine, slowpath.ChatRequest{Message: "hello"}, func(d time.Duration) {
		sleeps = append(sleeps, d)
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "answer", result.Text)
	assert.Equal(t, 3, attempts)
	assert.Equal(t, []time.Duration{3 * time.Second, 3 * time.Second}, sleeps)
}

func TestReplyChatWithRetryDoesNotRetryPermanentErrors(t *testing.T) {
	var attempts int
	wantErr := errors.New("bad request")
	chatEngine := &slowPathChatEngineStub{replyFunc: func(ctx context.Context, req slowpath.ChatRequest) (*slowpath.ChatResult, error) {
		attempts++
		return nil, fmt.Errorf("openai chat reply: %w", &openai.APIError{HTTPStatusCode: 400, HTTPStatus: "400 Bad Request", Message: wantErr.Error()})
	}}

	result, err := replyChatWithRetry(context.Background(), chatEngine, slowpath.ChatRequest{Message: "hello"}, func(d time.Duration) {
		t.Fatal("sleep should not be called for permanent errors")
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 1, attempts)
}

type slowPathChatEngineStub struct {
	replyFunc func(ctx context.Context, req slowpath.ChatRequest) (*slowpath.ChatResult, error)
	calls     []slowpath.ChatRequest
}

func (s *slowPathChatEngineStub) Reply(ctx context.Context, req slowpath.ChatRequest) (*slowpath.ChatResult, error) {
	s.calls = append(s.calls, req)
	return s.replyFunc(ctx, req)
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
