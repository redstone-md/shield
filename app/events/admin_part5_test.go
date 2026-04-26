package events

import (
	"context"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"testing"
)

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

		botMock := &mocks.BotMock{}
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
		err := adminHandler.MsgHandler(update)
		require.NoError(t, err)

		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, mockAPI.SendCalls())
		assert.Empty(t, botMock.UpdateSpamCalls())
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
		err := adminHandler.MsgHandler(update)
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
		err := adminHandler.MsgHandler(update)
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

		err := adm.MsgHandler(tbapi.Update{Message: msg})
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

		err := adm.MsgHandler(tbapi.Update{Message: msg})
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
		err := adminHandler.MsgHandler(update)
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
		err := adminHandler.MsgHandler(update)
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
		err := adminHandler.MsgHandler(update)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to remove user")

		assert.Len(t, mockAPI.SendCalls(), 1, "Should send detection results to admin")

		assert.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 2, "Should request to delete the message and ban user")
		assert.Len(t, botMock.UpdateSpamCalls(), 1, "Should update spam samples")
	})
}
