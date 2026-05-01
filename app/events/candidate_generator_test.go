package events

import (
	"context"
	"sync"
	"testing"
	"time"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

func TestTelegramListener_CandidateGenerationOnSpam(t *testing.T) {
	gen := &candidateGeneratorSpy{}

	botMock := &contextualBotSpy{
		onMessage: func(_ context.Context, _ bot.Message, _ bool) bot.Response {
			return bot.Response{
				Send:        true,
				BanInterval: time.Minute,
				User:        bot.User{ID: 42, Username: "spammer"},
				CheckResults: []spamcheck.Response{
					{Name: "rule", Spam: true, Details: "detected"},
				},
			}
		},
	}

	l := TelegramListener{
		Bot:                botMock,
		Locator:            &locatorContextSpy{},
		NoSpamReply:        true,
		PolicyEngine:       defaultPolicyEngine{},
		ActionExecutor:     &actionExecutorSpy{},
		AuditWriter:        &auditWriterSpy{},
		IncomingEvents:     &incomingEventsSpy{},
		CandidateGenerator: gen,
	}

	event := moderation.IncomingEvent{
		EventID:        "evt-cg-1",
		IdempotencyKey: "telegram:update:1:chat:99:message:77:edited:0",
	}
	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 77,
			Chat:      tbapi.Chat{ID: 99},
			Text:      "buy crypto now cheap price",
			From:      &tbapi.User{ID: 42, UserName: "spammer"},
			Date:      int(time.Now().Unix()),
		},
	}

	err := l.processQueuedEvent(context.Background(), event, update)
	require.NoError(t, err)
	require.Len(t, gen.calls, 1)
	assert.Equal(t, "buy crypto now cheap price", gen.calls[0])
}

func TestTelegramListener_CandidateGenerationNil(t *testing.T) {
	botMock := &contextualBotSpy{
		onMessage: func(_ context.Context, _ bot.Message, _ bool) bot.Response {
			return bot.Response{
				Send:        true,
				BanInterval: time.Minute,
				User:        bot.User{ID: 42, Username: "spammer"},
				CheckResults: []spamcheck.Response{
					{Name: "rule", Spam: true},
				},
			}
		},
	}

	l := TelegramListener{
		Bot:            botMock,
		Locator:        &locatorContextSpy{},
		NoSpamReply:    true,
		PolicyEngine:   defaultPolicyEngine{},
		ActionExecutor: &actionExecutorSpy{},
		AuditWriter:    &auditWriterSpy{},
		IncomingEvents: &incomingEventsSpy{},
	}

	event := moderation.IncomingEvent{
		EventID:        "evt-cg-nil",
		IdempotencyKey: "telegram:update:1:chat:99:message:78:edited:0",
	}
	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 78,
			Chat:      tbapi.Chat{ID: 99},
			Text:      "spam text",
			From:      &tbapi.User{ID: 42, UserName: "spammer"},
			Date:      int(time.Now().Unix()),
		},
	}

	err := l.processQueuedEvent(context.Background(), event, update)
	require.NoError(t, err)
}

func TestTelegramListener_CandidateGenerationSkippedOnNonSpam(t *testing.T) {
	gen := &candidateGeneratorSpy{}

	botMock := &contextualBotSpy{
		onMessage: func(_ context.Context, _ bot.Message, _ bool) bot.Response {
			return bot.Response{Send: false}
		},
	}

	l := TelegramListener{
		Bot:                botMock,
		Locator:            &locatorContextSpy{},
		NoSpamReply:        true,
		PolicyEngine:       defaultPolicyEngine{},
		ActionExecutor:     &actionExecutorSpy{},
		AuditWriter:        &auditWriterSpy{},
		IncomingEvents:     &incomingEventsSpy{},
		CandidateGenerator: gen,
	}

	event := moderation.IncomingEvent{
		EventID:        "evt-cg-nospam",
		IdempotencyKey: "telegram:update:1:chat:99:message:79:edited:0",
	}
	update := tbapi.Update{
		Message: &tbapi.Message{
			MessageID: 79,
			Chat:      tbapi.Chat{ID: 99},
			Text:      "normal message",
			From:      &tbapi.User{ID: 42, UserName: "user"},
			Date:      int(time.Now().Unix()),
		},
	}

	err := l.processQueuedEvent(context.Background(), event, update)
	require.NoError(t, err)
	assert.Empty(t, gen.calls)
}

type candidateGeneratorSpy struct {
	mu    sync.Mutex
	calls []string
}

func (s *candidateGeneratorSpy) GenerateCandidates(_ context.Context, text string) {
	s.mu.Lock()
	s.calls = append(s.calls, text)
	s.mu.Unlock()
}
