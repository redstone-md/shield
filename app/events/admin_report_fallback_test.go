package events

import (
	"context"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAdmin_MsgHandlerFallback(t *testing.T) {
	t.Run("locator fails, ForwardOrigin has user ID", func(t *testing.T) {
		var sentMessages []string
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				if msg, ok := c.(tbapi.MessageConfig); ok {
					sentMessages = append(sentMessages, msg.Text)
				}
				return tbapi.Message{Text: "test"}, nil
			},
		}

		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{CheckResults: []spamcheck.Response{
					{Name: "test", Spam: true, Details: "test details"},
				}}
			},
			UpdateSpamFunc: func(msg string) error { return nil },
		}

		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{}, false
			},
		}

		adminHandler := admin{
			tbAPI: mockAPI, bot: botMock, locator: locatorMock,
			primChatIDs: []int64{123}, adminChatID: 456, superUsers: SuperUsers{"superuser"},
		}

		msg := &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 456},
			From: &tbapi.User{UserName: "admin", ID: 123}, Text: "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type:       "user",
				SenderUser: &tbapi.User{ID: 555, UserName: "spammer"},
			},
		}

		err := adminHandler.MsgHandler(context.Background(), tbapi.Update{Message: msg})
		require.NoError(t, err)

		assert.Len(t, botMock.RemoveApprovedUserCalls(), 1)
		assert.Equal(t, int64(555), botMock.RemoveApprovedUserCalls()[0].ID)
		assert.Len(t, botMock.UpdateSpamCalls(), 1)
		assert.Equal(t, "spam message text", botMock.UpdateSpamCalls()[0].Msg)
		assert.Len(t, botMock.OnMessageCalls(), 1)
		assert.True(t, botMock.OnMessageCalls()[0].CheckOnly)

		assert.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 1, "should make ban request")

		require.Len(t, sentMessages, 2, "should send detection results and fallback warning")
		assert.Contains(t, sentMessages[0], `исходная диагностика для "spammer" (555)`)
		assert.Contains(t, sentMessages[1], "резервный режим locator")
		assert.Contains(t, sentMessages[1], "удалить вручную")
		assert.Contains(t, sentMessages[1], "spammer")
	})

	t.Run("locator fails, hidden user (fwdID=0)", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{}
		botMock := &mocks.BotMock{}
		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{}, false
			},
		}

		adminHandler := admin{
			tbAPI: mockAPI, bot: botMock, locator: locatorMock,
			primChatIDs: []int64{123}, adminChatID: 456,
		}

		msg := &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 456},
			From: &tbapi.User{UserName: "admin", ID: 123}, Text: "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type:           "hidden_user",
				SenderUserName: "hidden_name",
			},
		}

		err := adminHandler.MsgHandler(context.Background(), tbapi.Update{Message: msg})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")

		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, botMock.UpdateSpamCalls())
	})

	t.Run("locator fails, channel forward (fwdID=0)", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{}
		botMock := &mocks.BotMock{}
		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{}, false
			},
		}

		adminHandler := admin{
			tbAPI: mockAPI, bot: botMock, locator: locatorMock,
			primChatIDs: []int64{123}, adminChatID: 456,
		}

		msg := &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 456},
			From: &tbapi.User{UserName: "admin", ID: 123}, Text: "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type: "channel",
			},
		}

		err := adminHandler.MsgHandler(context.Background(), tbapi.Update{Message: msg})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("locator fails, ForwardOrigin has user ID, dry mode", func(t *testing.T) {
		var sentMessages []string
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				if msg, ok := c.(tbapi.MessageConfig); ok {
					sentMessages = append(sentMessages, msg.Text)
				}
				return tbapi.Message{Text: "test"}, nil
			},
		}

		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{CheckResults: []spamcheck.Response{
					{Name: "test", Spam: true, Details: "test details"},
				}}
			},
		}

		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{}, false
			},
		}

		adminHandler := admin{
			tbAPI: mockAPI, bot: botMock, locator: locatorMock,
			primChatIDs: []int64{123}, adminChatID: 456, dry: true,
		}

		msg := &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 456},
			From: &tbapi.User{UserName: "admin", ID: 123}, Text: "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type:       "user",
				SenderUser: &tbapi.User{ID: 555, UserName: "spammer"},
			},
		}

		err := adminHandler.MsgHandler(context.Background(), tbapi.Update{Message: msg})
		require.NoError(t, err)

		assert.Empty(t, mockAPI.RequestCalls(), "should not ban in dry mode")
		assert.Empty(t, botMock.UpdateSpamCalls(), "should not update spam in dry mode")

		require.Len(t, sentMessages, 2)
		assert.Contains(t, sentMessages[0], "исходная диагностика")
		assert.Contains(t, sentMessages[1], "резервный режим locator")
		assert.Contains(t, sentMessages[1], "dry mode")
	})

	t.Run("locator fails, ForwardOrigin has user ID, training mode", func(t *testing.T) {
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
				return bot.Response{CheckResults: []spamcheck.Response{
					{Name: "test", Spam: true, Details: "test details"},
				}}
			},
			UpdateSpamFunc: func(msg string) error { return nil },
		}

		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{}, false
			},
		}

		adminHandler := admin{
			tbAPI: mockAPI, bot: botMock, locator: locatorMock,
			primChatIDs: []int64{123}, adminChatID: 456, trainingMode: true,
		}

		msg := &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 456},
			From: &tbapi.User{UserName: "admin", ID: 123}, Text: "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type:       "user",
				SenderUser: &tbapi.User{ID: 555, UserName: "spammer"},
			},
		}

		err := adminHandler.MsgHandler(context.Background(), tbapi.Update{Message: msg})
		require.NoError(t, err)

		assert.Len(t, botMock.UpdateSpamCalls(), 1, "should update spam in training mode")
		assert.Len(t, botMock.RemoveApprovedUserCalls(), 1, "should remove approved user")
		assert.Len(t, botMock.OnMessageCalls(), 1, "should check message")
		assert.Empty(t, mockAPI.RequestCalls(), "training mode ban is a no-op, no request calls")
	})

	t.Run("locator fails, ForwardOrigin has user ID, super-user", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{}
		botMock := &mocks.BotMock{}
		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{}, false
			},
		}

		adminHandler := admin{
			tbAPI: mockAPI, bot: botMock, locator: locatorMock,
			primChatIDs: []int64{123}, adminChatID: 456, superUsers: SuperUsers{"superuser"},
		}

		msg := &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 456},
			From: &tbapi.User{UserName: "admin", ID: 123}, Text: "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type:       "user",
				SenderUser: &tbapi.User{ID: 555, UserName: "superuser"},
			},
		}

		err := adminHandler.MsgHandler(context.Background(), tbapi.Update{Message: msg})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "super-user")
		assert.Contains(t, err.Error(), "ignored")

		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, botMock.UpdateSpamCalls())
	})

	t.Run("locator fails, ForwardOrigin has user ID, super-user by ID only (no username)", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{}
		botMock := &mocks.BotMock{}
		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{}, false
			},
		}

		adminHandler := admin{
			tbAPI: mockAPI, bot: botMock, locator: locatorMock,
			primChatIDs: []int64{123}, adminChatID: 456, superUsers: SuperUsers{"555"},
		}

		msg := &tbapi.Message{
			MessageID: 789, Chat: tbapi.Chat{ID: 456},
			From: &tbapi.User{UserName: "admin", ID: 123}, Text: "spam message text",
			ForwardOrigin: &tbapi.MessageOrigin{
				Type:       "user",
				SenderUser: &tbapi.User{ID: 555, UserName: ""},
			},
		}

		err := adminHandler.MsgHandler(context.Background(), tbapi.Update{Message: msg})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "super-user")
		assert.Contains(t, err.Error(), "ignored")

		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, botMock.UpdateSpamCalls())
	})
}

func TestAdmin_DirectSpamReport_ImageOnly(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
	}

	botMock := &mocks.BotMock{
		RemoveApprovedUserFunc: func(id int64) error {
			return nil
		},
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{
				CheckResults: []spamcheck.Response{
					{Name: "image-spam", Spam: true, Details: "First message with image"},
				},
			}
		},
		UpdateSpamFunc: func(msg string) error {
			if msg == "" {
				return fmt.Errorf("can't update spam samples: message can't be empty")
			}
			return nil
		},
	}

	locatorMock := &mocks.LocatorMock{}

	adm := &admin{
		tbAPI:       mockAPI,
		bot:         botMock,
		primChatIDs: []int64{123},
		adminChatID: 456,
		locator:     locatorMock,
		superUsers:  SuperUsers{},
	}

	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 789,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "/spam",
			From:      &tbapi.User{UserName: "admin", ID: 111},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999,
				From:      &tbapi.User{ID: 666, UserName: "spammer"},
				Text:      "",
				Photo:     []tbapi.PhotoSize{{FileID: "photo123"}},
			},
		},
	}

	err := adm.directReport(context.Background(), update, true)
	require.NoError(t, err, "Should handle image-only spam without error")

	assert.Len(t, botMock.RemoveApprovedUserCalls(), 1, "Should remove user from approved list")
	assert.Equal(t, int64(666), botMock.RemoveApprovedUserCalls()[0].ID)

	assert.Len(t, mockAPI.SendCalls(), 1, "Should send detection results to admin")

	assert.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 2, "Should delete message and ban user")

	assert.Empty(t, botMock.UpdateSpamCalls(), "Should not update spam samples for empty messages")
}

func TestAdmin_DirectSpamReport_QuoteHandling(t *testing.T) {
	setup := func() (*mocks.TbAPIMock, *mocks.BotMock, *admin) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc:    func(c tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
		}
		botMock := &mocks.BotMock{
			RemoveApprovedUserFunc: func(id int64) error { return nil },
			OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
				return bot.Response{CheckResults: []spamcheck.Response{{Name: "test", Spam: true, Details: "spam"}}}
			},
			UpdateSpamFunc: func(msg string) error { return nil },
		}
		adm := &admin{
			tbAPI: mockAPI, bot: botMock, primChatIDs: []int64{123}, adminChatID: 456,
			locator: &mocks.LocatorMock{}, superUsers: SuperUsers{},
		}
		return mockAPI, botMock, adm
	}

	t.Run("message with quote text", func(t *testing.T) {
		_, botMock, adm := setup()
		update := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789, Chat: tbapi.Chat{ID: 123}, Text: "/spam",
				From: &tbapi.User{UserName: "admin", ID: 111},
				ReplyToMessage: &tbapi.Message{
					MessageID: 999, From: &tbapi.User{ID: 666, UserName: "spammer"},
					Text:  "Thank you",
					Quote: &tbapi.TextQuote{Text: "Buy cheap stuff at spam.com"},
				},
			},
		}
		err := adm.directReport(context.Background(), update, true)
		require.NoError(t, err)
		require.Len(t, botMock.UpdateSpamCalls(), 1)
		assert.Equal(t, "Thank you\nBuy cheap stuff at spam.com", botMock.UpdateSpamCalls()[0].Msg)
		require.Len(t, botMock.OnMessageCalls(), 1)
		assert.Equal(t, "Thank you\nBuy cheap stuff at spam.com", botMock.OnMessageCalls()[0].Msg.Text)
	})

	t.Run("message with empty quote text", func(t *testing.T) {
		_, botMock, adm := setup()
		update := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789, Chat: tbapi.Chat{ID: 123}, Text: "/spam",
				From: &tbapi.User{UserName: "admin", ID: 111},
				ReplyToMessage: &tbapi.Message{
					MessageID: 999, From: &tbapi.User{ID: 666, UserName: "spammer"},
					Text:  "some text",
					Quote: &tbapi.TextQuote{Text: ""},
				},
			},
		}
		err := adm.directReport(context.Background(), update, true)
		require.NoError(t, err)
		require.Len(t, botMock.UpdateSpamCalls(), 1)
		assert.Equal(t, "some text", botMock.UpdateSpamCalls()[0].Msg)
	})

	t.Run("message without quote", func(t *testing.T) {
		_, botMock, adm := setup()
		update := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789, Chat: tbapi.Chat{ID: 123}, Text: "/spam",
				From: &tbapi.User{UserName: "admin", ID: 111},
				ReplyToMessage: &tbapi.Message{
					MessageID: 999, From: &tbapi.User{ID: 666, UserName: "spammer"},
					Text: "plain spam text",
				},
			},
		}
		err := adm.directReport(context.Background(), update, true)
		require.NoError(t, err)
		require.Len(t, botMock.UpdateSpamCalls(), 1)
		assert.Equal(t, "plain spam text", botMock.UpdateSpamCalls()[0].Msg)
	})

	t.Run("empty text with quote present uses transform fallback plus quote", func(t *testing.T) {
		_, botMock, adm := setup()
		update := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789, Chat: tbapi.Chat{ID: 123}, Text: "/spam",
				From: &tbapi.User{UserName: "admin", ID: 111},
				ReplyToMessage: &tbapi.Message{
					MessageID: 999, From: &tbapi.User{ID: 666, UserName: "spammer"},
					Text:    "",
					Caption: "image caption",
					Photo:   []tbapi.PhotoSize{{FileID: "photo123"}},
					Quote:   &tbapi.TextQuote{Text: "quoted spam content"},
				},
			},
		}
		err := adm.directReport(context.Background(), update, true)
		require.NoError(t, err)
		require.Len(t, botMock.UpdateSpamCalls(), 1)
		assert.Equal(t, "image caption\nquoted spam content", botMock.UpdateSpamCalls()[0].Msg)
	})
}
