package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/storage/engine"
)

func TestModerationActionsAdd(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewModerationActions(context.Background(), db)
	require.NoError(t, err)

	err = store.Add(context.Background(), ModerationActionEntry{
		EventID:        "evt-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-1",
		Command:        "ban_user",
		Status:         "completed",
		ChatID:         123,
		SubjectID:      42,
		Attempt:        1,
		CreatedAt:      time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	err = store.Add(context.Background(), ModerationActionEntry{
		EventID:        "evt-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-1",
		Command:        "delete_message",
		Status:         "failed",
		ChatID:         123,
		SubjectID:      42,
		MessageID:      77,
		Attempt:        2,
		LastError:      "message not found",
		CreatedAt:      time.Date(2026, 4, 22, 12, 1, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	entries, err := store.ByEventID(context.Background(), "evt-1")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "gr1", entries[0].GID)
	assert.Equal(t, "ban_user", entries[0].Command)
	assert.Equal(t, "completed", entries[0].Status)
	assert.Equal(t, "delete_message", entries[1].Command)
	assert.Equal(t, "failed", entries[1].Status)
	assert.Equal(t, "message not found", entries[1].LastError)
}

func TestModerationActionsLast(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewModerationActions(context.Background(), db)
	require.NoError(t, err)

	err = store.Add(context.Background(), ModerationActionEntry{
		EventID:        "evt-1",
		CorrelationID:  "corr-1",
		IdempotencyKey: "key-1",
		Command:        "ban_user",
		Status:         "failed",
		ChatID:         123,
		SubjectID:      42,
		Attempt:        1,
		LastError:      "telegram timeout",
		CreatedAt:      time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	err = store.Add(context.Background(), ModerationActionEntry{
		EventID:        "evt-2",
		CorrelationID:  "corr-2",
		IdempotencyKey: "key-1",
		Command:        "ban_user",
		Status:         "completed",
		ChatID:         123,
		SubjectID:      42,
		Attempt:        2,
		CreatedAt:      time.Date(2026, 4, 22, 12, 1, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	replay, err := store.Last(context.Background(), ModerationActionLookup{
		IdempotencyKey: "key-1",
		Command:        "ban_user",
		ChatID:         123,
		SubjectID:      42,
	})
	require.NoError(t, err)
	assert.True(t, replay.Found)
	assert.True(t, replay.Completed)
	assert.Equal(t, 2, replay.Attempt)
	assert.Empty(t, replay.LastError)
}

func TestModerationActionsMigrateFromOldSchema(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS moderation_actions (" +
		"id INTEGER PRIMARY KEY AUTOINCREMENT, " +
		"event_id TEXT NOT NULL, " +
		"correlation_id TEXT NOT NULL DEFAULT '', " +
		"idempotency_key TEXT NOT NULL DEFAULT '', " +
		"command TEXT NOT NULL, " +
		"status TEXT NOT NULL, " +
		"chat_id INTEGER NOT NULL DEFAULT 0, " +
		"subject_id INTEGER NOT NULL DEFAULT 0, " +
		"message_id INTEGER NOT NULL DEFAULT 0, " +
		"attempt INTEGER NOT NULL DEFAULT 1, " +
		"last_error TEXT DEFAULT '', " +
		"created_at DATETIME DEFAULT CURRENT_TIMESTAMP)")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO moderation_actions
		(event_id, correlation_id, idempotency_key, command, status, chat_id, subject_id, attempt, created_at)
		VALUES ('evt-old', 'corr-old', 'key-old', 'ban_user', 'completed', 99, 88, 1, '2026-01-01T00:00:00Z')`)
	require.NoError(t, err)

	store, err := NewModerationActions(context.Background(), db)
	require.NoError(t, err)

	entries, err := store.ByEventID(context.Background(), "evt-old")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "gr1", entries[0].GID)
	assert.Equal(t, "evt-old", entries[0].EventID)
	assert.Equal(t, "ban_user", entries[0].Command)
}
