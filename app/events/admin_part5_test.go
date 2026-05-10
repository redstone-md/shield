package events

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/slowpath"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

type slowPathCheckerSpy struct {
	check func(ctx context.Context, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error)
	calls []slowpath.SlowPathRequest
}

func (s *slowPathCheckerSpy) Check(ctx context.Context, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error) {
	s.calls = append(s.calls, req)
	if s.check != nil {
		return s.check(ctx, req)
	}
	return &slowpath.SlowPathResult{}, nil
}

func TestAdmin_MsgHandler(t *testing.T) {
	t.Run("non-forwarded message", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: "test"}, nil
			},
		}

		botMock := &mocks.BotMock{
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				assert.True(t, checkOnly)
				assert.Equal(t, "regular message", msg.Text)
				return bot.Response{CheckResults: []spamcheck.Response{
					{Name: "message length", Spam: false, Details: "ok"},
				}}
			},
		}
		locatorMock := &mocks.LocatorMock{}

		adminHandler := admin{
			tbAPI:       mockAPI,
			bot:         botMock,
			locator:     locatorMock,
			primChatID:  123,
			adminChatID: 456,
		}

		msg := &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{UserName: "admin", ID: 123},
			Text:      "regular message",
		}

		update := tbapi.Update{Message: msg}
		err := adminHandler.MsgHandler(context.Background(), update)
		require.NoError(t, err)

		assert.Empty(t, mockAPI.RequestCalls())
		require.Len(t, mockAPI.SendCalls(), 1)
		sent, ok := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
		require.True(t, ok)
		assert.Equal(t, int64(456), sent.ChatID)
		assert.Contains(t, sent.Text, "демо-проверка")
		assert.Contains(t, sent.Text, "сообщение пройдет")
		assert.Contains(t, sent.Text, "message length")
		assert.Empty(t, botMock.UpdateSpamCalls())
		assert.Empty(t, botMock.RemoveApprovedUserCalls())
	})

	t.Run("non-forwarded spam message is diagnostic only", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: "test"}, nil
			},
		}

		botMock := &mocks.BotMock{
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				assert.True(t, checkOnly)
				return bot.Response{
					Send:        true,
					BanInterval: bot.PermanentBanDuration,
					CheckResults: []spamcheck.Response{
						{Name: "stop-word", Spam: true, Details: "matched casino"},
					},
				}
			},
			UpdateSpamFunc: func(msg string) error {
				return nil
			},
			RemoveApprovedUserFunc: func(id int64) error {
				return nil
			},
		}

		adm := admin{tbAPI: mockAPI, bot: botMock, locator: &mocks.LocatorMock{}, primChatID: 123, adminChatID: 456}
		msg := &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{UserName: "admin", ID: 123},
			Text:      "casino spam",
		}

		err := adm.MsgHandler(context.Background(), tbapi.Update{Message: msg})
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		sent := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
		assert.Contains(t, sent.Text, "сообщение НЕ пройдет")
		assert.Contains(t, sent.Text, "stop-word")
		assert.Contains(t, sent.Text, "matched casino")
		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, botMock.UpdateSpamCalls())
		assert.Empty(t, botMock.RemoveApprovedUserCalls())
	})

	t.Run("forwarded message from super-user", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: "test"}, nil
			},
		}

		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error {
				return nil
			},
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{
					CheckResults: []spamcheck.Response{
						{Name: "test", Spam: true, Details: "test details"},
					},
				}
			},
		}

		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{
					UserID:   888,
					UserName: "superuser",
					MsgID:    999,
				}, true
			},
		}

		adminHandler := admin{
			tbAPI:       mockAPI,
			bot:         botMock,
			locator:     locatorMock,
			primChatID:  123,
			adminChatID: 456,
			superUsers:  SuperUsers{"superuser"},
		}

		msg := &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{UserName: "admin", ID: 123},
			Text:      "spam message",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type: "user",
				SenderUser: &tbapi.User{
					ID:       555,
					UserName: "user",
				},
			},
		}

		update := tbapi.Update{Message: msg}
		err := adminHandler.MsgHandler(context.Background(), update)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "forwarded message is about super-user")

	})

	t.Run("successful forwarded message processing", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: "test"}, nil
			},
		}

		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error {
				return nil
			},
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{
					CheckResults: []spamcheck.Response{
						{Name: "test", Spam: true, Details: "test details"},
					},
				}
			},
			UpdateSpamFunc: func(msg string) error {
				return nil
			},
		}

		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{
					UserID:   888,
					UserName: "regularuser",
					MsgID:    999,
				}, true
			},
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{}, true
			},
		}

		adminHandler := admin{
			tbAPI:       mockAPI,
			bot:         botMock,
			locator:     locatorMock,
			primChatID:  123,
			adminChatID: 456,
			superUsers:  SuperUsers{"superuser"},
		}

		msg := &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{UserName: "admin", ID: 123},
			Text:      "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type: "user",
				SenderUser: &tbapi.User{
					ID:       555,
					UserName: "user",
				},
			},
		}

		update := tbapi.Update{Message: msg}
		err := adminHandler.MsgHandler(context.Background(), update)
		require.NoError(t, err)

		assert.Len(t, mockAPI.SendCalls(), 1, "Should send detection results to admin")

		assert.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 2, "Should request to delete the message and ban user")
		assert.Len(t, botMock.UpdateSpamCalls(), 1, "Should update spam samples")
		assert.Len(t, botMock.RemoveApprovedUserCalls(), 1, "Should remove user from approved list")
		assert.Len(t, botMock.OnMessageCalls(), 1, "Should check message for spam")

		assert.Equal(t, int64(888), botMock.RemoveApprovedUserCalls()[0].ID, "Should remove correct user ID")
		assert.Equal(t, "spam message text", botMock.UpdateSpamCalls()[0].Msg, "Should update with correct text")
	})

	t.Run("channel message uses BanChatSenderChatConfig", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: "test"}, nil
			},
		}

		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{CheckResults: []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}}}
			},
			UpdateSpamFunc: func(msg string) error { return nil },
		}

		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{UserID: -1001261918100, UserName: "spam_channel", MsgID: 999}, true
			},
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{}, true
			},
		}

		adm := admin{
			tbAPI: mockAPI, bot: botMock, locator: locatorMock,
			primChatID: 123, adminChatID: 456, superUsers: SuperUsers{"superuser"},
		}

		msg := &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 456},
			From: &tbapi.User{UserName: "admin", ID: 111},
			Text: "spam from channel",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type:       "channel",
				SenderChat: &tbapi.Chat{ID: -1001261918100, UserName: "spam_channel"},
			},
		}

		err := adm.MsgHandler(context.Background(), tbapi.Update{Message: msg})
		require.NoError(t, err)

		// verify BanChatSenderChatConfig was used with the channel ID
		var foundChannelBan bool
		for _, call := range mockAPI.RequestCalls() {
			if banCfg, ok := call.C.(tbapi.BanChatSenderChatConfig); ok {
				foundChannelBan = true
				assert.Equal(t, int64(-1001261918100), banCfg.SenderChatID)
				assert.Equal(t, int64(123), banCfg.ChatID)
			}
		}
		assert.True(t, foundChannelBan, "expected BanChatSenderChatConfig for channel message")

		for _, call := range mockAPI.RequestCalls() {
			_, isMemberBan := call.C.(tbapi.BanChatMemberConfig)
			assert.False(t, isMemberBan, "should not use BanChatMemberConfig for channel message")
		}
	})

	t.Run("anonymous admin post skips ban in MsgHandler", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: "test"}, nil
			},
		}

		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{CheckResults: []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}}}
			},
			UpdateSpamFunc: func(msg string) error { return nil },
		}

		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{UserID: 123, UserName: "the_group", MsgID: 999}, true
			},
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{}, true
			},
		}

		adm := admin{
			tbAPI: mockAPI, bot: botMock, locator: locatorMock,
			primChatID: 123, adminChatID: 456, superUsers: SuperUsers{"superuser"},
		}

		msg := &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 456},
			From: &tbapi.User{UserName: "admin", ID: 111},
			Text: "admin message forwarded",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type:           "hidden_user",
				SenderUserName: "the_group",
			},
		}

		err := adm.MsgHandler(context.Background(), tbapi.Update{Message: msg})
		require.NoError(t, err)

		require.Len(t, locatorMock.MessageCalls(), 1)

		for _, call := range mockAPI.RequestCalls() {
			_, isChannelBan := call.C.(tbapi.BanChatSenderChatConfig)
			assert.False(t, isChannelBan, "should not ban channel when user ID matches group chat")
			_, isMemberBan := call.C.(tbapi.BanChatMemberConfig)
			assert.False(t, isMemberBan, "should not ban member when user ID matches group chat")
		}
	})

	t.Run("message not found in locator with hidden user", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{}
		botMock := &mocks.BotMock{}
		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{}, false
			},
		}

		adminHandler := admin{
			tbAPI:       mockAPI,
			bot:         botMock,
			locator:     locatorMock,
			primChatID:  123,
			adminChatID: 456,
		}

		msg := &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{UserName: "admin", ID: 123},
			Text:      "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type:           "hidden_user",
				SenderUserName: "hidden",
			},
		}

		update := tbapi.Update{Message: msg}
		err := adminHandler.MsgHandler(context.Background(), update)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("dry mode", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: "test"}, nil
			},
		}

		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error {
				return nil
			},
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{
					CheckResults: []spamcheck.Response{
						{Name: "test", Spam: true, Details: "test details"},
					},
				}
			},
		}

		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{
					UserID:   888,
					UserName: "regularuser",
					MsgID:    999,
				}, true
			},
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{}, true
			},
		}

		adminHandler := admin{
			tbAPI:       mockAPI,
			bot:         botMock,
			locator:     locatorMock,
			primChatID:  123,
			adminChatID: 456,
			superUsers:  SuperUsers{"superuser"},
			dry:         true,
		}

		msg := &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{UserName: "admin", ID: 123},
			Text:      "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type: "user",
				SenderUser: &tbapi.User{
					ID:       555,
					UserName: "user",
				},
			},
		}

		update := tbapi.Update{Message: msg}
		err := adminHandler.MsgHandler(context.Background(), update)
		require.NoError(t, err)

		assert.Len(t, mockAPI.SendCalls(), 1, "Should send detection results to admin")
		assert.Empty(t, mockAPI.RequestCalls(), "Should not make request calls in dry mode")
		assert.Empty(t, botMock.UpdateSpamCalls(), "Should not update spam in dry mode")
	})

	t.Run("error removing approved user", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				return tbapi.Message{Text: "test"}, nil
			},
		}

		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error {
				return fmt.Errorf("failed to remove user")
			},
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{
					CheckResults: []spamcheck.Response{
						{Name: "test", Spam: true, Details: "test details"},
					},
				}
			},
			UpdateSpamFunc: func(msg string) error {
				return nil
			},
		}

		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{
					UserID:   888,
					UserName: "regularuser",
					MsgID:    999,
				}, true
			},
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{}, true
			},
		}

		adminHandler := admin{
			tbAPI:       mockAPI,
			bot:         botMock,
			locator:     locatorMock,
			primChatID:  123,
			adminChatID: 456,
			superUsers:  SuperUsers{"superuser"},
		}

		msg := &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{UserName: "admin", ID: 123},
			Text:      "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type: "user",
				SenderUser: &tbapi.User{
					ID:       555,
					UserName: "user",
				},
			},
		}

		update := tbapi.Update{Message: msg}
		err := adminHandler.MsgHandler(context.Background(), update)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove user")

		assert.Len(t, mockAPI.SendCalls(), 1, "Should send detection results to admin")

		assert.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 2, "Should request to delete the message and ban user")
		assert.Len(t, botMock.UpdateSpamCalls(), 1, "Should update spam samples")
	})
}

func TestTelegramListener_AdminChatPlainMessageDemoCheck(t *testing.T) {
	callOrder := make([]string, 0, 2)
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: config.ChatID}}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return []tbapi.ChatMember{{User: &tbapi.User{UserName: "admin", ID: 1}}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			callOrder = append(callOrder, "send")
			return tbapi.Message{MessageID: 100}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			t.Fatalf("admin demo check must not call Request: %T", c)
			return nil, nil
		},
	}

	botMock := &mocks.BotMock{
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			callOrder = append(callOrder, "detect")
			assert.True(t, checkOnly)
			assert.Equal(t, "buy casino", msg.Text)
			return bot.Response{Send: true, BanInterval: bot.PermanentBanDuration, CheckResults: []spamcheck.Response{
				{Name: "stop-word", Spam: true, Details: "casino"},
			}}
		},
	}
	policySpy := &policyEngineSpy{decide: func(ctx context.Context, req PolicyRequest) (PolicyOutcome, error) {
		return PolicyOutcome{Decision: moderation.PolicyDecision{Action: moderation.ActionBan}}, nil
	}}
	actionSpy := &actionExecutorSpy{}

	l := TelegramListener{
		TbAPI:          mockAPI,
		Bot:            botMock,
		SuperUsers:     SuperUsers{"admin"},
		Group:          "123",
		AdminGroup:     "456",
		Locator:        &locatorContextSpy{},
		PolicyEngine:   policySpy,
		ActionExecutor: actionSpy,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updates := make(chan tbapi.Update, 1)
	updates <- tbapi.Update{Message: &tbapi.Message{
		MessageID: 10,
		Chat:      tbapi.Chat{ID: 456},
		From:      &tbapi.User{UserName: "admin", ID: 1},
		Text:      "buy casino",
		Date:      int(time.Now().Unix()),
	}}
	close(updates)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updates }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	assert.Equal(t, []string{"detect", "send"}, callOrder)
	assert.Empty(t, policySpy.calls)
	assert.Empty(t, actionSpy.banCalls)
	assert.Empty(t, actionSpy.warnCalls)
	assert.Empty(t, actionSpy.deleteMessageCalls)
	assert.Empty(t, botMock.UpdateSpamCalls())

	require.Len(t, mockAPI.SendCalls(), 1)
	sent := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
	assert.Equal(t, int64(456), sent.ChatID)
	assert.True(t, strings.Contains(sent.Text, "сообщение НЕ пройдет"), sent.Text)
}

func TestTelegramListener_AdminChatPlainMessageDemoCheckIgnoresForwardDisable(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{MessageID: 100}, nil
		},
	}
	botMock := &mocks.BotMock{
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			assert.True(t, checkOnly)
			assert.Equal(t, "buy casino", msg.Text)
			return bot.Response{Send: true, BanInterval: bot.PermanentBanDuration, CheckResults: []spamcheck.Response{
				{Name: "stop-word", Spam: true, Details: "casino"},
			}}
		},
	}

	l := TelegramListener{
		TbAPI:                   mockAPI,
		Bot:                     botMock,
		SuperUsers:              SuperUsers{"admin"},
		DisableAdminSpamForward: true,
		adminChatID:             456,
		adminHandler:            &admin{tbAPI: mockAPI, bot: botMock, adminChatID: 456},
	}

	err := l.handleUpdate(context.Background(), tbapi.Update{Message: &tbapi.Message{
		MessageID: 10,
		Chat:      tbapi.Chat{ID: 456},
		From:      &tbapi.User{UserName: "admin", ID: 1},
		Text:      "buy casino",
	}})
	require.NoError(t, err)

	require.Len(t, mockAPI.SendCalls(), 1)
	sent := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
	assert.Equal(t, int64(456), sent.ChatID)
	assert.Contains(t, sent.Text, "демо-проверка")
	assert.Contains(t, sent.Text, "сообщение НЕ пройдет")
}

func TestTelegramListener_AdminChatDemoCheckUsesSlowPathForSticker(t *testing.T) {
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg thumbnail"))
	}))
	defer imgSrv.Close()

	slowSpy := &slowPathCheckerSpy{check: func(ctx context.Context, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error) {
		assert.Equal(t, slowpath.EscalationImageContent, req.Reason)
		assert.Equal(t, "image/jpeg", req.ImageMIME)
		assert.NotEmpty(t, req.ImageData)
		return &slowpath.SlowPathResult{Spam: true, Confidence: 91, Reason: "vision sticker spam", Providers: []string{"vision"}}, nil
	}}
	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			assert.Equal(t, "thumb-file", fileID)
			return imgSrv.URL, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{MessageID: 100}, nil
		},
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		assert.True(t, checkOnly)
		assert.True(t, msg.WithSticker)
		return bot.Response{CheckResults: []spamcheck.Response{{Name: "message length", Spam: false, Details: "too short"}}}
	}}

	l := TelegramListener{
		TbAPI:           mockAPI,
		Bot:             botMock,
		SuperUsers:      SuperUsers{"admin"},
		SlowPathEnabled: true,
		SlowPathEngine:  slowSpy,
		TenantID:        "tg-spam",
		adminChatID:     456,
	}
	l.adminHandler = &admin{tbAPI: mockAPI, bot: botMock, adminChatID: 456, mediaSlowPath: l.mediaSlowPathConfig()}

	err := l.handleUpdate(context.Background(), tbapi.Update{Message: &tbapi.Message{
		MessageID: 10,
		Chat:      tbapi.Chat{ID: 456},
		From:      &tbapi.User{UserName: "admin", ID: 1},
		Sticker: &tbapi.Sticker{
			FileID:    "sticker-file",
			Thumbnail: &tbapi.PhotoSize{FileID: "thumb-file"},
		},
	}})
	require.NoError(t, err)

	require.Len(t, slowSpy.calls, 1)
	require.Len(t, mockAPI.SendCalls(), 1)
	sent := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
	assert.Contains(t, sent.Text, "сообщение НЕ пройдет")
	assert.Contains(t, sent.Text, "vision: spam")
	assert.Contains(t, sent.Text, "vision sticker spam")
	assert.Contains(t, sent.Text, "confidence: 91%")
}

func TestTelegramListener_AdminChatDemoCheckUsesSlowPathForGIF(t *testing.T) {
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg frame"))
	}))
	defer imgSrv.Close()

	slowSpy := &slowPathCheckerSpy{check: func(ctx context.Context, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error) {
		assert.Equal(t, slowpath.EscalationImageContent, req.Reason)
		assert.Equal(t, "image/jpeg", req.ImageMIME)
		return &slowpath.SlowPathResult{Spam: true, Confidence: 88, Reason: "gif spam", Providers: []string{"vision"}}, nil
	}}
	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			assert.Equal(t, "gif-thumb", fileID)
			return imgSrv.URL, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{MessageID: 101}, nil },
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		assert.True(t, checkOnly)
		assert.NotNil(t, msg.Animation)
		return bot.Response{}
	}}

	l := TelegramListener{TbAPI: mockAPI, Bot: botMock, SuperUsers: SuperUsers{"admin"}, SlowPathEnabled: true,
		SlowPathEngine: slowSpy, TenantID: "tg-spam", adminChatID: 456}
	l.adminHandler = &admin{tbAPI: mockAPI, bot: botMock, adminChatID: 456, mediaSlowPath: l.mediaSlowPathConfig()}

	err := l.handleUpdate(context.Background(), tbapi.Update{Message: &tbapi.Message{
		MessageID: 11,
		Chat:      tbapi.Chat{ID: 456},
		From:      &tbapi.User{UserName: "admin", ID: 1},
		Animation: &tbapi.Animation{FileID: "gif-file", MimeType: "image/gif", Thumbnail: &tbapi.PhotoSize{FileID: "gif-thumb"}},
	}})
	require.NoError(t, err)
	require.Len(t, slowSpy.calls, 1)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "gif spam")
}

func TestTelegramListener_AdminChatDemoCheckUsesOriginalAnimationWithoutThumbnail(t *testing.T) {
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg frame"))
	}))
	defer imgSrv.Close()

	slowSpy := &slowPathCheckerSpy{check: func(ctx context.Context, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error) {
		assert.Equal(t, slowpath.EscalationImageContent, req.Reason)
		assert.Equal(t, "image/jpeg", req.ImageMIME)
		return &slowpath.SlowPathResult{Spam: false, Confidence: 12, Reason: "animation frame ok", Providers: []string{"vision"}}, nil
	}}
	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			assert.Equal(t, "gif-file", fileID)
			return imgSrv.URL, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{MessageID: 101}, nil },
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		assert.True(t, checkOnly)
		assert.NotNil(t, msg.Animation)
		return bot.Response{}
	}}

	l := TelegramListener{TbAPI: mockAPI, Bot: botMock, SuperUsers: SuperUsers{"admin"}, SlowPathEnabled: true,
		SlowPathEngine: slowSpy, TenantID: "tg-spam", adminChatID: 456}
	l.adminHandler = &admin{tbAPI: mockAPI, bot: botMock, adminChatID: 456, mediaSlowPath: l.mediaSlowPathConfig()}

	err := l.handleUpdate(context.Background(), tbapi.Update{Message: &tbapi.Message{
		MessageID: 11,
		Chat:      tbapi.Chat{ID: 456},
		From:      &tbapi.User{UserName: "admin", ID: 1},
		Animation: &tbapi.Animation{FileID: "gif-file", MimeType: "video/mp4"},
	}})

	require.NoError(t, err)
	require.Len(t, slowSpy.calls, 1)
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "сообщение пройдет")
}

func TestTelegramListener_AdminChatDemoCheckShowsSlowPathHamSummary(t *testing.T) {
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg frame"))
	}))
	defer imgSrv.Close()

	slowSpy := &slowPathCheckerSpy{check: func(ctx context.Context, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error) {
		assert.Equal(t, slowpath.EscalationImageContent, req.Reason)
		return &slowpath.SlowPathResult{
			Spam:       false,
			Confidence: 15,
			Reason:     "Упоминание другого пользователя без рекламного или оскорбительного содержания не считается спамом",
			Providers:  []string{"openai"},
		}, nil
	}}
	mockAPI := &mocks.TbAPIMock{
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			assert.Equal(t, "image-file", fileID)
			return imgSrv.URL, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{MessageID: 102}, nil },
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		assert.True(t, checkOnly)
		assert.NotNil(t, msg.Image)
		return bot.Response{}
	}}

	l := TelegramListener{TbAPI: mockAPI, Bot: botMock, SuperUsers: SuperUsers{"admin"}, SlowPathEnabled: true,
		SlowPathEngine: slowSpy, TenantID: "tg-spam", adminChatID: 456}
	l.adminHandler = &admin{tbAPI: mockAPI, bot: botMock, adminChatID: 456, mediaSlowPath: l.mediaSlowPathConfig()}

	err := l.handleUpdate(context.Background(), tbapi.Update{Message: &tbapi.Message{
		MessageID: 12,
		Chat:      tbapi.Chat{ID: 456},
		From:      &tbapi.User{UserName: "admin", ID: 1},
		Photo:     []tbapi.PhotoSize{{FileID: "image-file", Width: 256, Height: 256}},
	}})
	require.NoError(t, err)
	require.Len(t, slowSpy.calls, 1)
	require.Len(t, mockAPI.SendCalls(), 1)
	sent := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text
	assert.Contains(t, sent, "сообщение пройдет")
	assert.Contains(t, sent, "openai: ham")
	assert.Contains(t, sent, "Упоминание другого пользователя")
	assert.Contains(t, sent, "confidence: 15%")
}

func TestTelegramListener_AdminChatDemoCheckUsesSlowPathForCustomEmoji(t *testing.T) {
	webpData, decodeErr := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	require.NoError(t, decodeErr)
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(webpData)
	}))
	defer imgSrv.Close()

	slowSpy := &slowPathCheckerSpy{check: func(ctx context.Context, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error) {
		assert.Equal(t, "image/jpeg", req.ImageMIME)
		return &slowpath.SlowPathResult{Spam: true, Confidence: 90, Reason: "emoji spam", Providers: []string{"vision"}}, nil
	}}
	mockAPI := &mocks.TbAPIMock{
		GetCustomEmojiStickersFunc: func(config tbapi.GetCustomEmojiStickersConfig) ([]tbapi.Sticker, error) {
			require.Equal(t, []string{"emoji-1"}, config.CustomEmojiIDs)
			return []tbapi.Sticker{{FileID: "emoji-file", IsAnimated: true, Thumbnail: &tbapi.PhotoSize{FileID: "emoji-thumb"}}}, nil
		},
		GetFileDirectURLFunc: func(fileID string) (string, error) {
			assert.Equal(t, "emoji-thumb", fileID)
			return imgSrv.URL, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{MessageID: 102}, nil },
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		assert.True(t, checkOnly)
		assert.Equal(t, "emoji-1", msg.CustomEmojiID)
		return bot.Response{}
	}}

	l := TelegramListener{TbAPI: mockAPI, Bot: botMock, SuperUsers: SuperUsers{"admin"}, SlowPathEnabled: true,
		SlowPathEngine: slowSpy, TenantID: "tg-spam", adminChatID: 456}
	l.adminHandler = &admin{tbAPI: mockAPI, bot: botMock, adminChatID: 456, mediaSlowPath: l.mediaSlowPathConfig()}

	err := l.handleUpdate(context.Background(), tbapi.Update{Message: &tbapi.Message{
		MessageID: 12,
		Chat:      tbapi.Chat{ID: 456},
		From:      &tbapi.User{UserName: "admin", ID: 1},
		Text:      "🔥",
		Entities:  []tbapi.MessageEntity{{Type: "custom_emoji", Offset: 0, Length: 2, CustomEmojiID: "emoji-1"}},
	}})
	require.NoError(t, err)
	require.Len(t, slowSpy.calls, 1)
	require.Len(t, mockAPI.GetCustomEmojiStickersCalls(), 1)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "emoji spam")
}

func TestApplyMediaSlowPathRetriesRetryableErrors(t *testing.T) {
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg frame"))
	}))
	defer imgSrv.Close()

	attempts := 0
	slowSpy := &slowPathCheckerSpy{check: func(ctx context.Context, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("openai vision: 502 Bad Gateway retryable")
		}
		return &slowpath.SlowPathResult{Spam: true, Confidence: 91, Reason: "retried spam", Providers: []string{"vision"}}, nil
	}}
	mockAPI := &mocks.TbAPIMock{GetFileDirectURLFunc: func(fileID string) (string, error) { return imgSrv.URL, nil }}
	sleeps := []time.Duration{}
	cfg := mediaSlowPathConfig{enabled: true, api: mockAPI, engine: slowSpy, tenantID: "tg-spam", sleep: func(d time.Duration) {
		sleeps = append(sleeps, d)
	}}

	resp := applyMediaSlowPath(context.Background(), cfg, moderation.IncomingEvent{EventID: "e1"},
		&bot.Message{ID: 42, From: bot.User{ID: 1}, Image: &bot.Image{FileID: "image-file"}}, bot.Response{})

	assert.True(t, resp.Send)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, []time.Duration{3 * time.Second}, sleeps)
	require.Len(t, resp.CheckResults, 1)
	assert.Equal(t, "retried spam, confidence: 91%", resp.CheckResults[0].Details)
}

func TestApplyMediaSlowPathReportsSlowPathError(t *testing.T) {
	imgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg frame"))
	}))
	defer imgSrv.Close()

	slowSpy := &slowPathCheckerSpy{check: func(ctx context.Context, req slowpath.SlowPathRequest) (*slowpath.SlowPathResult, error) {
		return nil, errors.New("openai vision: error, status code: 400")
	}}
	mockAPI := &mocks.TbAPIMock{GetFileDirectURLFunc: func(fileID string) (string, error) { return imgSrv.URL, nil }}
	cfg := mediaSlowPathConfig{enabled: true, api: mockAPI, engine: slowSpy, tenantID: "tg-spam"}

	resp := applyMediaSlowPath(context.Background(), cfg, moderation.IncomingEvent{EventID: "e1"},
		&bot.Message{ID: 42, From: bot.User{ID: 1}, Image: &bot.Image{FileID: "image-file"}}, bot.Response{})

	assert.False(t, resp.Send)
	require.Len(t, resp.CheckResults, 1)
	assert.Equal(t, "slowpath", resp.CheckResults[0].Name)
	assert.False(t, resp.CheckResults[0].Spam)
	assert.Contains(t, resp.CheckResults[0].Details, "openai vision: error, status code: 400")
}
