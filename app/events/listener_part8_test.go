package events

import (
	"context"
	"errors"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
	"os"
	"testing"
	"time"
)

func TestProcLeftChatMemberMessage(t *testing.T) {
	type deleteMessageArgs struct {
		ChatID int64
		MsgID  int
	}

	tests := []struct {
		name                       string
		update                     tbapi.Update
		expectedError              bool
		expectedDeleteMessageArgs  deleteMessageArgs
		expectedDeleteMessageCalls int
		returnErrorInRequest       bool
	}{
		{
			name: "new chat member kick by admin successfully",
			update: tbapi.Update{
				Message: &tbapi.Message{
					Chat:           tbapi.Chat{ID: 123},
					From:           &tbapi.User{UserName: "superuser1", ID: 77},
					LeftChatMember: &tbapi.User{ID: 88, UserName: "user1"},
					MessageID:      22,
				},
			},
			expectedError: false,
			expectedDeleteMessageArgs: deleteMessageArgs{
				ChatID: 123,
				MsgID:  20,
			},
			expectedDeleteMessageCalls: 1,
		},
		{
			name: "new chat member left self successfully",
			update: tbapi.Update{
				Message: &tbapi.Message{
					Chat:           tbapi.Chat{ID: 123},
					From:           &tbapi.User{UserName: "user1", ID: 88},
					LeftChatMember: &tbapi.User{ID: 88, UserName: "user1"},
					MessageID:      22,
				},
			},
			expectedError:              false,
			expectedDeleteMessageArgs:  deleteMessageArgs{},
			expectedDeleteMessageCalls: 0,
		},
		{
			name: "message from unauthorized chat",
			update: tbapi.Update{
				Message: &tbapi.Message{
					Chat:           tbapi.Chat{ID: 999},
					LeftChatMember: &tbapi.User{ID: 88, UserName: "user1"},
				},
			},
			expectedError:              false,
			expectedDeleteMessageArgs:  deleteMessageArgs{},
			expectedDeleteMessageCalls: 0,
		},
		{
			name: "no new message in the chat found",
			update: tbapi.Update{
				Message: &tbapi.Message{
					Chat:           tbapi.Chat{ID: 123},
					LeftChatMember: &tbapi.User{ID: 88, UserName: "user1"},
					From:           &tbapi.User{ID: 77, UserName: "superuser1"},
				},
			},
			expectedError:              false,
			expectedDeleteMessageArgs:  deleteMessageArgs{},
			expectedDeleteMessageCalls: 0,
		},
		{
			name: "failed to delete new chat member message",
			update: tbapi.Update{
				Message: &tbapi.Message{
					Chat:           tbapi.Chat{ID: 123},
					LeftChatMember: &tbapi.User{ID: 88, UserName: "user1"},
					From:           &tbapi.User{ID: 77, UserName: "superuser1"},
				},
			},
			expectedError: true,
			expectedDeleteMessageArgs: deleteMessageArgs{
				ChatID: 123,
				MsgID:  20,
			},
			expectedDeleteMessageCalls: 1,
			returnErrorInRequest:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockAPI := &mocks.TbAPIMock{
				GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
					return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
				},
				SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
					return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "user"}}, nil
				},
				RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
					if tt.returnErrorInRequest {
						return nil, errors.New("request error")
					}
					return &tbapi.APIResponse{Ok: true}, nil
				},
			}

			locator, teardown := prepTestLocator(t)
			defer teardown()

			l := &TelegramListener{
				Locator: locator,
				chatID:  123,
				TbAPI:   mockAPI,
			}

			if tt.expectedDeleteMessageArgs.ChatID != 0 && tt.expectedDeleteMessageArgs.MsgID != 0 {
				msg := fmt.Sprintf("new_%d_%d", tt.expectedDeleteMessageArgs.ChatID, tt.update.Message.LeftChatMember.ID)
				err := l.Locator.AddMessage(context.Background(), msg, tt.expectedDeleteMessageArgs.ChatID,
					tt.update.Message.LeftChatMember.ID, "", tt.expectedDeleteMessageArgs.MsgID)
				require.NoError(t, err)
			}

			err := l.procLeftChatMemberMessage(tt.update)
			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Len(t, mockAPI.RequestCalls(), tt.expectedDeleteMessageCalls)
			if tt.expectedDeleteMessageCalls == 1 {
				assert.Equal(t, tt.expectedDeleteMessageArgs.ChatID, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
				assert.Equal(t, tt.expectedDeleteMessageArgs.MsgID, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
			}
		})
	}
}

func prepTestLocator(t *testing.T) (loc *storage.Locator, teardown func()) {
	f, err := os.CreateTemp("", "locator")
	require.NoError(t, err)
	db, err := engine.NewSqlite(f.Name(), "gr1")
	require.NoError(t, err)

	loc, err = storage.NewLocator(context.Background(), 10*time.Minute, 100, db)
	require.NoError(t, err)
	return loc, func() {
		_ = os.Remove(f.Name())
	}
}

func TestTelegramListener_ForwardedGiveaway(t *testing.T) {
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
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}

	botMock := &mocks.BotMock{
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			t.Logf("on-message: %+v", msg)
			if msg.WithForward && msg.Text == "" {

				return bot.Response{
					Send:          true,
					Text:          "detected forwarded spam",
					BanInterval:   bot.PermanentBanDuration,
					User:          bot.User{ID: 456, Username: "spammer"},
					DeleteReplyTo: true,
				}
			}
			return bot.Response{}
		},
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        botMock,
		Group:      "gr",
		AdminGroup: "987654321",
		Locator:    locator,
		SuperUsers: SuperUsers{"super"},
		chatID:     123,
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			Chat:          tbapi.Chat{ID: 123},
			From:          &tbapi.User{ID: 456, UserName: "spammer"},
			Date:          int(time.Now().Unix()),
			Text:          "",
			ForwardOrigin: &tbapi.MessageOrigin{},
			MessageID:     789,
		},
	}

	err := l.procEvents(update)
	require.NoError(t, err)

	require.Len(t, botMock.OnMessageCalls(), 1)
	assert.True(t, botMock.OnMessageCalls()[0].Msg.WithForward)
	assert.Empty(t, botMock.OnMessageCalls()[0].Msg.Text)
	assert.Equal(t, int64(456), botMock.OnMessageCalls()[0].Msg.From.ID)

	require.Len(t, mockAPI.RequestCalls(), 1)
	assert.Equal(t, int64(456), mockAPI.RequestCalls()[0].C.(tbapi.BanChatMemberConfig).UserID)

	require.Len(t, mockLogger.SaveCalls(), 1)
}

func TestTelegramListener_DeleteJoinMessages(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
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
		TbAPI:              mockAPI,
		Bot:                b,
		SuperUsers:         SuperUsers{"admin"},
		Group:              "gr",
		Locator:            locator,
		DeleteJoinMessages: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat:           tbapi.Chat{ID: 123},
			From:           &tbapi.User{UserName: "admin", ID: 100},
			NewChatMembers: []tbapi.User{{UserName: "new_user", ID: 321}},
			MessageID:      42,
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	require.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 1)
	var found bool
	for _, call := range mockAPI.RequestCalls() {
		if deleteConfig, ok := call.C.(tbapi.DeleteMessageConfig); ok {
			if deleteConfig.MessageID == 42 && deleteConfig.ChatID == 123 {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "delete message request should be called for join message")
}

func TestTelegramListener_DeleteLeaveMessages(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
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
		DeleteLeaveMessages: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat:           tbapi.Chat{ID: 123},
			From:           &tbapi.User{UserName: "admin", ID: 100},
			LeftChatMember: &tbapi.User{UserName: "leaving_user", ID: 321},
			MessageID:      43,
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	require.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 1)
	var found bool
	for _, call := range mockAPI.RequestCalls() {
		if deleteConfig, ok := call.C.(tbapi.DeleteMessageConfig); ok {
			if deleteConfig.MessageID == 43 && deleteConfig.ChatID == 123 {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "delete message request should be called for leave message")
}

func TestTelegramListener_DeleteSystemMessage(t *testing.T) {
	t.Run("successful deletion", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				deleteConfig, ok := c.(tbapi.DeleteMessageConfig)
				require.True(t, ok)
				assert.Equal(t, 123, deleteConfig.MessageID)
				assert.Equal(t, int64(456), deleteConfig.ChatID)
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		l := TelegramListener{TbAPI: mockAPI}
		l.deleteSystemMessage(123, 456, "join")

		require.Len(t, mockAPI.RequestCalls(), 1)
	})

	t.Run("deletion failure", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return nil, assert.AnError
			},
		}

		l := TelegramListener{TbAPI: mockAPI}

		l.deleteSystemMessage(123, 456, "leave")

		require.Len(t, mockAPI.RequestCalls(), 1)
	})
}

func TestTelegramListener_NoDeleteWhenFlagsDisabled(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
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
		DeleteJoinMessages:  false,
		DeleteLeaveMessages: false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat:           tbapi.Chat{ID: 123},
			From:           &tbapi.User{UserName: "admin", ID: 100},
			NewChatMembers: []tbapi.User{{UserName: "new_user", ID: 321}},
			MessageID:      44,
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	for _, call := range mockAPI.RequestCalls() {
		if deleteConfig, ok := call.C.(tbapi.DeleteMessageConfig); ok {
			assert.NotEqual(t, 44, deleteConfig.MessageID, "should not delete join message when flag is false")
		}
	}
}
