package events

import (
	"context"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestTelegramListener_DoWithExtraDeleteIDs(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}

	deletedMessages := []int{}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "user"}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {

			if delConfig, ok := c.(tbapi.DeleteMessageConfig); ok {
				deletedMessages = append(deletedMessages, delConfig.MessageID)
			}
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
	}

	b := &mocks.BotMock{
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			t.Logf("on-message: %+v", msg)
			if msg.Text == "spam spam spam" {

				return bot.Response{
					Send:        true,
					Text:        "duplicate spam detected",
					BanInterval: time.Hour,
					User:        bot.User{Username: "user", ID: 1},
					CheckResults: []spamcheck.Response{
						{
							Name:           "duplicate",
							Spam:           true,
							Details:        "message repeated 3 times",
							ExtraDeleteIDs: []int{100, 101},
						},
					},
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
		Bot:        b,
		Group:      "gr",
		Locator:    locator,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 102,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "spam spam spam",
			From:      &tbapi.User{UserName: "user", ID: 1},
			Date:      int(time.Now().Unix()),
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	assert.Contains(t, deletedMessages, 100, "should delete first duplicate")
	assert.Contains(t, deletedMessages, 101, "should delete second duplicate")

	assert.Len(t, mockAPI.RequestCalls(), 3, "should have 1 ban + 2 delete requests")
}

func TestTelegramListener_DoWithExtraDeleteIDs_SuperUser(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}

	deletedMessages := []int{}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "user"}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {

			if delConfig, ok := c.(tbapi.DeleteMessageConfig); ok {
				deletedMessages = append(deletedMessages, delConfig.MessageID)
			}
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
	}

	b := &mocks.BotMock{
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			if msg.Text == "spam spam spam" {

				return bot.Response{
					Send:        true,
					Text:        "duplicate spam detected",
					BanInterval: time.Hour,
					User:        bot.User{Username: "user", ID: 1},
					CheckResults: []spamcheck.Response{
						{
							Name:           "duplicate",
							Spam:           true,
							Details:        "message repeated 3 times",
							ExtraDeleteIDs: []int{100, 101},
						},
					},
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
		Bot:        b,
		Group:      "gr",
		Locator:    locator,
		SuperUsers: SuperUsers{"1"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 102,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "spam spam spam",
			From:      &tbapi.User{UserName: "user", ID: 1},
			Date:      int(time.Now().Unix()),
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	assert.Empty(t, deletedMessages, "superuser extra deletions should be skipped")

	assert.Empty(t, mockAPI.RequestCalls(), "should not call Request for superuser")
}

func TestTelegramListener_DoWithForwarded(t *testing.T) {
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
	b := &mocks.BotMock{
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			t.Logf("on-message: %+v", msg)
			if msg.Text == "text 123" && msg.From.Username == "user" {
				return bot.Response{Send: true, Text: "bot's answer"}
			}

			if msg.WithForward && msg.Text == "" {
				return bot.Response{Send: true, Text: "detected forwarded spam"}
			}

			return bot.Response{}
		},
		UpdateSpamFunc: func(msg string) error {
			t.Logf("update-spam: %s", msg)
			return nil
		},
		RemoveApprovedUserFunc: func(id int64) error { return nil },
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        b,
		Group:      "gr",
		AdminGroup: "123",
		StartupMsg: "startup",
		SuperUsers: SuperUsers{"moderator"},
		Locator:    locator,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	err := l.Locator.AddMessage(ctx, "text 123", 123, 88, "user", 999999)
	require.NoError(t, err)

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat:          tbapi.Chat{ID: 123},
			Text:          "text 123",
			From:          &tbapi.User{UserName: "moderator", ID: 77},
			Date:          int(time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()),
			ForwardOrigin: &tbapi.MessageOrigin{SenderUserName: "forwarded_name"},
			MessageID:     999999,
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err = l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	assert.Empty(t, mockLogger.SaveCalls())

	require.Len(t, mockAPI.SendCalls(), 2)
	assert.Equal(t, "startup", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
	assert.Contains(t, mockAPI.SendCalls()[1].C.(tbapi.MessageConfig).Text, "исходная диагностика")
	assert.Equal(t, int64(123), mockAPI.SendCalls()[1].C.(tbapi.MessageConfig).ChatID)

	require.Len(t, b.UpdateSpamCalls(), 1)
	assert.Equal(t, "text 123", b.UpdateSpamCalls()[0].Msg)

	assert.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Equal(t, 999999, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[1].C.(tbapi.BanChatMemberConfig).ChatID)
	assert.Equal(t, int64(88), mockAPI.RequestCalls()[1].C.(tbapi.BanChatMemberConfig).UserID)

	assert.Len(t, b.RemoveApprovedUserCalls(), 1)
	assert.Equal(t, int64(88), b.RemoveApprovedUserCalls()[0].ID)
}

func TestTelegramListener_DoWithDirectSpamReport(t *testing.T) {
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
		RemoveApprovedUserFunc: func(id int64) error {
			return nil
		},
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			t.Logf("on-message: %+v", msg)
			if msg.Text == "text 123" && msg.From.Username == "user" {
				return bot.Response{Send: true, Text: "bot's answer"}
			}
			return bot.Response{}
		},
		UpdateSpamFunc: func(msg string) error {
			t.Logf("update-spam: %s", msg)
			return nil
		},
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        botMock,
		Group:      "gr",
		StartupMsg: "startup",
		SuperUsers: SuperUsers{"superuser1"},
		Locator:    locator,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat: tbapi.Chat{ID: 123},
			Text: "/SpAm",
			From: &tbapi.User{UserName: "superuser1", ID: 77},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999999,
				From:      &tbapi.User{ID: 666, UserName: "user"},
				Text:      "text 123",
			},
		},
	}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)

	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	assert.Empty(t, mockLogger.SaveCalls())

	require.Len(t, mockAPI.SendCalls(), 2)
	assert.Equal(t, "startup", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
	assert.Contains(t, mockAPI.SendCalls()[1].C.(tbapi.MessageConfig).Text, "исходная диагностика")
	assert.Contains(t, mockAPI.SendCalls()[1].C.(tbapi.MessageConfig).Text, `пользователь забанен администратором [superuser1](tg://user?id=77)`)

	require.Len(t, botMock.OnMessageCalls(), 1)
	assert.Equal(t, "text 123", botMock.OnMessageCalls()[0].Msg.Text)
	assert.True(t, botMock.OnMessageCalls()[0].CheckOnly)

	require.Len(t, botMock.UpdateSpamCalls(), 1)
	assert.Equal(t, "text 123", botMock.UpdateSpamCalls()[0].Msg)

	require.Len(t, botMock.RemoveApprovedUserCalls(), 1)
	assert.Equal(t, int64(666), botMock.RemoveApprovedUserCalls()[0].ID)

	require.Len(t, mockAPI.RequestCalls(), 3)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Equal(t, 999999, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Equal(t, 0, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[2].C.(tbapi.BanChatMemberConfig).ChatID)
	assert.Equal(t, int64(666), mockAPI.RequestCalls()[2].C.(tbapi.BanChatMemberConfig).UserID)
}

func TestTelegramListener_DoWithDirectWarnReport(t *testing.T) {
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
	b := &mocks.BotMock{
		RemoveApprovedUserFunc: func(id int64) error {
			return nil
		},
		OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
			t.Logf("on-message: %+v", msg)
			if msg.Text == "text 123" && msg.From.Username == "user" {
				return bot.Response{Send: true, Text: "bot's answer"}
			}
			return bot.Response{}
		},
		UpdateSpamFunc: func(msg string) error {
			t.Logf("update-spam: %s", msg)
			return nil
		},
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        b,
		Group:      "gr",
		StartupMsg: "startup",
		SuperUsers: SuperUsers{"superuser1"},
		Locator:    locator,
		WarnMsg:    "Не нарушайте правила чата.",
		ModerationConfig: ModerationConfig{
			WarnStrikes: 3,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat: tbapi.Chat{ID: 123},
			Text: "/wArn",
			From: &tbapi.User{UserName: "superuser1", ID: 77},
			ReplyToMessage: &tbapi.Message{
				MessageID: 999999,
				From:      &tbapi.User{ID: 666, UserName: "user"},
				Text:      "text 123",
			},
		},
	}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)

	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	assert.Empty(t, mockLogger.SaveCalls())

	require.Len(t, mockAPI.SendCalls(), 2)
	assert.Equal(t, "startup", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
	assert.True(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).DisableNotification)
	assert.Contains(t, mockAPI.SendCalls()[1].C.(tbapi.MessageConfig).Text, "Предупреждение 1/3")
	assert.Contains(t, mockAPI.SendCalls()[1].C.(tbapi.MessageConfig).Text, `Не нарушайте правила чата`)

	require.Empty(t, b.OnMessageCalls())
	require.Empty(t, b.UpdateSpamCalls())
	require.Empty(t, b.RemoveApprovedUserCalls())

	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Equal(t, 999999, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(123), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Equal(t, 0, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
}
