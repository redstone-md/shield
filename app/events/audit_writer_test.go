package events

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/events/mocks"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

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
