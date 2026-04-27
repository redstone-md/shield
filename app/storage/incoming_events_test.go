package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/storage/engine"
)

func TestIncomingEventsRecord(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewIncomingEvents(context.Background(), db)
	require.NoError(t, err)

	event := moderation.IncomingEvent{
		EventID:         "evt-1",
		CorrelationID:   "corr-1",
		TenantID:        "tg-spam",
		Source:          "telegram.update",
		UpdateID:        701,
		ChatID:          123,
		MessageID:       77,
		EditedMessageID: 0,
		IdempotencyKey:  "telegram:update:701:chat:123:message:77:edited:0",
		ReceivedAt:      time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC),
	}

	created, err := store.Record(context.Background(), event)
	require.NoError(t, err)
	assert.True(t, created)

	created, err = store.Record(context.Background(), event)
	require.NoError(t, err)
	assert.False(t, created)

	record, err := store.ByIdempotencyKey(context.Background(), event.IdempotencyKey)
	require.NoError(t, err)
	assert.Equal(t, "gr1", record.GID)
	assert.Equal(t, event.EventID, record.EventID)
	assert.Equal(t, event.CorrelationID, record.CorrelationID)
	assert.Equal(t, "gr1", record.TenantID)
	assert.Equal(t, event.UpdateID, record.UpdateID)
	assert.Equal(t, event.ChatID, record.ChatID)
	assert.Equal(t, event.MessageID, record.MessageID)
	assert.Equal(t, event.EditedMessageID, record.EditedMessageID)
	assert.Equal(t, event.IdempotencyKey, record.IdempotencyKey)
	assert.Equal(t, event.ReceivedAt, record.ReceivedAt)
}

func TestIncomingEventsReserveAndComplete(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewIncomingEvents(context.Background(), db)
	require.NoError(t, err)

	event := moderation.IncomingEvent{
		EventID:         "evt-2",
		CorrelationID:   "corr-2",
		TenantID:        "tg-spam",
		Source:          "telegram.update",
		UpdateID:        702,
		ChatID:          123,
		MessageID:       78,
		EditedMessageID: 0,
		IdempotencyKey:  "telegram:update:702:chat:123:message:78:edited:0",
		ReceivedAt:      time.Date(2026, 4, 13, 11, 5, 0, 0, time.UTC),
	}

	replay, err := store.Reserve(context.Background(), event)
	require.NoError(t, err)
	assert.True(t, replay.Recorded)
	assert.False(t, replay.Processed)

	err = store.Complete(context.Background(), event.IdempotencyKey,
		moderation.PolicyDecision{
			EventID:       event.EventID,
			CorrelationID: event.CorrelationID,
			Action:        moderation.ActionBan,
			Reason:        "smoke policy",
			Score:         1,
			DecidedAt:     time.Date(2026, 4, 13, 11, 6, 0, 0, time.UTC),
		},
		moderation.ModerationActionResult{
			EventID:       event.EventID,
			CorrelationID: event.CorrelationID,
			Action:        moderation.ActionBan,
			Applied:       true,
			Provider:      "telegram",
			AppliedAt:     time.Date(2026, 4, 13, 11, 6, 0, 0, time.UTC),
		},
	)
	require.NoError(t, err)

	replay, err = store.Reserve(context.Background(), event)
	require.NoError(t, err)
	assert.False(t, replay.Recorded)
	assert.True(t, replay.Processed)
	assert.Equal(t, moderation.ActionBan, replay.Decision.Action)
	assert.Equal(t, "smoke policy", replay.Decision.Reason)
	assert.True(t, replay.ActionResult.Applied)

	record, err := store.ByIdempotencyKey(context.Background(), event.IdempotencyKey)
	require.NoError(t, err)
	assert.True(t, record.ProcessedAt.Valid)
	assert.Equal(t, "ban", record.DecisionAction)
	assert.Equal(t, "smoke policy", record.DecisionReason)
	assert.True(t, record.ActionApplied.Valid)
	assert.True(t, record.ActionApplied.Bool)
}

func TestIncomingEventsReserveAllowsRetryAfterFailedAction(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewIncomingEvents(context.Background(), db)
	require.NoError(t, err)

	event := moderation.IncomingEvent{
		EventID:        "evt-3",
		CorrelationID:  "corr-3",
		TenantID:       "tg-spam",
		Source:         "telegram.update",
		UpdateID:       703,
		ChatID:         123,
		MessageID:      79,
		IdempotencyKey: "telegram:update:703:chat:123:message:79:edited:0",
		ReceivedAt:     time.Date(2026, 4, 13, 11, 10, 0, 0, time.UTC),
	}

	replay, err := store.Reserve(context.Background(), event)
	require.NoError(t, err)
	assert.True(t, replay.Recorded)
	assert.False(t, replay.Processed)

	err = store.Complete(context.Background(), event.IdempotencyKey,
		moderation.PolicyDecision{
			EventID:       event.EventID,
			CorrelationID: event.CorrelationID,
			Action:        moderation.ActionBan,
			Reason:        "telegram failure",
			Score:         1,
			DecidedAt:     time.Date(2026, 4, 13, 11, 11, 0, 0, time.UTC),
		},
		moderation.ModerationActionResult{
			EventID:       event.EventID,
			CorrelationID: event.CorrelationID,
			Action:        moderation.ActionBan,
			Applied:       false,
			Provider:      "telegram",
			Error:         "telegram timeout",
			AppliedAt:     time.Date(2026, 4, 13, 11, 11, 0, 0, time.UTC),
		},
	)
	require.NoError(t, err)

	replay, err = store.Reserve(context.Background(), event)
	require.NoError(t, err)
	assert.True(t, replay.Recorded)
	assert.False(t, replay.Processed)

	record, err := store.ByIdempotencyKey(context.Background(), event.IdempotencyKey)
	require.NoError(t, err)
	assert.False(t, record.ProcessedAt.Valid)
	assert.Equal(t, "ban", record.DecisionAction)
	assert.Equal(t, "telegram timeout", record.ActionError)
}
