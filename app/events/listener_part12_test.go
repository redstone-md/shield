package events

import (
	"context"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"strings"
	"testing"
	"time"
)

func TestTelegramListener_LinkedChannelBanSpam(t *testing.T) {
	const (
		groupChatID     = int64(-1001688024850)
		linkedChannelID = int64(-1001234567890)
		channelBotID    = int64(136817688)
		targetUserID    = int64(666)
	)

	tests := []struct {
		name           string
		command        string
		wantBan        bool
		wantSpamTrain  bool
		wantWarnMsg    bool
		wantOnMessage  bool // whether bot.OnMessage should be called (for diagnostics)
		wantDeleteOrig bool // whether original replied-to message should be deleted
	}{
		{
			name:           "linked channel /ban command bans target user",
			command:        "/ban",
			wantBan:        true,
			wantSpamTrain:  false,
			wantWarnMsg:    false,
			wantOnMessage:  true,
			wantDeleteOrig: true,
		},
		{
			name:           "linked channel /spam command bans and trains",
			command:        "/spam",
			wantBan:        true,
			wantSpamTrain:  true,
			wantWarnMsg:    false,
			wantOnMessage:  true,
			wantDeleteOrig: true,
		},
		{
			name:           "linked channel /warn command sends warning",
			command:        "/warn",
			wantBan:        false,
			wantSpamTrain:  false,
			wantWarnMsg:    true,
			wantOnMessage:  false,
			wantDeleteOrig: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
			mockAPI := &mocks.TbAPIMock{
				GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
					return tbapi.ChatFullInfo{
						Chat:         tbapi.Chat{ID: groupChatID},
						LinkedChatID: linkedChannelID,
					}, nil
				},
				SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
					return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "bot"}}, nil
				},
				RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
					return &tbapi.APIResponse{Ok: true}, nil
				},
				GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
					return nil, nil
				},
			}
			botMock := &mocks.BotMock{
				OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
					return bot.Response{Send: true, Text: "detected spam"}
				},
				UpdateSpamFunc:         func(msg string) error { return nil },
				RemoveApprovedUserFunc: func(id int64) error { return nil },
			}

			locator, teardown := prepTestLocator(t)
			defer teardown()

			l := TelegramListener{
				SpamLogger: mockLogger,
				TbAPI:      mockAPI,
				Bot:        botMock,
				Group:      fmt.Sprintf("%d", groupChatID),
				Locator:    locator,
				SuperUsers: SuperUsers{},
				WarnMsg:    "You have been warned",
			}

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			updMsg := tbapi.Update{
				Message: &tbapi.Message{
					Chat: tbapi.Chat{ID: groupChatID},
					Text: tt.command,
					From: &tbapi.User{ID: channelBotID, UserName: "Channel_Bot"},
					SenderChat: &tbapi.Chat{
						ID:       linkedChannelID,
						Type:     "channel",
						UserName: "linked_channel",
					},
					ReplyToMessage: &tbapi.Message{
						MessageID: 999999,
						From:      &tbapi.User{ID: targetUserID, UserName: "spammer"},
						Text:      "this is spam text",
					},
				},
			}

			updChan := make(chan tbapi.Update, 1)
			updChan <- updMsg
			close(updChan)
			mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

			err := l.Do(ctx)
			require.EqualError(t, err, "telegram update chan closed")

			assert.Equal(t, linkedChannelID, l.linkedChannelID)

			if tt.wantBan {
				// find ban request among Request calls
				var foundBan bool
				for _, call := range mockAPI.RequestCalls() {
					if ban, ok := call.C.(tbapi.BanChatMemberConfig); ok {
						assert.Equal(t, groupChatID, ban.ChatID)
						assert.Equal(t, targetUserID, ban.UserID)
						foundBan = true
					}
				}
				assert.True(t, foundBan, "expected ban request for target user")
			}

			if tt.wantSpamTrain {
				require.Len(t, botMock.UpdateSpamCalls(), 1)
				assert.Equal(t, "this is spam text", botMock.UpdateSpamCalls()[0].Msg)
			} else {
				assert.Empty(t, botMock.UpdateSpamCalls())
			}

			if tt.wantOnMessage {
				require.Len(t, botMock.OnMessageCalls(), 1)
				assert.True(t, botMock.OnMessageCalls()[0].CheckOnly, "spam command should call OnMessage with checkOnly=true")
			} else {
				assert.Empty(t, botMock.OnMessageCalls())
			}

			if tt.wantWarnMsg {
				// find warning send among Send calls
				var foundWarn bool
				for _, call := range mockAPI.SendCalls() {
					mc := call.C.(tbapi.MessageConfig)
					if strings.Contains(mc.Text, "warning from") && strings.Contains(mc.Text, "You have been warned") {
						foundWarn = true
					}
				}
				assert.True(t, foundWarn, "expected warning message to be sent")
			}

			if tt.wantDeleteOrig {
				// verify the original replied-to message was deleted
				var foundDelete bool
				for _, call := range mockAPI.RequestCalls() {
					if del, ok := call.C.(tbapi.DeleteMessageConfig); ok {
						if del.MessageID == 999999 {
							foundDelete = true
						}
					}
				}
				assert.True(t, foundDelete, "expected original message to be deleted")
			}
		})
	}
}

func TestTelegramListener_LinkedChannelSkipsSpamCheck(t *testing.T) {
	const (
		groupChatID     = int64(-1001688024850)
		linkedChannelID = int64(-1001234567890)
		channelBotID    = int64(136817688)
	)

	t.Run("linked channel message skips spam check", func(t *testing.T) {
		mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{
					Chat:         tbapi.Chat{ID: groupChatID},
					LinkedChatID: linkedChannelID,
				}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "bot"}}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return nil, nil
			},
		}
		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			t.Fatalf("bot.OnMessage should not be called for linked channel message")
			return bot.Response{}
		}}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			Group:      fmt.Sprintf("%d", groupChatID),
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: groupChatID},
				Text: "a normal message from the linked channel",
				From: &tbapi.User{ID: channelBotID, UserName: "Channel_Bot"},
				SenderChat: &tbapi.Chat{
					ID:       linkedChannelID,
					Type:     "channel",
					UserName: "linked_channel",
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

	t.Run("non-linked channel message runs spam check", func(t *testing.T) {
		mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{
					Chat:         tbapi.Chat{ID: groupChatID},
					LinkedChatID: linkedChannelID,
				}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "bot"}}, nil
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return nil, nil
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
			Group:      fmt.Sprintf("%d", groupChatID),
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: groupChatID},
				Text: "suspicious message from unknown channel",
				From: &tbapi.User{ID: channelBotID, UserName: "Channel_Bot"},
				SenderChat: &tbapi.Chat{
					ID:       -1009999999999,
					Type:     "channel",
					UserName: "random_channel",
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
		assert.Equal(t, "suspicious message from unknown channel", botMock.OnMessageCalls()[0].Msg.Text)
	})

	t.Run("linked channel reply with non-command text skips spam check", func(t *testing.T) {

		mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
		mockAPI := &mocks.TbAPIMock{
			GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
				return tbapi.ChatFullInfo{
					Chat:         tbapi.Chat{ID: groupChatID},
					LinkedChatID: linkedChannelID,
				}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "bot"}}, nil
			},
			GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
				return nil, nil
			},
		}
		botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			t.Fatalf("bot.OnMessage should not be called for linked channel reply")
			return bot.Response{}
		}}

		locator, teardown := prepTestLocator(t)
		defer teardown()

		l := TelegramListener{
			SpamLogger: mockLogger,
			TbAPI:      mockAPI,
			Bot:        botMock,
			Group:      fmt.Sprintf("%d", groupChatID),
			Locator:    locator,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		updMsg := tbapi.Update{
			Message: &tbapi.Message{
				Chat: tbapi.Chat{ID: groupChatID},
				Text: "just a normal reply",
				From: &tbapi.User{ID: channelBotID, UserName: "Channel_Bot"},
				SenderChat: &tbapi.Chat{
					ID:       linkedChannelID,
					Type:     "channel",
					UserName: "linked_channel",
				},
				ReplyToMessage: &tbapi.Message{
					MessageID: 42,
					Text:      "original message from a user",
					From:      &tbapi.User{ID: 999, UserName: "some_user"},
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

func TestTelegramListener_LinkedChannelGetChatFailure(t *testing.T) {
	// when GetChat fails during linked channel resolution at startup, the bot should
	// start normally with linkedChannelID = 0 and treat all channels as non-linked
	const (
		groupChatID  = int64(-1001688024850)
		channelBotID = int64(136817688)
	)

	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {

			return tbapi.ChatFullInfo{}, fmt.Errorf("telegram api error: chat not found")
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "bot"}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
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
		Group:      fmt.Sprintf("%d", groupChatID),
		Locator:    locator,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	channelID := int64(-1001234567890)
	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat: tbapi.Chat{ID: groupChatID},
			Text: "message from a channel",
			From: &tbapi.User{ID: channelBotID, UserName: "Channel_Bot"},
			SenderChat: &tbapi.Chat{
				ID:       channelID,
				Type:     "channel",
				UserName: "some_channel",
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

	assert.Len(t, mockAPI.GetChatCalls(), 1)

	assert.Len(t, botMock.OnMessageCalls(), 1)
	assert.Equal(t, "message from a channel", botMock.OnMessageCalls()[0].Msg.Text)
}
