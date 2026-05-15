package events

import (
	"bytes"
	"context"
	"fmt"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"log"
	"testing"
	"time"
)

func TestTelegramListener_TracerBulletSmoke(t *testing.T) {
	callOrder := make([]string, 0, 4)

	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
	}

	locator := &locatorContextSpy{}

	botMock := &contextualBotSpy{
		onMessage: func(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response {
			callOrder = append(callOrder, "detection")
			return bot.Response{
				Send:        true,
				BanInterval: time.Minute,
				User:        bot.User{ID: 42, Username: "user"},
				CheckResults: []spamcheck.Response{
					{Name: "rule", Spam: true, Details: "smoke spam"},
				},
			}
		},
	}

	policySpy := &policyEngineSpy{
		decide: func(ctx context.Context, req PolicyRequest) (PolicyOutcome, error) {
			callOrder = append(callOrder, "policy")
			return PolicyOutcome{
				Decision: moderation.PolicyDecision{
					EventID:       req.Event.EventID,
					CorrelationID: req.Event.CorrelationID,
					Action:        moderation.ActionBan,
					Reason:        "smoke policy",
					Score:         1,
					DecidedAt:     time.Now().UTC(),
				},
				Duration: time.Minute,
			}, nil
		},
	}

	actionSpy := &actionExecutorSpy{
		applyBan: func(ctx context.Context, req banRequest) error {
			callOrder = append(callOrder, "action")
			return nil
		},
	}

	auditSpy := &auditWriterSpy{
		write: func(ctx context.Context, record AuditRecord) error {
			callOrder = append(callOrder, "audit")
			return locator.AddSpam(ctx, record.SpamUserID, record.Response.CheckResults)
		},
	}

	l := TelegramListener{
		TbAPI:          mockAPI,
		Bot:            botMock,
		Group:          "gr",
		Locator:        locator,
		NoSpamReply:    true,
		PolicyEngine:   policySpy,
		ActionExecutor: actionSpy,
		AuditWriter:    auditSpy,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	updChan := make(chan tbapi.Update, 1)
	updChan <- tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 55,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "spam payload",
			From:      &tbapi.User{ID: 42, UserName: "user"},
			Date:      int(time.Now().Unix()),
		},
	}
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	assert.Equal(t, []string{"detection", "policy", "action", "audit"}, callOrder)
	require.Len(t, policySpy.calls, 1)
	require.Len(t, actionSpy.banCalls, 1)
	require.Len(t, auditSpy.calls, 1)
	assert.Equal(t, moderation.ActionBan, auditSpy.calls[0].Decision.Action)
	assert.Equal(t, "spam payload", auditSpy.calls[0].Message.Text)
	assert.Equal(t, int64(42), auditSpy.calls[0].SpamUserID)

	eventID := policySpy.calls[0].Event.EventID
	correlationID := policySpy.calls[0].Event.CorrelationID
	require.NotEmpty(t, eventID)
	require.NotEmpty(t, correlationID)
	assertContextMetadata(t, botMock.ctxs[0], eventID, correlationID)
	assertContextMetadata(t, actionSpy.banCtxs[0], eventID, correlationID)
	assertContextMetadata(t, actionSpy.deleteExtraCalls[0].Context, eventID, correlationID)
	assertContextMetadata(t, auditSpy.ctxs[0], eventID, correlationID)
	assertContextMetadata(t, locator.addMessageCtxs[0], eventID, correlationID)
	assertContextMetadata(t, locator.addSpamCtxs[0], eventID, correlationID)
}

func TestTelegramListener_AutomaticWarnCarriesMessageIDAndDeleteDuration(t *testing.T) {
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return nil, nil
		},
	}

	botMock := &contextualBotSpy{
		onMessage: func(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{
				Send: true,
				User: bot.User{ID: 42, Username: "baz_02l_wss", DisplayName: "Asya Kilisa",
					FirstName: "Asya", LastName: "Kilisa"},
				CheckResults: []spamcheck.Response{{Name: "slowpath", Spam: true, Details: "vision spam reason"}},
			}
		},
	}

	policySpy := &policyEngineSpy{
		decide: func(ctx context.Context, req PolicyRequest) (PolicyOutcome, error) {
			return PolicyOutcome{
				Decision: moderation.PolicyDecision{
					EventID:       req.Event.EventID,
					CorrelationID: req.Event.CorrelationID,
					Action:        moderation.ActionWarn,
					Reason:        "warn before mute",
					Score:         1,
					DecidedAt:     time.Now().UTC(),
				},
			}, nil
		},
	}

	actionSpy := &actionExecutorSpy{}
	l := TelegramListener{
		TbAPI:          mockAPI,
		Bot:            botMock,
		Group:          "gr",
		Locator:        &locatorContextSpy{},
		NoSpamReply:    true,
		PolicyEngine:   policySpy,
		ActionExecutor: actionSpy,
		ModerationConfig: ModerationConfig{
			WarnStrikes:        3,
			WarnDeleteDuration: time.Minute,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	updChan := make(chan tbapi.Update, 1)
	updChan <- tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 55,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "spam payload",
			From: &tbapi.User{ID: 42, UserName: "baz_02l_wss",
				FirstName: "Asya", LastName: "Kilisa"},
			Date: int(time.Now().Unix()),
		},
	}
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")

	require.Len(t, actionSpy.warnCalls, 1)
	assert.Equal(t, int64(123), actionSpy.warnCalls[0].chatID)
	assert.Equal(t, int64(42), actionSpy.warnCalls[0].subjectID)
	assert.Equal(t, 55, actionSpy.warnCalls[0].messageID)
	assert.Equal(t, time.Minute, actionSpy.warnCalls[0].warnDelTime)
	assert.Contains(t, actionSpy.warnCalls[0].text, `<a href="https://t.me/baz_02l_wss">Asya Kilisa</a>`)
	assert.Contains(t, actionSpy.warnCalls[0].text, `Причина: vision spam reason`)
}

func TestTelegramListener_ProcessQueuedEventLogsCorrelationIDs(t *testing.T) {
	var buf bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	}()

	locator := &locatorContextSpy{}
	botMock := &contextualBotSpy{
		onMessage: func(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{}
		},
	}

	l := TelegramListener{
		Bot:            botMock,
		Locator:        locator,
		PolicyEngine:   &policyEngineSpy{},
		ActionExecutor: &actionExecutorSpy{},
		AuditWriter:    &auditWriterSpy{},
	}

	event := moderation.IncomingEvent{
		EventID:       "evt-123",
		CorrelationID: "corr-123",
	}
	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 55,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "hello",
			From:      &tbapi.User{ID: 42, UserName: "user"},
			Date:      int(time.Now().Unix()),
		},
	}

	err := l.processQueuedEvent(context.Background(), event, update)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "evt=evt-123 corr=corr-123")
}

func TestTelegramListener_ProcessQueuedEventCompletesReplayState(t *testing.T) {
	locator := &locatorContextSpy{}
	botMock := &contextualBotSpy{
		onMessage: func(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{
				Send:        true,
				BanInterval: time.Minute,
				User:        bot.User{ID: 42, Username: "user"},
				CheckResults: []spamcheck.Response{
					{Name: "rule", Spam: true, Details: "smoke spam"},
				},
			}
		},
	}

	store := &incomingEventsSpy{}
	l := TelegramListener{
		Bot:            botMock,
		Locator:        locator,
		IncomingEvents: store,
		NoSpamReply:    true,
		PolicyEngine:   defaultPolicyEngine{},
		ActionExecutor: &actionExecutorSpy{},
		AuditWriter:    &auditWriterSpy{},
	}

	event := moderation.IncomingEvent{
		EventID:        "evt-123",
		CorrelationID:  "corr-123",
		IdempotencyKey: "telegram:update:1:chat:123:message:55:edited:0",
	}
	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 55,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "hello",
			From:      &tbapi.User{ID: 42, UserName: "user"},
			Date:      int(time.Now().Unix()),
		},
	}

	err := l.processQueuedEvent(context.Background(), event, update)
	require.NoError(t, err)
	require.Len(t, store.completeCalls, 1)
	assert.Equal(t, event.IdempotencyKey, store.completeCalls[0].IdempotencyKey)
	assert.Equal(t, moderation.ActionBan, store.completeCalls[0].Decision.Action)
	assert.True(t, store.completeCalls[0].ActionResult.Applied)
}

func TestTelegramListener_ProcessQueuedEventDeletesMessageForDeletePolicy(t *testing.T) {
	locator := &locatorContextSpy{}
	botMock := &contextualBotSpy{
		onMessage: func(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{
				Send: true,
				User: bot.User{ID: 42, Username: "user"},
				CheckResults: []spamcheck.Response{
					{Name: "slowpath", Spam: true, Details: "vision spam"},
				},
			}
		},
	}

	actionSpy := &actionExecutorSpy{}
	store := &incomingEventsSpy{}
	l := TelegramListener{
		Bot:            botMock,
		Locator:        locator,
		IncomingEvents: store,
		NoSpamReply:    true,
		PolicyEngine:   defaultPolicyEngine{},
		ActionExecutor: actionSpy,
		AuditWriter:    &auditWriterSpy{},
	}

	event := moderation.IncomingEvent{
		EventID:        "evt-delete",
		CorrelationID:  "corr-delete",
		IdempotencyKey: "telegram:update:2:chat:123:message:226:edited:0",
	}
	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 226,
			Chat:      tbapi.Chat{ID: 123},
			Text:      "",
			From:      &tbapi.User{ID: 42, UserName: "user"},
			Date:      int(time.Now().Unix()),
		},
	}

	err := l.processQueuedEvent(context.Background(), event, update)
	require.NoError(t, err)
	require.Len(t, actionSpy.deleteMessageCalls, 1)
	assert.Equal(t, int64(123), actionSpy.deleteMessageCalls[0].ChatID)
	assert.Equal(t, 226, actionSpy.deleteMessageCalls[0].MsgID)
	require.Len(t, store.completeCalls, 1)
	assert.Equal(t, moderation.ActionDelete, store.completeCalls[0].Decision.Action)
	assert.True(t, store.completeCalls[0].ActionResult.Applied)
}

func TestTelegramListener_DuplicateRetryDoesNotRepeatSuccessfulActionOrAudit(t *testing.T) {
	locator, teardown := prepTestLocator(t)
	defer teardown()

	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewIncomingEvents(context.Background(), db)
	require.NoError(t, err)

	botMock := &contextualBotSpy{
		onMessage: func(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{
				Send:        true,
				BanInterval: time.Minute,
				User:        bot.User{ID: 42, Username: "user"},
				CheckResults: []spamcheck.Response{
					{Name: "rule", Spam: true, Details: "smoke spam"},
				},
			}
		},
	}
	actionSpy := &actionExecutorSpy{}
	auditSpy := &auditWriterSpy{}

	l := TelegramListener{
		Bot:            botMock,
		Locator:        locator,
		IncomingEvents: store,
		NoSpamReply:    true,
		PolicyEngine:   defaultPolicyEngine{},
		ActionExecutor: actionSpy,
		AuditWriter:    auditSpy,
		Group:          "123",
		TenantID:       "tg-spam",
		chatID:         123,
	}

	update := tbapi.Update{
		UpdateID: 704,
		Message: &tbapi.Message{
			MessageID: 80,
			Chat:      tbapi.Chat{ID: 123, Type: "supergroup"},
			Text:      "retry spam",
			From:      &tbapi.User{ID: 42, UserName: "user"},
			Date:      int(time.Date(2026, 4, 13, 10, 15, 0, 0, time.UTC).Unix()),
		},
	}

	err = l.procEvents(update)
	require.NoError(t, err)
	err = l.procEvents(update)
	require.NoError(t, err)

	require.Len(t, actionSpy.banCalls, 1)
	require.Len(t, auditSpy.calls, 1)
}

func TestTelegramListener_DuplicateRetryRecoversAfterTelegramActionFailure(t *testing.T) {
	locator, teardown := prepTestLocator(t)
	defer teardown()

	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewIncomingEvents(context.Background(), db)
	require.NoError(t, err)

	botMock := &contextualBotSpy{
		onMessage: func(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response {
			return bot.Response{
				Send:        true,
				BanInterval: time.Minute,
				User:        bot.User{ID: 42, Username: "user"},
				CheckResults: []spamcheck.Response{
					{Name: "rule", Spam: true, Details: "smoke spam"},
				},
			}
		},
	}
	attempts := 0
	actionSpy := &actionExecutorSpy{
		applyBan: func(ctx context.Context, req banRequest) error {
			attempts++
			if attempts == 1 {
				return fmt.Errorf("telegram timeout")
			}
			return nil
		},
	}
	auditSpy := &auditWriterSpy{}

	l := TelegramListener{
		Bot:            botMock,
		Locator:        locator,
		IncomingEvents: store,
		NoSpamReply:    true,
		PolicyEngine:   defaultPolicyEngine{},
		ActionExecutor: actionSpy,
		AuditWriter:    auditSpy,
		Group:          "123",
		TenantID:       "tg-spam",
		chatID:         123,
	}

	update := tbapi.Update{
		UpdateID: 705,
		Message: &tbapi.Message{
			MessageID: 81,
			Chat:      tbapi.Chat{ID: 123, Type: "supergroup"},
			Text:      "retry spam",
			From:      &tbapi.User{ID: 42, UserName: "user"},
			Date:      int(time.Date(2026, 4, 13, 10, 20, 0, 0, time.UTC).Unix()),
		},
	}

	err = l.procEvents(update)
	require.Error(t, err)
	err = l.procEvents(update)
	require.NoError(t, err)

	require.Len(t, actionSpy.banCalls, 2)
	require.Len(t, auditSpy.calls, 1)

	record, err := store.ByIdempotencyKey(context.Background(), "telegram:update:705:chat:123:message:81:edited:0")
	require.NoError(t, err)
	assert.True(t, record.ProcessedAt.Valid)
	assert.True(t, record.ActionApplied.Valid)
	assert.True(t, record.ActionApplied.Bool)
}

func assertContextMetadata(t *testing.T, ctx context.Context, eventID, correlationID string) { //nolint:revive // t *testing.T must come first in test helpers
	t.Helper()
	meta, ok := observability.MetadataFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, eventID, meta.EventID)
	assert.Equal(t, correlationID, meta.CorrelationID)
}

func TestTelegramListener_Do(t *testing.T) {
	mockLogger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	mockAPI := &mocks.TbAPIMock{
		GetChatFunc: func(config tbapi.ChatInfoConfig) (tbapi.ChatFullInfo, error) {
			return tbapi.ChatFullInfo{Chat: tbapi.Chat{ID: 123}}, nil
		},
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			return tbapi.Message{Text: c.(tbapi.MessageConfig).Text, From: &tbapi.User{UserName: "user"}}, nil
		},
		GetChatAdministratorsFunc: func(config tbapi.ChatAdministratorsConfig) ([]tbapi.ChatMember, error) {
			return []tbapi.ChatMember{
				{
					User: &tbapi.User{
						UserName: "admin",
						ID:       1,
					},
					Status: "administrator",
				},
			}, nil
		},
	}
	botMock := &mocks.BotMock{OnMessageFunc: func(msg bot.Message, checkOnly bool) bot.Response {
		t.Logf("on-message: %+v", msg)
		return bot.Response{}
	}}

	locator, teardown := prepTestLocator(t)
	defer teardown()

	l := TelegramListener{
		SpamLogger: mockLogger,
		TbAPI:      mockAPI,
		Bot:        botMock,
		Group:      "gr",
		AdminGroup: "987654321",
		StartupMsg: "startup",
		Locator:    locator,
		SuperUsers: SuperUsers{"super"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Minute)
	defer cancel()

	updMsg := tbapi.Update{
		Message: &tbapi.Message{
			Chat: tbapi.Chat{ID: 123},
			Text: "text 123",
			From: &tbapi.User{UserName: "user"},
			Date: int(time.Date(2020, 2, 11, 19, 35, 55, 9, time.UTC).Unix()),
		},
	}

	updChan := make(chan tbapi.Update, 1)
	updChan <- updMsg
	close(updChan)
	mockAPI.GetUpdatesChanFunc = func(config tbapi.UpdateConfig) tbapi.UpdatesChannel { return updChan }

	err := l.Do(ctx)
	require.EqualError(t, err, "telegram update chan closed")
	assert.Equal(t, SuperUsers{"super", "1"}, l.SuperUsers)

	assert.Empty(t, mockLogger.SaveCalls())
	require.Len(t, mockAPI.SendCalls(), 1)
	assert.Equal(t, "startup", mockAPI.SendCalls()[0].C.(tbapi.MessageConfig).Text)
	assert.Len(t, mockAPI.GetChatAdministratorsCalls(), 1)

	require.Len(t, botMock.OnMessageCalls(), 1)
	assert.Equal(t, "text 123", botMock.OnMessageCalls()[0].Msg.Text)
	assert.Equal(t, "user", botMock.OnMessageCalls()[0].Msg.From.Username)
	assert.False(t, botMock.OnMessageCalls()[0].CheckOnly)

}
