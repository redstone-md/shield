package events

import (
	"context"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAdmin_InlineCallbacks(t *testing.T) {
	setupCallback := func(trainingMode bool, softBan bool) (*mocks.TbAPIMock, *mocks.BotMock, *admin, *tbapi.CallbackQuery) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {

				switch v := c.(type) {
				case tbapi.MessageConfig:
					return tbapi.Message{Text: v.Text}, nil
				case tbapi.EditMessageTextConfig:
					return tbapi.Message{Text: v.Text}, nil
				default:
					return tbapi.Message{}, nil
				}
			},
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
		}

		botMock := &mocks.BotMock{
			UpdateSpamFunc: func(msg string) error {
				return nil
			},
		}

		locatorMock := &mocks.LocatorMock{
			MessageFunc: func(ctx context.Context, msg string) (storage.MsgMeta, bool) {
				return storage.MsgMeta{}, true
			},
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{}, true
			},
			UserNameByIDFunc: func(ctx context.Context, userID int64) string {
				return "testuser"
			},
		}

		adm := &admin{
			tbAPI:        mockAPI,
			bot:          botMock,
			primChatIDs:  []int64{123},
			adminChatID:  456,
			locator:      locatorMock,
			trainingMode: trainingMode,
			softBan:      softBan,
		}

		query := &tbapi.CallbackQuery{
			ID:   "test-callback-id",
			Data: "+12345:999",
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 456},
				Text:      "**permanently banned [user_name_with_underscore](tg://user?id=12345)**\n\nSpam message text",
				From:      &tbapi.User{UserName: "bot"},
			},
			From: &tbapi.User{
				UserName: "admin",
				ID:       111,
			},
		}

		return mockAPI, botMock, adm, query
	}

	t.Run("callbackBanConfirmed", func(t *testing.T) {
		mockAPI, botMock, adm, query := setupCallback(false, false)

		err := adm.callbackBanConfirmed(context.Background(), query)
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		editMsg := mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig)
		assert.Equal(t, query.Message.Chat.ID, editMsg.ChatID)
		assert.Equal(t, query.Message.MessageID, editMsg.MessageID)
		assert.Contains(t, editMsg.Text, `бан подтвержден администратором <a href="tg://user?id=111">admin</a>`)
		assert.Empty(t, editMsg.ReplyMarkup.InlineKeyboard)

		require.Len(t, botMock.UpdateSpamCalls(), 1)
		assert.Equal(t, "Spam message text", botMock.UpdateSpamCalls()[0].Msg)
	})

	t.Run("callbackBanConfirmed_TrainingMode", func(t *testing.T) {
		mockAPI, _, adm, query := setupCallback(true, false)

		err := adm.callbackBanConfirmed(context.Background(), query)
		require.NoError(t, err)

		require.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 1)

		lastCall := mockAPI.RequestCalls()[len(mockAPI.RequestCalls())-1]
		assert.Equal(t, 999, lastCall.C.(tbapi.DeleteMessageConfig).MessageID)
		assert.Equal(t, int64(123), lastCall.C.(tbapi.DeleteMessageConfig).ChatID)
	})

	t.Run("callbackBanConfirmed_SoftBan", func(t *testing.T) {
		mockAPI, botMock, adm, query := setupCallback(false, true)

		err := adm.callbackBanConfirmed(context.Background(), query)
		require.NoError(t, err)

		require.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 1)

		// find the BanChatMemberConfig call
		var foundBanCall bool
		for _, call := range mockAPI.RequestCalls() {
			banCall, ok := call.C.(tbapi.BanChatMemberConfig)
			if !ok {
				continue
			}
			foundBanCall = true
			assert.Equal(t, int64(12345), banCall.UserID)
			assert.Equal(t, int64(123), banCall.ChatID)
			break
		}

		assert.True(t, foundBanCall, "Expected a BanChatMemberConfig call")

		require.Len(t, botMock.UpdateSpamCalls(), 1)
		assert.Equal(t, "Spam message text", botMock.UpdateSpamCalls()[0].Msg)
	})

	t.Run("callbackUnbanConfirmed_channel", func(t *testing.T) {
		mockAPI, _, adm, _ := setupCallback(false, false)
		botMock := &mocks.BotMock{
			UpdateHamFunc:       func(msg string) error { return nil },
			AddApprovedUserFunc: func(id int64, name string) error { return nil },
		}
		adm.bot = botMock

		query := &tbapi.CallbackQuery{
			ID:   "test-callback-id",
			Data: "-100999888:999",
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 456},
				Text:      "**permanently banned [spamchannel](https://t.me/spamchannel)**\n\nSpam from channel",
				From:      &tbapi.User{UserName: "bot"},
			},
			From: &tbapi.User{UserName: "admin", ID: 111},
		}

		err := adm.callbackUnbanConfirmed(context.Background(), query)
		require.NoError(t, err)

		// verify UnbanChatSenderChatConfig was used (not UnbanChatMemberConfig)
		var foundChannelUnban bool
		for _, call := range mockAPI.RequestCalls() {
			if unbanCall, ok := call.C.(tbapi.UnbanChatSenderChatConfig); ok {
				foundChannelUnban = true
				assert.Equal(t, int64(-100999888), unbanCall.SenderChatID)
				assert.Equal(t, int64(123), unbanCall.ChatID)
				break
			}
		}
		assert.True(t, foundChannelUnban, "expected UnbanChatSenderChatConfig for channel ID")

		require.Len(t, botMock.AddApprovedUserCalls(), 1)
		assert.Equal(t, int64(-100999888), botMock.AddApprovedUserCalls()[0].ID)
		assert.Equal(t, "spamchannel", botMock.AddApprovedUserCalls()[0].Name)
	})

	t.Run("callbackUnbanConfirmed_channel_plain_title", func(t *testing.T) {
		mockAPI, _, adm, _ := setupCallback(false, false)
		botMock := &mocks.BotMock{
			UpdateHamFunc:       func(msg string) error { return nil },
			AddApprovedUserFunc: func(id int64, name string) error { return nil },
		}
		adm.bot = botMock

		query := &tbapi.CallbackQuery{
			ID:   "test-callback-id",
			Data: "-100999888:999",
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 456},
				Text:      "**permanently banned Spam News Channel (-100999888)**\n\nSpam from channel",
				From:      &tbapi.User{UserName: "bot"},
			},
			From: &tbapi.User{UserName: "admin", ID: 111},
		}

		err := adm.callbackUnbanConfirmed(context.Background(), query)
		require.NoError(t, err)

		// verify UnbanChatSenderChatConfig was used
		var foundChannelUnban bool
		for _, call := range mockAPI.RequestCalls() {
			if unbanCall, ok := call.C.(tbapi.UnbanChatSenderChatConfig); ok {
				foundChannelUnban = true
				assert.Equal(t, int64(-100999888), unbanCall.SenderChatID)
				break
			}
		}
		assert.True(t, foundChannelUnban, "expected UnbanChatSenderChatConfig for channel ID")

		require.Len(t, botMock.AddApprovedUserCalls(), 1)
		assert.Equal(t, int64(-100999888), botMock.AddApprovedUserCalls()[0].ID)
		assert.Equal(t, "Spam News Channel", botMock.AddApprovedUserCalls()[0].Name)
	})

	t.Run("callbackBanConfirmed_SoftBan_channel", func(t *testing.T) {
		mockAPI, botMock, adm, _ := setupCallback(false, true)

		query := &tbapi.CallbackQuery{
			ID:   "test-callback-id",
			Data: "+-100999888:999",
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 456},
				Text:      "**permanently banned [spamchannel](https://t.me/spamchannel)**\n\nSpam from channel",
				From:      &tbapi.User{UserName: "bot"},
			},
			From: &tbapi.User{UserName: "admin", ID: 111},
		}

		err := adm.callbackBanConfirmed(context.Background(), query)
		require.NoError(t, err)

		// in soft ban mode with channel, should use BanChatSenderChatConfig
		var foundChannelBan bool
		for _, call := range mockAPI.RequestCalls() {
			if banCall, ok := call.C.(tbapi.BanChatSenderChatConfig); ok {
				foundChannelBan = true
				assert.Equal(t, int64(-100999888), banCall.SenderChatID)
				assert.Equal(t, int64(123), banCall.ChatID)
				break
			}
		}
		assert.True(t, foundChannelBan, "expected BanChatSenderChatConfig for channel ID")

		_ = botMock
	})
}

func TestAdmin_CallbackShowInfo_HTMLMode(t *testing.T) {

	t.Run("Preserve underscores for usernames, single HTML attempt", func(t *testing.T) {
		var sendAttempts int
		var usedParseMode string
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				sendAttempts++
				if msg, ok := c.(tbapi.EditMessageTextConfig); ok {
					usedParseMode = msg.ParseMode
					assert.Contains(t, msg.Text, "user_name_with_underscore",
						"underscore in username should pass through unescaped")
					assert.Contains(t, msg.Text, "<b>spam detection results</b>")
				}
				return tbapi.Message{}, nil
			},
		}

		adm := &admin{
			tbAPI:       mockAPI,
			adminChatID: 456,
		}

		query := &tbapi.CallbackQuery{
			ID:   "test-callback-id",
			Data: "!12345:999",
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 456},
				Text:      "permanently banned user_name_with_underscore\n\nSpam message text",
				From:      &tbapi.User{UserName: "bot"},
			},
			From: &tbapi.User{
				UserName: "admin",
				ID:       111,
			},
		}

		adm.locator = &mocks.LocatorMock{
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{
					Checks: []spamcheck.Response{
						{Name: "test", Spam: true, Details: "test details"},
					},
				}, true
			},
		}

		err := adm.callbackShowInfo(context.Background(), query)
		require.NoError(t, err)

		assert.Equal(t, 1, sendAttempts, "Should succeed on first HTML attempt")
		assert.Equal(t, tbapi.ModeHTML, usedParseMode, "Should use HTML parse mode")
	})

	t.Run("info button preserves text for normal username", func(t *testing.T) {
		sendAttempts := 0
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				sendAttempts++
				if editMsg, ok := c.(tbapi.EditMessageTextConfig); ok {
					assert.Contains(t, editMsg.Text, "permanently banned")
					assert.Contains(t, editMsg.Text, "spam detection results")

					assert.Equal(t, tbapi.ModeHTML, editMsg.ParseMode)
				}
				return tbapi.Message{}, nil
			},
		}

		adm := admin{
			tbAPI:       mockAPI,
			adminChatID: 123,
		}

		query := &tbapi.CallbackQuery{
			ID:   "test",
			Data: "!6236647121:123",
			Message: &tbapi.Message{
				MessageID: 123,
				Chat:      tbapi.Chat{ID: 456},
				Text:      "permanently banned Евгения\n\nНужны 2-3 человека совершеннолетних, занятость из дома ! От 8К в день Пиши в лс",
				From:      &tbapi.User{UserName: "bot"},
			},
			From: &tbapi.User{UserName: "admin", ID: 111},
		}

		adm.locator = &mocks.LocatorMock{
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{
					Checks: []spamcheck.Response{
						{Name: "stopword", Spam: true, Details: "в лс"},
					},
				}, true
			},
		}

		err := adm.callbackShowInfo(context.Background(), query)
		require.NoError(t, err)
		assert.Equal(t, 1, sendAttempts, "Should succeed on first HTML attempt")
	})

	t.Run("info button keeps username with underscore raw", func(t *testing.T) {
		sendAttempts := 0
		var usedParseMode string
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
				sendAttempts++
				if editMsg, ok := c.(tbapi.EditMessageTextConfig); ok {
					usedParseMode = editMsg.ParseMode

					assert.Contains(t, editMsg.Text, "surkova_vlada", "Underscore should stay raw in HTML mode")

					return tbapi.Message{}, nil
				}
				return tbapi.Message{}, nil
			},
		}

		adm := admin{
			tbAPI:       mockAPI,
			adminChatID: 123,
		}

		query := &tbapi.CallbackQuery{
			ID:   "test",
			Data: "!5519827604:123",
			Message: &tbapi.Message{
				MessageID: 123,
				Chat:      tbapi.Chat{ID: 456},
				Text:      "permanently banned surkova_vlada Влада Суркова\n\nЕсть способ заработка от 35 000 р в неделю.",
				From:      &tbapi.User{UserName: "bot"},
			},
			From: &tbapi.User{UserName: "admin", ID: 111},
		}

		adm.locator = &mocks.LocatorMock{
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{
					Checks: []spamcheck.Response{
						{Name: "stopword", Spam: true, Details: "заработка"},
					},
				}, true
			},
		}

		err := adm.callbackShowInfo(context.Background(), query)
		require.NoError(t, err)

		assert.Equal(t, 1, sendAttempts, "Should succeed on first HTML attempt")
		assert.Equal(t, tbapi.ModeHTML, usedParseMode, "Should use HTML parse mode")
	})
}
