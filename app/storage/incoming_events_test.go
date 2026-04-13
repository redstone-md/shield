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
	assert.Equal(t, event.TenantID, record.TenantID)
	assert.Equal(t, event.UpdateID, record.UpdateID)
	assert.Equal(t, event.ChatID, record.ChatID)
	assert.Equal(t, event.MessageID, record.MessageID)
	assert.Equal(t, event.EditedMessageID, record.EditedMessageID)
	assert.Equal(t, event.IdempotencyKey, record.IdempotencyKey)
	assert.Equal(t, event.ReceivedAt, record.ReceivedAt)
}
