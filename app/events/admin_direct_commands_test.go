package events

import (
	"context"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/observability"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdmin_DirectCommands(t *testing.T) {

	setupTest := func() (*mocks.TbAPIMock, *mocks.BotMock, *admin, func()) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {

				return &tbapi.APIResponse{Ok: true}, nil
			},
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
				return storage.MsgMeta{}, true
			},
			SpamFunc: func(ctx context.Context, userID int64) (storage.SpamData, bool) {
				return storage.SpamData{}, true
			},
			UserNameByIDFunc: func(ctx context.Context, userID int64) string {
				return "testuser"
			},
			UserIDByNameFunc: func(ctx context.Context, userName string) int64 {
				if userName == "spammer" {
					return 222
				}
				return 0
			},
		}

		adm := &admin{
			tbAPI:              mockAPI,
			bot:                botMock,
			primChatID:         123,
			adminChatID:        456,
			locator:            locatorMock,
			superUsers:         SuperUsers{"superuser"},
			warnMsg:            "Не нарушайте правила чата.",
			moderation:         ModerationConfig{WarnStrikes: 3, FirstStrike: 30 * time.Minute, SecondStrike: 6 * time.Hour},
			warnDeleteDuration: time.Minute,
		}

		teardown := func() {}
		return mockAPI, botMock, adm, teardown
	}

	verifyDirectReportResults := func(t *testing.T, mockAPI *mocks.TbAPIMock, botMock *mocks.BotMock) {

		require.GreaterOrEqual(t, len(mockAPI.RequestCalls()), 3)

		assert.Equal(t, 999, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)

		assert.Equal(t, 789, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)

		require.Len(t, mockAPI.SendCalls(), 1)
		adminMsg := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
		assert.Equal(t, int64(456), adminMsg.ChatID)
		assert.Contains(t, adminMsg.Text, "исходная диагностика для spammer (222)")
		assert.Contains(t, adminMsg.Text, "пользователь забанен администратором [admin](tg://user?id=111)")

		require.Len(t, botMock.RemoveApprovedUserCalls(), 1)
		assert.Equal(t, int64(222), botMock.RemoveApprovedUserCalls()[0].ID)

		require.Len(t, botMock.OnMessageCalls(), 1)
		assert.Equal(t, "spam message text", botMock.OnMessageCalls()[0].Msg.Text)
		assert.Equal(t, int64(222), botMock.OnMessageCalls()[0].Msg.From.ID)
		assert.True(t, botMock.OnMessageCalls()[0].CheckOnly)
	}

	createReplyUpdate := func(adminName string, adminID int64, spammerName string, spammerID int64, text string) tbapi.Update {
		return tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 123},
				From:      &tbapi.User{UserName: adminName, ID: adminID},
				ReplyToMessage: &tbapi.Message{
					MessageID: 999,
					From:      &tbapi.User{UserName: spammerName, ID: spammerID},
					Text:      text,
				},
			},
		}
	}

	t.Run("DirectBanReport", func(t *testing.T) {
		mockAPI, botMock, adm, teardown := setupTest()
		defer teardown()

		update := createReplyUpdate("admin", 111, "spammer", 222, "spam message text")

		err := adm.DirectBanReport(context.Background(), update)
		require.NoError(t, err)

		verifyDirectReportResults(t, mockAPI, botMock)

		assert.Empty(t, botMock.UpdateSpamCalls())
	})

	t.Run("DirectBanTarget_ByID", func(t *testing.T) {
		mockAPI, botMock, adm, teardown := setupTest()
		defer teardown()

		update := createReplyUpdate("admin", 111, "", 0, "")
		update.Message.Text = "/ban 222"
		update.Message.ReplyToMessage = nil

		err := adm.DirectBanTarget(context.Background(), update, "222")
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "[testuser](tg://user?id=222) (222)")
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "[admin](tg://user?id=111)")
		require.Len(t, mockAPI.RequestCalls(), 2)
		assert.Equal(t, 789, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
		banCfg := mockAPI.RequestCalls()[1].C.(tbapi.BanChatMemberConfig)
		assert.Equal(t, int64(123), banCfg.ChatID)
		assert.Equal(t, int64(222), banCfg.UserID)
		require.Len(t, botMock.RemoveApprovedUserCalls(), 1)
		assert.Equal(t, int64(222), botMock.RemoveApprovedUserCalls()[0].ID)
	})

	t.Run("DirectBanTarget_ByIDWithoutKnownUsername", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		adm.locator = &mocks.LocatorMock{
			UserNameByIDFunc: func(ctx context.Context, userID int64) string { return "" },
			UserIDByNameFunc: func(ctx context.Context, userName string) int64 { return 0 },
		}
		update := createReplyUpdate("admin", 111, "", 0, "")
		update.Message.Text = "/ban 8642668745"
		update.Message.ReplyToMessage = nil

		err := adm.DirectBanTarget(context.Background(), update, "8642668745")
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		text := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text
		assert.Contains(t, text, "пользователь [8642668745](tg://user?id=8642668745) забанен")
		assert.NotContains(t, text, "user 8642668745")
		assert.NotContains(t, text, "](tg://user?id=8642668745) (8642668745)")
	})

	t.Run("DirectBanTarget_ByUsernameWithAt", func(t *testing.T) {
		mockAPI, botMock, adm, teardown := setupTest()
		defer teardown()

		update := createReplyUpdate("admin", 111, "", 0, "")
		update.Message.Text = "/ban @spammer"
		update.Message.ReplyToMessage = nil

		err := adm.DirectBanTarget(context.Background(), update, "@spammer")
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "[spammer](tg://user?id=222) (222)")
		require.Len(t, mockAPI.RequestCalls(), 2)
		banCfg := mockAPI.RequestCalls()[1].C.(tbapi.BanChatMemberConfig)
		assert.Equal(t, int64(222), banCfg.UserID)
		require.Len(t, botMock.RemoveApprovedUserCalls(), 1)
		assert.Equal(t, int64(222), botMock.RemoveApprovedUserCalls()[0].ID)
	})

	t.Run("DirectBanTarget_ByUsernameWithoutAt", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		update := createReplyUpdate("admin", 111, "", 0, "")
		update.Message.Text = "/ban spammer"
		update.Message.ReplyToMessage = nil

		err := adm.DirectBanTarget(context.Background(), update, "spammer")
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "[spammer](tg://user?id=222) (222)")
		require.Len(t, mockAPI.RequestCalls(), 2)
		banCfg := mockAPI.RequestCalls()[1].C.(tbapi.BanChatMemberConfig)
		assert.Equal(t, int64(222), banCfg.UserID)
	})

	t.Run("DirectBanTarget_UnknownUsername", func(t *testing.T) {
		mockAPI, botMock, adm, teardown := setupTest()
		defer teardown()

		update := createReplyUpdate("admin", 111, "", 0, "")
		update.Message.Text = "/ban @unknown"
		update.Message.ReplyToMessage = nil

		err := adm.DirectBanTarget(context.Background(), update, "@unknown")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `can't resolve username "unknown" to user id`)
		assert.Empty(t, mockAPI.SendCalls())
		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, botMock.RemoveApprovedUserCalls())
	})

	t.Run("DirectSpamReport", func(t *testing.T) {
		mockAPI, botMock, adm, teardown := setupTest()
		defer teardown()

		update := createReplyUpdate("admin", 111, "spammer", 222, "spam message text")

		err := adm.DirectSpamReport(context.Background(), update)
		require.NoError(t, err)

		verifyDirectReportResults(t, mockAPI, botMock)

		require.Len(t, botMock.UpdateSpamCalls(), 1)
		assert.Equal(t, "spam message text", botMock.UpdateSpamCalls()[0].Msg)
	})

	t.Run("DirectReport_DryMode", func(t *testing.T) {
		mockAPI, botMock, adm, teardown := setupTest()
		defer teardown()
		adm.dry = true

		update := createReplyUpdate("admin", 111, "spammer", 222, "spam message text")

		err := adm.DirectSpamReport(context.Background(), update)
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		adminMsg := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
		assert.Equal(t, int64(456), adminMsg.ChatID)
		assert.Contains(t, adminMsg.Text, "исходная диагностика для spammer (222)")

		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, botMock.UpdateSpamCalls())
	})

	t.Run("DirectDeleteReply", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		update := createReplyUpdate("admin", 111, "bot", 777, "message to delete")
		update.Message.Text = "/del"
		update.Message.MessageID = 789
		update.Message.Chat.ID = 456
		update.Message.ReplyToMessage.MessageID = 999
		update.Message.ReplyToMessage.Chat.ID = 456

		err := adm.DirectDeleteReply(context.Background(), update)
		require.NoError(t, err)

		require.Len(t, mockAPI.RequestCalls(), 2)
		assert.Equal(t, 999, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
		assert.Equal(t, int64(456), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
		assert.Equal(t, 789, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
		assert.Equal(t, int64(456), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)
	})

	t.Run("DirectDeleteByID", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		update := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 456},
				From:      &tbapi.User{UserName: "admin", ID: 111},
				Text:      "/del 999",
			},
		}

		err := adm.DirectDeleteByID(context.Background(), update, 456, 999)
		require.NoError(t, err)

		require.Len(t, mockAPI.RequestCalls(), 2)
		assert.Equal(t, 999, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
		assert.Equal(t, int64(456), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
		assert.Equal(t, 789, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
		assert.Equal(t, int64(456), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)
	})

	t.Run("DirectWarnReport", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		update := createReplyUpdate("admin", 111, "user", 222, "inappropriate message")

		err := adm.DirectWarnReport(update)
		require.NoError(t, err)

		require.Len(t, mockAPI.RequestCalls(), 2)

		assert.Equal(t, 999, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)

		assert.Equal(t, 789, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)

		require.Len(t, mockAPI.SendCalls(), 1)
		warnMsg := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
		assert.Equal(t, int64(123), warnMsg.ChatID)
		assert.Contains(t, warnMsg.Text, "Предупреждение 1/3")
		assert.Contains(t, warnMsg.Text, "Не нарушайте правила чата")
	})

	t.Run("DirectWarnReport_UsesActionExecutor", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		actionSpy := &actionExecutorSpy{}
		detectedSpy := &detectedSpamCounterSpy{}
		adm.actions = actionSpy
		adm.detectedSpam = detectedSpy

		update := createReplyUpdate("admin", 111, "user", 222, "inappropriate message")
		err := adm.DirectWarnReport(update)
		require.NoError(t, err)

		require.Len(t, actionSpy.deleteMessageCalls, 2)
		assert.Equal(t, 999, actionSpy.deleteMessageCalls[0].MsgID)
		assert.Equal(t, 789, actionSpy.deleteMessageCalls[1].MsgID)

		require.Len(t, actionSpy.warnCalls, 1)
		assert.Equal(t, int64(123), actionSpy.warnCalls[0].chatID)
		assert.Equal(t, int64(222), actionSpy.warnCalls[0].subjectID)
		assert.Equal(t, 999, actionSpy.warnCalls[0].messageID)
		assert.Equal(t, time.Minute, actionSpy.warnCalls[0].warnDelTime)
		assert.Contains(t, actionSpy.warnCalls[0].text, "Предупреждение 1/3")
		assert.Contains(t, actionSpy.warnCalls[0].text, "Не нарушайте правила чата.")
		require.Len(t, detectedSpy.writes, 1)
		assert.Equal(t, int64(222), detectedSpy.writes[0].UserID)

		meta, ok := observability.MetadataFromContext(actionSpy.warnCtxs[0])
		require.True(t, ok)
		assert.Equal(t, "warn-123-999", meta.EventID)
		assert.Equal(t, "corr-warn-999", meta.CorrelationID)
		assert.Equal(t, "warn:chat:123:msg:999:cmd:789", meta.IdempotencyKey)

		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, mockAPI.SendCalls())
	})

	t.Run("DirectWarnReport_EscalatesAfterWarnLimit", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		actionSpy := &actionExecutorSpy{}
		adm.actions = actionSpy
		adm.detectedSpam = &detectedSpamCounterSpy{count: 3}

		update := createReplyUpdate("admin", 111, "user", 222, "inappropriate message")
		err := adm.DirectWarnReport(update)
		require.NoError(t, err)

		require.Len(t, actionSpy.deleteMessageCalls, 2)
		assert.Empty(t, actionSpy.warnCalls)
		require.Len(t, actionSpy.banCalls, 1)
		assert.Equal(t, int64(222), actionSpy.banCalls[0].userID)
		assert.True(t, actionSpy.banCalls[0].restrict)
		assert.Equal(t, 30*time.Minute, actionSpy.banCalls[0].duration)
		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, mockAPI.SendCalls())
	})

	t.Run("DirectUnwarnReport_RemovesLatestManualWarning", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		detectedSpy := &detectedSpamCounterSpy{count: 2, deleteResult: true}
		adm.detectedSpam = detectedSpy

		update := createReplyUpdate("admin", 111, "bot", 777, "")
		update.Message.Text = "/unwarn"
		update.Message.ReplyToMessage.Text = `<b>⚠️ WARNING 2/3</b> <a href="https://t.me/user">Firstname</a> (222)`

		err := adm.DirectUnwarnReport(update)
		require.NoError(t, err)

		require.Len(t, mockAPI.RequestCalls(), 1)
		assert.Equal(t, 789, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
		require.Len(t, detectedSpy.deleteByIDCalls, 1)
		assert.Equal(t, int64(222), detectedSpy.deleteByIDCalls[0].userID)
		assert.Equal(t, manualWarnSignalSource, detectedSpy.deleteByIDCalls[0].signalSource)
		require.Len(t, mockAPI.SendCalls(), 1)
		adminMsg := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
		assert.Contains(t, adminMsg.Text, "Предупреждение снято")
		assert.Contains(t, adminMsg.Text, "осталось 2")
	})

	t.Run("DirectUnwarnReport_NoManualWarning", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		adm.detectedSpam = &detectedSpamCounterSpy{deleteResult: false}

		update := createReplyUpdate("admin", 111, "user", 222, "regular message")
		update.Message.Text = "/unwarn"

		err := adm.DirectUnwarnReport(update)
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, "Предупреждений для user (222) не найдено")
	})

	t.Run("DirectSpamReport_ChannelMessage", func(t *testing.T) {
		mockAPI, botMock, adm, teardown := setupTest()
		defer teardown()

		update := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 123},
				From:      &tbapi.User{UserName: "admin", ID: 111},
				ReplyToMessage: &tbapi.Message{
					MessageID:  999,
					From:       &tbapi.User{UserName: "Channel_Bot", ID: 136817688},
					SenderChat: &tbapi.Chat{ID: 12345, UserName: "spam_channel"},
					Text:       "spam message text",
				},
			},
		}

		err := adm.DirectSpamReport(context.Background(), update)
		require.NoError(t, err)

		// verify ban used BanChatSenderChatConfig with channel ID, not BanChatMemberConfig
		var foundChannelBan bool
		for _, call := range mockAPI.RequestCalls() {
			if banCfg, ok := call.C.(tbapi.BanChatSenderChatConfig); ok {
				foundChannelBan = true
				assert.Equal(t, int64(12345), banCfg.SenderChatID)
				assert.Equal(t, int64(123), banCfg.ChatID)
			}
		}
		assert.True(t, foundChannelBan, "expected BanChatSenderChatConfig for channel message")

		for _, call := range mockAPI.RequestCalls() {
			_, isMemberBan := call.C.(tbapi.BanChatMemberConfig)
			assert.False(t, isMemberBan, "should not use BanChatMemberConfig for channel message")
		}

		require.Len(t, botMock.UpdateSpamCalls(), 1)

		require.Len(t, mockAPI.SendCalls(), 1)
		adminMsg := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
		assert.Contains(t, adminMsg.Text, "spam\\_channel")
		assert.Contains(t, adminMsg.Text, "12345")
		assert.NotContains(t, adminMsg.Text, "Channel\\_Bot")
		assert.NotContains(t, adminMsg.Text, "136817688")
	})

	t.Run("DirectSpamReport_AnonymousAdmin", func(t *testing.T) {
		mockAPI, botMock, adm, teardown := setupTest()
		defer teardown()

		update := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 123},
				From:      &tbapi.User{UserName: "admin", ID: 111},
				ReplyToMessage: &tbapi.Message{
					MessageID:  999,
					From:       &tbapi.User{UserName: "GroupLinkedChannel", ID: 136817688},
					SenderChat: &tbapi.Chat{ID: 123, UserName: "the_group"},
					Text:       "admin message",
				},
			},
		}

		err := adm.DirectSpamReport(context.Background(), update)
		require.NoError(t, err)

		for _, call := range mockAPI.RequestCalls() {
			_, isChannelBan := call.C.(tbapi.BanChatSenderChatConfig)
			assert.False(t, isChannelBan, "should not use BanChatSenderChatConfig for anonymous admin post")
		}

		require.Len(t, botMock.UpdateSpamCalls(), 1)
	})

	t.Run("DirectWarnReport_ChannelMessage", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		update := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 123},
				From:      &tbapi.User{UserName: "admin", ID: 111},
				ReplyToMessage: &tbapi.Message{
					MessageID:  999,
					From:       &tbapi.User{UserName: "Channel_Bot", ID: 136817688},
					SenderChat: &tbapi.Chat{ID: -100999888, UserName: "spam_channel"},
					Text:       "inappropriate channel message",
				},
			},
		}

		err := adm.DirectWarnReport(update)
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		warnMsg := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
		assert.Equal(t, int64(123), warnMsg.ChatID)
		assert.Contains(t, warnMsg.Text, "spam\\_channel")
		assert.NotContains(t, warnMsg.Text, "@Channel\\_Bot")
		assert.Contains(t, warnMsg.Text, "Предупреждение 1/3")
		assert.Contains(t, warnMsg.Text, "Не нарушайте правила чата")
	})

	t.Run("DirectWarnReport_ChannelMessage_TitleOnly", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		update := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 123},
				From:      &tbapi.User{UserName: "admin", ID: 111},
				ReplyToMessage: &tbapi.Message{
					MessageID:  999,
					From:       &tbapi.User{UserName: "Channel_Bot", ID: 136817688},
					SenderChat: &tbapi.Chat{ID: -100999888, Title: "Spam Channel"},
					Text:       "inappropriate channel message",
				},
			},
		}

		err := adm.DirectWarnReport(update)
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		warnMsg := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
		assert.Equal(t, int64(123), warnMsg.ChatID)

		assert.Contains(t, warnMsg.Text, "Spam Channel")
		assert.NotContains(t, warnMsg.Text, "@Spam Channel")
		assert.NotContains(t, warnMsg.Text, "@Channel")
		assert.Contains(t, warnMsg.Text, "Предупреждение 1/3")
		assert.Contains(t, warnMsg.Text, "Не нарушайте правила чата")
	})

	t.Run("DirectSpamReport_ChannelMessage_TitleOnly", func(t *testing.T) {
		mockAPI, botMock, adm, teardown := setupTest()
		defer teardown()

		update := tbapi.Update{
			Message: &tbapi.Message{
				MessageID: 789,
				Chat:      tbapi.Chat{ID: 123},
				From:      &tbapi.User{UserName: "admin", ID: 111},
				ReplyToMessage: &tbapi.Message{
					MessageID:  999,
					From:       &tbapi.User{UserName: "Channel_Bot", ID: 136817688},
					SenderChat: &tbapi.Chat{ID: 12345, Title: "Spam Channel"},
					Text:       "spam message text",
				},
			},
		}

		err := adm.DirectSpamReport(context.Background(), update)
		require.NoError(t, err)

		require.Len(t, mockAPI.SendCalls(), 1)
		adminMsg := mockAPI.SendCalls()[0].C.(tbapi.MessageConfig)
		assert.Contains(t, adminMsg.Text, "Spam Channel")
		assert.Contains(t, adminMsg.Text, "12345")
		assert.NotContains(t, adminMsg.Text, "Channel\\_Bot")

		require.Len(t, botMock.UpdateSpamCalls(), 1)
	})

	t.Run("DirectWarnReport_SuperUser", func(t *testing.T) {
		mockAPI, _, adm, teardown := setupTest()
		defer teardown()

		update := createReplyUpdate("admin", 111, "superuser", 222, "inappropriate message")

		err := adm.DirectWarnReport(update)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "warn message is from super-user")

		assert.Empty(t, mockAPI.RequestCalls())
		assert.Empty(t, mockAPI.SendCalls())
	})
}

func TestAdmin_WarningNotSpamCallback(t *testing.T) {
	t.Run("ask confirmation", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
		}
		adm := admin{tbAPI: mockAPI, adminChatID: 456}

		query := &tbapi.CallbackQuery{
			Data: "W?7187750383:777",
			Message: &tbapi.Message{
				MessageID: 100,
				Chat:      tbapi.Chat{ID: 456},
				Text:      "<b>⚠️ WARNING 1/3</b> Dorothy (7187750383)\n\nfalse positive\n\n",
			},
		}

		err := adm.InlineCallbackHandler(context.Background(), query)
		require.NoError(t, err)
		require.Len(t, mockAPI.SendCalls(), 1)
		edit := mockAPI.SendCalls()[0].C.(tbapi.EditMessageReplyMarkupConfig)
		buttons := edit.ReplyMarkup.InlineKeyboard[0]
		require.Len(t, buttons, 2)
		assert.Equal(t, "Да, ham", buttons[0].Text)
		require.NotNil(t, buttons[0].CallbackData)
		assert.Equal(t, "W+7187750383:777", *buttons[0].CallbackData)
		assert.Equal(t, "Отмена", buttons[1].Text)
	})

	t.Run("confirm updates ham", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
				return &tbapi.APIResponse{Ok: true}, nil
			},
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
		}
		botMock := &mocks.BotMock{UpdateHamFunc: func(msg string) error { return nil }}
		autoLearner := &autoLearnerSpy{}
		detectedSpy := &detectedSpamCounterSpy{count: 2, deleteResult: true}
		adm := admin{tbAPI: mockAPI, bot: botMock, adminChatID: 456, autoLearner: autoLearner, detectedSpam: detectedSpy}

		query := &tbapi.CallbackQuery{
			ID:   "callback-id",
			Data: "W+7187750383:777",
			From: &tbapi.User{ID: 111, UserName: "admin"},
			Message: &tbapi.Message{
				MessageID: 100,
				Date:      int(time.Now().Unix()),
				Chat:      tbapi.Chat{ID: 456},
				Text: `<b>⚠️ WARNING 1/3</b> Dorothy (7187750383)

Проводим инвайтинг в чаты, рассылку по группам

Причина: classifier`,
			},
		}

		err := adm.InlineCallbackHandler(context.Background(), query)
		require.NoError(t, err)
		require.Len(t, botMock.UpdateHamCalls(), 1)
		assert.Equal(t, "Проводим инвайтинг в чаты, рассылку по группам", botMock.UpdateHamCalls()[0].Msg)
		require.Len(t, autoLearner.hamCalls, 1)
		assert.Equal(t, "Проводим инвайтинг в чаты, рассылку по группам", autoLearner.hamCalls[0].text)
		assert.Equal(t, "admin", autoLearner.hamCalls[0].labeledBy)
		require.Len(t, detectedSpy.deleteByIDCalls, 2)
		assert.Equal(t, int64(7187750383), detectedSpy.deleteByIDCalls[0].userID)
		require.Len(t, detectedSpy.deleteLatestByIDCalls, 2)
		assert.Equal(t, int64(7187750383), detectedSpy.deleteLatestByIDCalls[0])
		require.Len(t, mockAPI.SendCalls(), 1)
		edit := mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig)
		assert.Contains(t, edit.Text, "ham подтвержден администратором [admin](tg://user?id=111)")
		require.NotNil(t, edit.ReplyMarkup)
		assert.Empty(t, edit.ReplyMarkup.InlineKeyboard)
	})

	t.Run("cancel restores not-spam and info buttons", func(t *testing.T) {
		mockAPI := &mocks.TbAPIMock{
			SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
		}
		adm := admin{tbAPI: mockAPI, adminChatID: 456}

		query := &tbapi.CallbackQuery{
			Data: "WX7187750383:777",
			Message: &tbapi.Message{
				MessageID: 100,
				Chat:      tbapi.Chat{ID: 456},
				Text:      "<b>⚠️ WARNING 1/3</b> Dorothy (7187750383)\n\nfalse positive\n\n",
			},
		}

		err := adm.InlineCallbackHandler(context.Background(), query)
		require.NoError(t, err)
		require.Len(t, mockAPI.SendCalls(), 1)
		edit := mockAPI.SendCalls()[0].C.(tbapi.EditMessageReplyMarkupConfig)
		buttons := edit.ReplyMarkup.InlineKeyboard[0]
		require.Len(t, buttons, 2)
		assert.Equal(t, "Не спам", buttons[0].Text)
		require.NotNil(t, buttons[0].CallbackData)
		assert.Equal(t, "W?7187750383:777", *buttons[0].CallbackData)
		assert.Equal(t, "⚑ info", buttons[1].Text)
		require.NotNil(t, buttons[1].CallbackData)
		assert.Equal(t, "!7187750383:777", *buttons[1].CallbackData)
	})
}

type autoLearnerSpy struct {
	hamCalls []struct {
		text      string
		labeledBy string
	}
}

func (s *autoLearnerSpy) LearnSpam(context.Context, string, string) {}

func (s *autoLearnerSpy) LearnHam(_ context.Context, text, labeledBy string) {
	s.hamCalls = append(s.hamCalls, struct {
		text      string
		labeledBy string
	}{text: text, labeledBy: labeledBy})
}
