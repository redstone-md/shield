package events

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/bot"
	"github.com/redstone-md/shield/app/events/mocks"
	"github.com/redstone-md/shield/app/moderation"
	"github.com/redstone-md/shield/lib/spamcheck"
)

type enrichedAuditLoggerSpy struct {
	calls []AuditRecord
}

func (s *enrichedAuditLoggerSpy) Save(_ *bot.Message, _ *bot.Response) {}

func (s *enrichedAuditLoggerSpy) SaveAudit(_ context.Context, record AuditRecord) error {
	s.calls = append(s.calls, record)
	return nil
}

func TestDefaultAuditWriter(t *testing.T) {
	logger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	locator := &mocks.LocatorMock{
		AddSpamFunc: func(ctx context.Context, userID int64, checks []spamcheck.Response) error { return nil },
	}
	writer := defaultAuditWriter{spamLogger: logger, locator: locator}

	record := AuditRecord{
		Event:      moderation.IncomingEvent{EventID: "evt-1"},
		Message:    &bot.Message{Text: "spam"},
		SpamUserID: 42,
		Response: bot.Response{
			Send:        true,
			BanInterval: 1,
			CheckResults: []spamcheck.Response{
				{Name: "rule", Spam: true},
			},
		},
	}

	err := writer.Write(context.Background(), record)
	require.NoError(t, err)
	assert.Len(t, logger.SaveCalls(), 1)
	assert.Len(t, locator.AddSpamCalls(), 1)
	assert.Equal(t, int64(42), locator.AddSpamCalls()[0].UserID)
}

func TestDefaultAuditWriterSkipsNonActionableResponse(t *testing.T) {
	logger := &mocks.SpamLoggerMock{SaveFunc: func(msg *bot.Message, response *bot.Response) {}}
	locator := &mocks.LocatorMock{
		AddSpamFunc: func(ctx context.Context, userID int64, checks []spamcheck.Response) error { return nil },
	}
	writer := defaultAuditWriter{spamLogger: logger, locator: locator}

	err := writer.Write(context.Background(), AuditRecord{
		Message:  &bot.Message{Text: "ham"},
		Response: bot.Response{Send: false},
	})
	require.NoError(t, err)
	assert.Empty(t, logger.SaveCalls())
	assert.Empty(t, locator.AddSpamCalls())
}

func TestDefaultAuditWriterUsesEnrichedLoggerWhenAvailable(t *testing.T) {
	logger := &enrichedAuditLoggerSpy{}
	locator := &mocks.LocatorMock{
		AddSpamFunc: func(ctx context.Context, userID int64, checks []spamcheck.Response) error { return nil },
	}
	writer := defaultAuditWriter{spamLogger: logger, locator: locator}

	record := AuditRecord{
		Event:          moderation.IncomingEvent{EventID: "evt-1"},
		Message:        &bot.Message{Text: "spam"},
		SpamUserID:     42,
		RuleSetVersion: 7,
		Decision:       moderation.PolicyDecision{Score: 3},
		Response: bot.Response{
			Send:        true,
			BanInterval: 1,
			CheckResults: []spamcheck.Response{
				{Name: "duplicates", Spam: true},
				{Name: "openai", Spam: true},
			},
		},
	}

	err := writer.Write(context.Background(), record)
	require.NoError(t, err)
	require.Len(t, logger.calls, 1)
	assert.Equal(t, 7, logger.calls[0].RuleSetVersion)
	assert.Len(t, locator.AddSpamCalls(), 1)
}
