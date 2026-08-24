package events

import (
	"context"
	"fmt"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegramListener_DoWithDirectWarnReportUsesActionExecutor(t *testing.T) {
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
	actionSpy := &actionExecutorSpy{}
	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        &mocks.BotMock{},
		Group:      "gr",
		StartupMsg: "startup",
		SuperUsers: SuperUsers{"superuser1"},
		Locator:    &locatorContextSpy{},
		WarnMsg:    "Не нарушайте правила чата.",
		ModerationConfig: ModerationConfig{
			WarnStrikes:        3,
			WarnDeleteDuration: time.Minute,
		},
		ActionExecutor: actionSpy,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat: tbapi.Chat{ID: 123},
			Text: "/warn",
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

	require.Len(t, actionSpy.deleteMessageCalls, 2)
	assert.Equal(t, 999999, actionSpy.deleteMessageCalls[0].MsgID)
	assert.Equal(t, 0, actionSpy.deleteMessageCalls[1].MsgID)
	require.Len(t, actionSpy.warnCalls, 1)
	assert.Equal(t, int64(123), actionSpy.warnCalls[0].chatID)
	assert.Equal(t, int64(666), actionSpy.warnCalls[0].subjectID)
	assert.Equal(t, 999999, actionSpy.warnCalls[0].messageID)
	assert.Equal(t, time.Minute, actionSpy.warnCalls[0].warnDelTime)
	assert.Contains(t, actionSpy.warnCalls[0].text, "Предупреждение 1/3")
	assert.Contains(t, actionSpy.warnCalls[0].text, "Не нарушайте правила чата.")

	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Equal(t, "startup", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
	assert.Empty(t, mockAPI.RequestCalls())
}

func TestTelegramListener_DoWithDirectDeleteReply(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        &mocks.BotMock{},
		Group:      "gr",
		SuperUsers: SuperUsers{"superuser1"},
		Locator:    &locatorContextSpy{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{Message: &tbapi.Message{
		MessageID: 101,
		Chat:      tbapi.Chat{ID: 456},
		Text:      "/del",
		From:      &tbapi.User{UserName: "superuser1", ID: 77},
		ReplyToMessage: &tbapi.Message{
			MessageID: 202,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{ID: 777, UserName: "bot"},
			Text:      "bot message",
		},
	}}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, 202, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(456), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Equal(t, 101, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(456), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Empty(t, mockLogger.SaveCalls())
}

func TestTelegramListener_DirectDeleteReplyRequiresSuperUser(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response { return bot.Response{} }}
	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        botMock,
		Group:      "gr",
		SuperUsers: SuperUsers{"superuser1"},
		Locator:    &locatorContextSpy{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{Message: &tbapi.Message{
		MessageID: 101,
		Chat:      tbapi.Chat{ID: 456},
		Text:      "/del",
		From:      &tbapi.User{UserName: "linked", ID: 77},
		SenderChat: &tbapi.Chat{
			ID: 123,
		},
		ReplyToMessage: &tbapi.Message{
			MessageID: 202,
			Chat:      tbapi.Chat{ID: 456},
			From:      &tbapi.User{ID: 777, UserName: "bot"},
			Text:      "bot message",
		},
	}}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	assert.Empty(t, mockAPI.RequestCalls())
	assert.Empty(t, botMock.OnMessageCalls())
}

func TestTelegramListener_DoWithDirectDeleteByID(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        &mocks.BotMock{},
		Group:      "gr",
		SuperUsers: SuperUsers{"superuser1"},
		Locator:    &locatorContextSpy{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{Message: &tbapi.Message{
		MessageID: 101,
		Chat:      tbapi.Chat{ID: 456},
		Text:      "/del 202",
		From:      &tbapi.User{UserName: "superuser1", ID: 77},
	}}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, 202, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(456), mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Equal(t, 101, mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).MessageID)
	assert.Equal(t, int64(456), mockAPI.RequestCalls()[1].C.(tbapi.DeleteMessageConfig).ChatID)
	assert.Empty(t, mockLogger.SaveCalls())
}

func TestTelegramListener_DoWithDirectBanTarget(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		GetChatMemberFunc: func(config tbapi.GetChatMemberConfig) (tbapi.ChatMember, error) {
			return tbapi.ChatMember{}, fmt.Errorf("Bad Request: user not found")
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	botMock := &mocks.BotMock{
		RemoveApprovedUserFunc: func(id int64) error { return nil },
	}
	locator := &locatorContextSpy{}
	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        botMock,
		Group:      "gr",
		AdminGroup: "admin",
		SuperUsers: SuperUsers{"superuser1"},
		Locator:    locator,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{Message: &tbapi.Message{
		MessageID: 101,
		Chat:      tbapi.Chat{ID: 456},
		Text:      "/ban 222",
		From:      &tbapi.User{UserName: "superuser1", ID: 77},
	}}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text, `<a href="tg://user?id=222">222</a> забанен`)
	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, 101, mockAPI.RequestCalls()[0].C.(tbapi.DeleteMessageConfig).MessageID)
	banCfg := mockAPI.RequestCalls()[1].C.(tbapi.BanChatMemberConfig)
	assert.Equal(t, int64(123), banCfg.ChatID)
	assert.Equal(t, int64(222), banCfg.UserID)
	require.Len(t, botMock.RemoveApprovedUserCalls(), 1)
	assert.Empty(t, mockLogger.SaveCalls())
}

func TestTelegramListener_DirectBanTargetRequiresSuperUser(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response { return bot.Response{} }}
	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        botMock,
		Group:      "gr",
		SuperUsers: SuperUsers{"superuser1"},
		Locator:    &locatorContextSpy{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	updMsg := tbapi.Update{Message: &tbapi.Message{
		MessageID: 101,
		Chat:      tbapi.Chat{ID: 456},
		Text:      "/ban 222",
		From:      &tbapi.User{UserName: "regular", ID: 77},
	}}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	assert.Empty(t, mockAPI.RequestCalls())
	assert.Empty(t, mockLogger.SaveCalls())
	assert.Empty(t, botMock.RemoveApprovedUserCalls())
}

func TestTelegramListener_DoWithAdminUnBan(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if mc, ok := c.(tbapi.MessageConfig); ok {
				return tbapi.Message{Text: mc.Text, From: &tbapi.User{UserName: "user"}}, nil
			}
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	b := &mocks.BotMock{
		UpdateHamFunc: func(msg string) error {
			return nil
		},
		AddApprovedUserFunc: func(id int64, name string) error { return nil },
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        b,
		SuperUsers: SuperUsers{"admin"},
		Group:      "gr",
		Locator:    locator,
		AdminGroup: "123",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		CallbackQuery: &tbapi.CallbackQuery{
			Data: "777:999",
			Message: &tbapi.Message{
				MessageID:     987654,
				Chat:          tbapi.Chat{ID: 123},
				Text:          "unban user blah\n\nthis was the ham, not spam",
				From:          &tbapi.User{UserName: "user", ID: 999},
				ForwardOrigin: &tbapi.MessageOrigin{Date: time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()},
			},
			From: &tbapi.User{UserName: "admin", ID: 1000},
		},
	}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Equal(t, 987654, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).MessageID)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, `разбанено администратором <a href="tg://user?id=1000">admin</a>`)
	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, "принято", mockAPI.RequestCalls()[0].C.(tbapi.CallbackConfig).Text)

	assert.Equal(t, int64(777), mockAPI.RequestCalls()[1].C.(tbapi.UnbanChatMemberConfig).UserID)
	require.Len(t, b.UpdateHamCalls(), 1)
	assert.Equal(t, "this was the ham, not spam", b.UpdateHamCalls()[0].Msg)
	require.Len(t, b.AddApprovedUserCalls(), 1)
	assert.Equal(t, int64(777), b.AddApprovedUserCalls()[0].ID)
}

func TestTelegramListener_DoWithAdminSoftUnBan(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if mc, ok := c.(tbapi.MessageConfig); ok {
				return tbapi.Message{Text: mc.Text, From: &tbapi.User{UserName: "user"}}, nil
			}
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	b := &mocks.BotMock{
		UpdateHamFunc: func(msg string) error {
			return nil
		},
		AddApprovedUserFunc: func(id int64, name string) error { return nil },
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger:  mockLogger,
		TbAPI:       mockAPI,
		Bot:         b,
		SuperUsers:  SuperUsers{"admin"},
		Group:       "gr",
		Locator:     locator,
		AdminGroup:  "123",
		SoftBanMode: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		CallbackQuery: &tbapi.CallbackQuery{
			Data: "777:999",
			Message: &tbapi.Message{
				MessageID:     987654,
				Chat:          tbapi.Chat{ID: 123},
				Text:          "unban user blah\n\nthis was the ham, not spam",
				From:          &tbapi.User{UserName: "user", ID: 999},
				ForwardOrigin: &tbapi.MessageOrigin{Date: time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()},
			},
			From: &tbapi.User{UserName: "admin", ID: 1000},
		},
	}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Equal(t, 987654, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).MessageID)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, `разбанено администратором <a href="tg://user?id=1000">admin</a>`)
	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, "принято", mockAPI.RequestCalls()[0].C.(tbapi.CallbackConfig).Text)

	assert.Equal(t, int64(777), mockAPI.RequestCalls()[1].C.(tbapi.RestrictChatMemberConfig).UserID)
	assert.Equal(t, &tbapi.ChatPermissions{CanSendMessages: true, CanSendAudios: true, CanSendDocuments: true, CanSendPhotos: true, CanSendVideos: true, CanSendVideoNotes: true, CanSendVoiceNotes: true, CanSendOtherMessages: true, CanChangeInfo: true, CanInviteUsers: true, CanPinMessages: true},
		mockAPI.RequestCalls()[1].C.(tbapi.RestrictChatMemberConfig).Permissions)
	require.Len(t, b.UpdateHamCalls(), 1)
	assert.Equal(t, "this was the ham, not spam", b.UpdateHamCalls()[0].Msg)
	require.Len(t, b.AddApprovedUserCalls(), 1)
	assert.Equal(t, int64(777), b.AddApprovedUserCalls()[0].ID)
}

func TestTelegramListener_DoWithAdminSoftUnBanEmptyText(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if mc, ok := c.(tbapi.MessageConfig); ok {
				return tbapi.Message{Text: mc.Text, From: &tbapi.User{UserName: "user"}}, nil
			}
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	b := &mocks.BotMock{
		UpdateHamFunc: func(msg string) error {
			return nil
		},
		AddApprovedUserFunc: func(id int64, name string) error { return nil },
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger:  mockLogger,
		TbAPI:       mockAPI,
		Bot:         b,
		SuperUsers:  SuperUsers{"admin"},
		Group:       "gr",
		Locator:     locator,
		AdminGroup:  "123",
		SoftBanMode: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		CallbackQuery: &tbapi.CallbackQuery{
			Data: "777:999",
			Message: &tbapi.Message{
				MessageID:     987654,
				Chat:          tbapi.Chat{ID: 123},
				Text:          "unban user blah\n\n",
				From:          &tbapi.User{UserName: "user", ID: 999},
				ForwardOrigin: &tbapi.MessageOrigin{Date: time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()},
			},
			From: &tbapi.User{UserName: "admin", ID: 1000},
		},
	}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Equal(t, 987654, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).MessageID)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, `разбанено администратором <a href="tg://user?id=1000">admin</a>`)
	require.Len(t, mockAPI.RequestCalls(), 2)
	assert.Equal(t, "принято", mockAPI.RequestCalls()[0].C.(tbapi.CallbackConfig).Text)

	assert.Equal(t, int64(777), mockAPI.RequestCalls()[1].C.(tbapi.RestrictChatMemberConfig).UserID)
	assert.Equal(t, &tbapi.ChatPermissions{CanSendMessages: true, CanSendAudios: true, CanSendDocuments: true, CanSendPhotos: true, CanSendVideos: true, CanSendVideoNotes: true, CanSendVoiceNotes: true, CanSendOtherMessages: true, CanChangeInfo: true, CanInviteUsers: true, CanPinMessages: true},
		mockAPI.RequestCalls()[1].C.(tbapi.RestrictChatMemberConfig).Permissions)
	require.Empty(t, b.UpdateHamCalls())
	require.Len(t, b.AddApprovedUserCalls(), 1)
	assert.Equal(t, int64(777), b.AddApprovedUserCalls()[0].ID)
}

func TestTelegramListener_DoWithAdminUnBan_Training(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if mc, ok := c.(tbapi.MessageConfig); ok {
				return tbapi.Message{Text: mc.Text, From: &tbapi.User{UserName: "user"}}, nil
			}
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	b := &mocks.BotMock{
		UpdateHamFunc: func(msg string) error {
			return nil
		},
		AddApprovedUserFunc: func(id int64, name string) error { return nil },
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger:   mockLogger,
		TbAPI:        mockAPI,
		Bot:          b,
		SuperUsers:   SuperUsers{"admin"},
		Group:        "gr",
		Locator:      locator,
		AdminGroup:   "123",
		TrainingMode: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		CallbackQuery: &tbapi.CallbackQuery{
			Data: "777:999",
			Message: &tbapi.Message{
				MessageID:     987654,
				Chat:          tbapi.Chat{ID: 123},
				Text:          "unban user blah\n\nthis was the ham, not spam",
				From:          &tbapi.User{UserName: "user", ID: 999},
				ForwardOrigin: &tbapi.MessageOrigin{Date: time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()},
			},
			From: &tbapi.User{UserName: "admin", ID: 1000},
		},
	}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Equal(t, 987654, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).MessageID)
	assert.Contains(t, mockAPI.SendCalls()[0].C.(tbapi.EditMessageTextConfig).Text, `разбанено администратором <a href="tg://user?id=1000">admin</a>`)
	require.Len(t, mockAPI.RequestCalls(), 1)
	assert.Equal(t, "принято", mockAPI.RequestCalls()[0].C.(tbapi.CallbackConfig).Text)
	require.Len(t, b.UpdateHamCalls(), 1)
	assert.Equal(t, "this was the ham, not spam", b.UpdateHamCalls()[0].Msg)
	require.Len(t, b.AddApprovedUserCalls(), 1)
	assert.Equal(t, int64(777), b.AddApprovedUserCalls()[0].ID)
}

func TestTelegramListener_DoWithAdminUnBanConfirmation(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			if mc, ok := c.(tbapi.MessageConfig); ok {
				return tbapi.Message{Text: mc.Text, From: &tbapi.User{UserName: "user"}}, nil
			}
			return tbapi.Message{}, nil
		},
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			return &tbapi.APIResponse{Ok: true}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) { return nil, nil },
	}
	b := &mocks.BotMock{
		UpdateHamFunc: func(msg string) error {
			return nil
		},
		AddApprovedUserFunc: func(id int64, name string) error { return nil },
	}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        b,
		SuperUsers: SuperUsers{"admin"},
		Group:      "gr",
		Locator:    locator,
		AdminGroup: "123",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		CallbackQuery: &tbapi.CallbackQuery{
			Data: "?777",
			Message: &tbapi.Message{
				MessageID:     987654,
				Chat:          tbapi.Chat{ID: 123},
				Text:          "unban user blah\n\nthis was the ham, not spam",
				From:          &tbapi.User{UserName: "user", ID: 999},
				ForwardOrigin: &tbapi.MessageOrigin{Date: time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()},
			},
			From: &tbapi.User{UserName: "admin", ID: 1000},
		},
	}
	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Equal(t, 987654, mockAPI.SendCalls()[0].C.(tbapi.EditMessageReplyMarkupConfig).MessageID)
	kb := mockAPI.SendCalls()[0].C.(tbapi.EditMessageReplyMarkupConfig).ReplyMarkup.InlineKeyboard
	assert.Len(t, kb[0], 2, " tow yes/no buttons")
	assert.Empty(t, mockAPI.RequestCalls())
	assert.Empty(t, b.UpdateHamCalls())
	require.Empty(t, b.AddApprovedUserCalls())
}
