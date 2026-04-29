package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/storage/engine"
)

func TestRetentionService_CleanNow(t *testing.T) {
	db := newRetentionDB(t)
	ctx := t.Context()

	svc := NewRetentionService(db, RetentionConfig{
		IncidentsTTL:       48 * time.Hour,
		IncomingEventsTTL:  24 * time.Hour,
		UsageCountersTTL:   1 * time.Hour,
	})

	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS incidents (
		id INTEGER PRIMARY KEY AUTOINCREMENT, gid TEXT DEFAULT '', tenant_id TEXT DEFAULT '',
		source TEXT DEFAULT '', status TEXT DEFAULT 'open', severity TEXT DEFAULT 'low',
		reason_code TEXT DEFAULT '', spam_user_id INTEGER DEFAULT 0, spam_user_name TEXT DEFAULT '',
		message_text TEXT DEFAULT '', chat_id INTEGER DEFAULT 0, rule_set_version INTEGER DEFAULT 0,
		created_at DATETIME, updated_at DATETIME)`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS incoming_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT, gid TEXT DEFAULT '', tenant_id TEXT DEFAULT '',
		event_type TEXT DEFAULT '', event_id TEXT DEFAULT '', chat_id INTEGER DEFAULT 0,
		user_id INTEGER DEFAULT 0, correlation_id TEXT DEFAULT '', idempotency_key TEXT DEFAULT '',
		status TEXT DEFAULT '', timestamp DATETIME)`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS usage_counters (
		id INTEGER PRIMARY KEY AUTOINCREMENT, gid TEXT DEFAULT '', tenant_id TEXT DEFAULT '',
		meter_type TEXT DEFAULT '', count INTEGER DEFAULT 0,
		window_start DATETIME, window_end DATETIME)`)
	require.NoError(t, err)

	now := time.Now().UTC()

	_, err = db.ExecContext(ctx, `INSERT INTO incidents (tenant_id, created_at, updated_at) VALUES (?, ?, ?)`,
		"test", now.Add(-72*time.Hour), now.Add(-72*time.Hour))
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO incidents (tenant_id, created_at, updated_at) VALUES (?, ?, ?)`,
		"test", now, now)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO incoming_events (tenant_id, timestamp) VALUES (?, ?)`,
		"test", now.Add(-48*time.Hour))
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO incoming_events (tenant_id, timestamp) VALUES (?, ?)`,
		"test", now)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO usage_counters (tenant_id, meter_type, window_start, window_end) VALUES (?, ?, ?, ?)`,
		"test", "api_calls", now.Add(-2*time.Hour), now.Add(-1*time.Hour))
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO usage_counters (tenant_id, meter_type, window_start, window_end) VALUES (?, ?, ?, ?)`,
		"test", "api_calls", now, now.Add(time.Hour))
	require.NoError(t, err)

	report, err := svc.CleanNow(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1, report["incidents"])
	assert.Equal(t, 1, report["incoming_events"])
	assert.Equal(t, 1, report["usage_counters"])
}

func TestRetentionService_ZeroTTL_NoDelete(t *testing.T) {
	db := newRetentionDB(t)
	ctx := t.Context()

	svc := NewRetentionService(db, RetentionConfig{})

	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS incidents (
		id INTEGER PRIMARY KEY AUTOINCREMENT, gid TEXT DEFAULT '', tenant_id TEXT DEFAULT '',
		source TEXT DEFAULT '', status TEXT DEFAULT 'open', severity TEXT DEFAULT 'low',
		reason_code TEXT DEFAULT '', spam_user_id INTEGER DEFAULT 0, spam_user_name TEXT DEFAULT '',
		message_text TEXT DEFAULT '', chat_id INTEGER DEFAULT 0, rule_set_version INTEGER DEFAULT 0,
		created_at DATETIME, updated_at DATETIME)`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO incidents (tenant_id, created_at, updated_at) VALUES (?, ?, ?)`,
		"test", time.Now().Add(-365*24*time.Hour), time.Now().Add(-365*24*time.Hour))
	require.NoError(t, err)

	report, err := svc.CleanNow(ctx)
	require.NoError(t, err)
	_, exists := report["incidents"]
	assert.False(t, exists)
}

func newRetentionDB(t *testing.T) *engine.SQL {
	t.Helper()
	ctx := t.Context()
	connURL := "sqlite://:memory:"
	db, err := engine.New(ctx, connURL, "test")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}
