package events

import (
	"context"
	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"reflect"
	"testing"
	"time"
)

type processorSpy struct {
	process func(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error
	calls   []processorSpyCall
}

type processorSpyCall struct {
	Event  moderation.IncomingEvent
	Update tbapi.Update
}

func (s *processorSpy) Process(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error {
	s.calls = append(s.calls, processorSpyCall{Event: event, Update: update})
	if s.process != nil {
		return s.process(ctx, event, update)
	}
	return nil
}

type policyEngineSpy struct {
	decide func(ctx context.Context, req PolicyRequest) (PolicyOutcome, error)
	calls  []PolicyRequest
}

func (s *policyEngineSpy) Decide(ctx context.Context, req PolicyRequest) (PolicyOutcome, error) {
	s.calls = append(s.calls, req)
	if s.decide != nil {
		return s.decide(ctx, req)
	}
	return PolicyOutcome{}, nil
}

type actionExecutorSpy struct {
	applyBan           func(ctx context.Context, req banRequest) error
	deleteMessage      func(ctx context.Context, chatID int64, msgID int) error
	deleteExtra        func(ctx context.Context, checkResults []spamcheck.Response, userID int64, username string, chatID int64) error
	warnUser           func(ctx context.Context, req warnRequest) error
	banCtxs            []context.Context
	banCalls           []banRequest
	warnCtxs           []context.Context
	warnCalls          []warnRequest
	deleteMessageCalls []struct {
		Context context.Context
		ChatID  int64
		MsgID   int
	}
	deleteExtraCalls []struct {
		Context      context.Context
		CheckResults []spamcheck.Response
		UserID       int64
		Username     string
		ChatID       int64
	}
}

type detectedSpamCounterSpy struct {
	count           int
	countByIDCalls  []int64
	nameCount       int
	deleteByIDCalls []struct {
		userID       int64
		signalSource string
	}
	deleteLatestByIDCalls []int64
	deleteByNameCalls     []struct {
		userName     string
		signalSource string
	}
	deleteResult bool
	writes       []storage.DetectedSpamInfo
	checks       [][]spamcheck.Response
}

func (s *detectedSpamCounterSpy) CountByUserID(_ context.Context, userID int64) (int, error) {
	s.countByIDCalls = append(s.countByIDCalls, userID)
	return s.count, nil
}

func (s *detectedSpamCounterSpy) CountByUserIDAndSignalSource(_ context.Context, _ int64, _ string) (int, error) {
	return s.count, nil
}

func (s *detectedSpamCounterSpy) DeleteLatestByUserID(_ context.Context, userID int64) (bool, error) {
	s.deleteLatestByIDCalls = append(s.deleteLatestByIDCalls, userID)
	return s.deleteResult, nil
}

func (s *detectedSpamCounterSpy) CountByUserNameAndSignalSource(_ context.Context, _ string, _ string) (int, error) {
	return s.nameCount, nil
}

func (s *detectedSpamCounterSpy) DeleteLatestByUserIDAndSignalSource(_ context.Context, userID int64, signalSource string) (bool, error) {
	s.deleteByIDCalls = append(s.deleteByIDCalls, struct {
		userID       int64
		signalSource string
	}{userID: userID, signalSource: signalSource})
	return s.deleteResult, nil
}

func (s *detectedSpamCounterSpy) DeleteLatestByUserNameAndSignalSource(_ context.Context, userName, signalSource string) (bool, error) {
	s.deleteByNameCalls = append(s.deleteByNameCalls, struct {
		userName     string
		signalSource string
	}{userName: userName, signalSource: signalSource})
	return s.deleteResult, nil
}

func (s *detectedSpamCounterSpy) Write(_ context.Context, entry storage.DetectedSpamInfo, checks []spamcheck.Response) error {
	s.writes = append(s.writes, entry)
	s.checks = append(s.checks, checks)
	return nil
}

func (s *actionExecutorSpy) ApplyBan(ctx context.Context, req banRequest) error {
	s.banCtxs = append(s.banCtxs, ctx)
	s.banCalls = append(s.banCalls, req)
	if s.applyBan != nil {
		return s.applyBan(ctx, req)
	}
	return nil
}

func (s *actionExecutorSpy) DeleteMessage(ctx context.Context, chatID int64, msgID int) error {
	s.deleteMessageCalls = append(s.deleteMessageCalls, struct {
		Context context.Context
		ChatID  int64
		MsgID   int
	}{Context: ctx, ChatID: chatID, MsgID: msgID})
	if s.deleteMessage != nil {
		return s.deleteMessage(ctx, chatID, msgID)
	}
	return nil
}

func (s *actionExecutorSpy) DeleteExtraMessages(ctx context.Context, checkResults []spamcheck.Response,
	userID int64, username string, chatID int64,
) error {
	s.deleteExtraCalls = append(s.deleteExtraCalls, struct {
		Context      context.Context
		CheckResults []spamcheck.Response
		UserID       int64
		Username     string
		ChatID       int64
	}{Context: ctx, CheckResults: checkResults, UserID: userID, Username: username, ChatID: chatID})
	if s.deleteExtra != nil {
		return s.deleteExtra(ctx, checkResults, userID, username, chatID)
	}
	return nil
}

func (s *actionExecutorSpy) WarnUser(ctx context.Context, req warnRequest) error {
	s.warnCtxs = append(s.warnCtxs, ctx)
	s.warnCalls = append(s.warnCalls, req)
	if s.warnUser != nil {
		return s.warnUser(ctx, req)
	}
	return nil
}

type auditWriterSpy struct {
	write func(ctx context.Context, record AuditRecord) error
	calls []AuditRecord
	ctxs  []context.Context
}

func (s *auditWriterSpy) Write(ctx context.Context, record AuditRecord) error {
	s.ctxs = append(s.ctxs, ctx)
	s.calls = append(s.calls, record)
	if s.write != nil {
		return s.write(ctx, record)
	}
	return nil
}

type incomingEventsSpy struct {
	record   func(ctx context.Context, event moderation.IncomingEvent) (bool, error)
	reserve  func(ctx context.Context, event moderation.IncomingEvent) (storage.IncomingEventReplay, error)
	complete func(ctx context.Context, idempotencyKey string, decision moderation.PolicyDecision,
		actionResult moderation.ModerationActionResult) error
	recordCalls   []moderation.IncomingEvent
	reserveCalls  []moderation.IncomingEvent
	completeCalls []struct {
		IdempotencyKey string
		Decision       moderation.PolicyDecision
		ActionResult   moderation.ModerationActionResult
	}
	ctxs []context.Context
}

func (s *incomingEventsSpy) Record(ctx context.Context, event moderation.IncomingEvent) (bool, error) {
	s.ctxs = append(s.ctxs, ctx)
	s.recordCalls = append(s.recordCalls, event)
	if s.record != nil {
		return s.record(ctx, event)
	}
	return true, nil
}

func (s *incomingEventsSpy) Reserve(ctx context.Context, event moderation.IncomingEvent) (storage.IncomingEventReplay, error) {
	s.ctxs = append(s.ctxs, ctx)
	s.reserveCalls = append(s.reserveCalls, event)
	if s.reserve != nil {
		return s.reserve(ctx, event)
	}
	return storage.IncomingEventReplay{Recorded: true}, nil
}

func (s *incomingEventsSpy) Complete(ctx context.Context, idempotencyKey string, decision moderation.PolicyDecision,
	actionResult moderation.ModerationActionResult,
) error {
	s.ctxs = append(s.ctxs, ctx)
	s.completeCalls = append(s.completeCalls, struct {
		IdempotencyKey string
		Decision       moderation.PolicyDecision
		ActionResult   moderation.ModerationActionResult
	}{IdempotencyKey: idempotencyKey, Decision: decision, ActionResult: actionResult})
	if s.complete != nil {
		return s.complete(ctx, idempotencyKey, decision, actionResult)
	}
	return nil
}

type contextualBotSpy struct {
	onMessage func(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response
	ctxs      []context.Context
	msgs      []bot.Message
}

func (s *contextualBotSpy) OnMessageWithContext(ctx context.Context, msg bot.Message, checkOnly bool) bot.Response {
	s.ctxs = append(s.ctxs, ctx)
	s.msgs = append(s.msgs, msg)
	if s.onMessage != nil {
		return s.onMessage(ctx, msg, checkOnly)
	}
	return bot.Response{}
}

func (s *contextualBotSpy) OnMessage(msg bot.Message, checkOnly bool) bot.Response {
	return s.OnMessageWithContext(context.Background(), msg, checkOnly)
}

func (s *contextualBotSpy) UpdateSpam(string) error { return nil }

func (s *contextualBotSpy) UpdateHam(string) error { return nil }

func (s *contextualBotSpy) AddApprovedUser(int64, string) error { return nil }

func (s *contextualBotSpy) RemoveApprovedUser(int64) error { return nil }

func (s *contextualBotSpy) IsApprovedUser(int64) bool { return false }

type locatorContextSpy struct {
	addMessageCtxs []context.Context
	addSpamCtxs    []context.Context
}

func (s *locatorContextSpy) AddMessage(ctx context.Context, msg string, chatID, userID int64, userName string, msgID int) error {
	s.addMessageCtxs = append(s.addMessageCtxs, ctx)
	return nil
}

func (s *locatorContextSpy) AddSpam(ctx context.Context, userID int64, checks []spamcheck.Response) error {
	s.addSpamCtxs = append(s.addSpamCtxs, ctx)
	return nil
}

func (s *locatorContextSpy) Message(context.Context, string) (storage.MsgMeta, bool) {
	return storage.MsgMeta{}, false
}

func (s *locatorContextSpy) Spam(context.Context, int64) (storage.SpamData, bool) {
	return storage.SpamData{}, false
}

func (s *locatorContextSpy) MsgHash(string) string { return "" }

func (s *locatorContextSpy) UserNameByID(context.Context, int64) string { return "" }

func (s *locatorContextSpy) UserIDByName(context.Context, string) int64 { return 0 }

func (s *locatorContextSpy) GetUserMessageIDs(context.Context, int64, int) ([]int, error) {
	return nil, nil
}

func TestTelegramListener_ProcEventsPublishesIncomingEvent(t *testing.T) {
	locator, teardown := prepTestLocator(t)
	defer teardown()

	spy := &processorSpy{}
	eventStore := &incomingEventsSpy{}
	callOrder := make([]string, 0, 2)
	eventStore.reserve = func(ctx context.Context, event moderation.IncomingEvent) (storage.IncomingEventReplay, error) {
		callOrder = append(callOrder, "record")
		return storage.IncomingEventReplay{Recorded: true}, nil
	}
	spy.process = func(ctx context.Context, event moderation.IncomingEvent, update tbapi.Update) error {
		callOrder = append(callOrder, "process")
		return nil
	}
	l := TelegramListener{
		Bot:            &mocks.BotMock{},
		Locator:        locator,
		IncomingEvents: eventStore,
		Group:          "123",
		TenantID:       "tg-spam",
		chatID:         123,
		Queue:          moderation.NewInMemoryQueue(1),
		processor:      spy,
	}
	defer l.shutdownPipeline()

	update := tbapi.Update{
		UpdateID: 701,
		Message: &tbapi.Message{
			MessageID: 77,
			Chat:      tbapi.Chat{ID: 123, Type: "supergroup"},
			Text:      "visit https://example.com now",
			Entities: []tbapi.MessageEntity{
				{Type: "text_link", Offset: 6, Length: 19, URL: "https://example.com"},
			},
			From: &tbapi.User{ID: 42, UserName: "user"},
			Date: int(time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC).Unix()),
		},
	}

	err := l.procEvents(update)
	require.NoError(t, err)
	require.Len(t, spy.calls, 1)
	require.Len(t, eventStore.reserveCalls, 1)

	got := spy.calls[0].Event
	assert.Equal(t, "telegram.update", got.Source)
	assert.Equal(t, "tg-spam", got.TenantID)
	assert.Equal(t, 701, got.UpdateID)
	assert.Equal(t, int64(123), got.ChatID)
	assert.Equal(t, 77, got.MessageID)
	assert.Equal(t, 0, got.EditedMessageID)
	assert.Equal(t, "telegram:update:701:chat:123:message:77:edited:0", got.IdempotencyKey)
	assert.Equal(t, int64(42), got.Subject.ID)
	assert.Equal(t, "user", got.Subject.UserName)
	assert.Equal(t, "visit https://example.com now", got.Content.Text)
	assert.True(t, reflect.DeepEqual([]string{"https://example.com"}, got.Content.Links))
	assert.False(t, got.Content.HasMedia)
	assert.Equal(t, "false", got.Content.Attributes["with_forward"])
	assert.Equal(t, update, spy.calls[0].Update)
	assert.NotEmpty(t, got.EventID)
	assert.NotEmpty(t, got.CorrelationID)
	assert.Equal(t, got, eventStore.reserveCalls[0])
	assert.Equal(t, []string{"record", "process"}, callOrder)
	assertContextMetadata(t, eventStore.ctxs[0], got.EventID, got.CorrelationID)
}

func TestTelegramListener_ProcEventsSkipsProcessedDuplicate(t *testing.T) {
	locator, teardown := prepTestLocator(t)
	defer teardown()

	spy := &processorSpy{}
	eventStore := &incomingEventsSpy{
		reserve: func(ctx context.Context, event moderation.IncomingEvent) (storage.IncomingEventReplay, error) {
			return storage.IncomingEventReplay{
				Recorded:  false,
				Processed: true,
				Decision: moderation.PolicyDecision{
					Action: moderation.ActionBan,
				},
				ActionResult: moderation.ModerationActionResult{
					Action:  moderation.ActionBan,
					Applied: true,
				},
			}, nil
		},
	}

	l := TelegramListener{
		Bot:            &mocks.BotMock{},
		Locator:        locator,
		IncomingEvents: eventStore,
		Group:          "123",
		TenantID:       "tg-spam",
		chatID:         123,
		Queue:          moderation.NewInMemoryQueue(1),
		processor:      spy,
	}
	defer l.shutdownPipeline()

	update := tbapi.Update{
		UpdateID: 703,
		Message: &tbapi.Message{
			MessageID: 79,
			Chat:      tbapi.Chat{ID: 123, Type: "supergroup"},
			Text:      "retry spam",
			From:      &tbapi.User{ID: 42, UserName: "user"},
			Date:      int(time.Date(2026, 4, 13, 10, 10, 0, 0, time.UTC).Unix()),
		},
	}

	err := l.procEvents(update)
	require.NoError(t, err)
	require.Empty(t, spy.calls)
	require.Len(t, eventStore.reserveCalls, 1)
}

func TestTelegramListener_ProcEventsBuildsEditedMessageIdempotencyKey(t *testing.T) {
	locator, teardown := prepTestLocator(t)
	defer teardown()

	spy := &processorSpy{}
	l := TelegramListener{
		Bot:       &mocks.BotMock{},
		Locator:   locator,
		Group:     "123",
		TenantID:  "tg-spam",
		chatID:    123,
		Queue:     moderation.NewInMemoryQueue(1),
		processor: spy,
	}
	defer l.shutdownPipeline()

	edited := &tbapi.Message{
		MessageID: 88,
		Chat:      tbapi.Chat{ID: 123, Type: "supergroup"},
		Text:      "edited spam",
		From:      &tbapi.User{ID: 42, UserName: "user"},
		Date:      int(time.Date(2026, 4, 13, 10, 5, 0, 0, time.UTC).Unix()),
	}

	err := l.procEvents(tbapi.Update{
		UpdateID:      702,
		Message:       edited,
		EditedMessage: edited,
	})
	require.NoError(t, err)
	require.Len(t, spy.calls, 1)

	got := spy.calls[0].Event
	assert.Equal(t, 702, got.UpdateID)
	assert.Equal(t, 88, got.MessageID)
	assert.Equal(t, 88, got.EditedMessageID)
	assert.Equal(t, "telegram:update:702:chat:123:message:88:edited:88", got.IdempotencyKey)
}
