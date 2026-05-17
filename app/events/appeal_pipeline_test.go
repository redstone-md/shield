package events

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/slowpath"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

type incidentCreatorStub struct {
	id    int64
	calls int
}

func (s *incidentCreatorStub) CreateIncident(_ context.Context, _ string, _ int64, _ int,
	_ int64, _ string, _ string, _ []spamcheck.Response, _ *slowpath.SlowPathInvocation) (int64, error) {
	s.calls++
	return s.id, nil
}

func TestProcessWarn_PassesIncidentIDToWarnRequest(t *testing.T) {
	spy := &actionExecutorSpy{}
	creator := &incidentCreatorStub{id: 77}
	l := &TelegramListener{
		ActionExecutor:  spy,
		IncidentCreator: creator,
		BotUsername:     "shield_bot",
		AuditWriter:     &auditWriterSpy{},
	}

	pc := pipelineContext{
		event:    moderation.IncomingEvent{IdempotencyKey: "k1"},
		msg:      &bot.Message{ID: 5, From: bot.User{ID: 200}},
		resp:     bot.Response{Send: true, BanInterval: time.Hour, CheckResults: []spamcheck.Response{{Spam: true}}},
		fromChat: 100,
	}

	err := l.processWarn(context.Background(), pc)
	require.NoError(t, err)
	require.NotEmpty(t, spy.warnCalls)
	assert.Equal(t, int64(77), spy.warnCalls[0].incidentID)
	assert.Equal(t, "shield_bot", spy.warnCalls[0].botUsername)
	assert.Equal(t, 1, creator.calls)
}
