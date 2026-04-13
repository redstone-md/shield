package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateIncomingEventsTable engine.DBCmd = iota + 600
	CmdCreateIncomingEventsIndexes
	CmdAddIncomingEvent
	CmdGetIncomingEventByKey
)

var incomingEventsQueries = engine.NewQueryMap().
	Add(CmdCreateIncomingEventsTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS incoming_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			gid TEXT NOT NULL DEFAULT '',
			event_id TEXT NOT NULL,
			correlation_id TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL,
			update_id INTEGER NOT NULL DEFAULT 0,
			chat_id INTEGER NOT NULL,
			message_id INTEGER NOT NULL DEFAULT 0,
			edited_message_id INTEGER NOT NULL DEFAULT 0,
			idempotency_key TEXT NOT NULL,
			received_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(gid, idempotency_key)
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS incoming_events (
			id SERIAL PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			event_id TEXT NOT NULL,
			correlation_id TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL,
			update_id INTEGER NOT NULL DEFAULT 0,
			chat_id BIGINT NOT NULL,
			message_id INTEGER NOT NULL DEFAULT 0,
			edited_message_id INTEGER NOT NULL DEFAULT 0,
			idempotency_key TEXT NOT NULL,
			received_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(gid, idempotency_key)
		)`,
	}).
	AddSame(CmdCreateIncomingEventsIndexes, `
		CREATE INDEX IF NOT EXISTS idx_incoming_events_gid_key ON incoming_events(gid, idempotency_key);
		CREATE INDEX IF NOT EXISTS idx_incoming_events_gid_received ON incoming_events(gid, received_at DESC);
		CREATE INDEX IF NOT EXISTS idx_incoming_events_gid_event ON incoming_events(gid, event_id);
	`).
	Add(CmdAddIncomingEvent, engine.Query{
		Sqlite: `INSERT OR IGNORE INTO incoming_events
			(gid, event_id, correlation_id, tenant_id, source, update_id, chat_id, message_id, edited_message_id,
			 idempotency_key, received_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		Postgres: `INSERT INTO incoming_events
			(gid, event_id, correlation_id, tenant_id, source, update_id, chat_id, message_id, edited_message_id,
			 idempotency_key, received_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (gid, idempotency_key) DO NOTHING`,
	}).
	AddSame(CmdGetIncomingEventByKey, `SELECT
			id, gid, event_id, correlation_id, tenant_id, source, update_id, chat_id, message_id,
			edited_message_id, idempotency_key, received_at, created_at
		FROM incoming_events
		WHERE gid = ? AND idempotency_key = ?`)

// IncomingEventRecord stores one normalized Telegram ingress event.
type IncomingEventRecord struct {
	ID              int64     `db:"id"`
	GID             string    `db:"gid"`
	EventID         string    `db:"event_id"`
	CorrelationID   string    `db:"correlation_id"`
	TenantID        string    `db:"tenant_id"`
	Source          string    `db:"source"`
	UpdateID        int       `db:"update_id"`
	ChatID          int64     `db:"chat_id"`
	MessageID       int       `db:"message_id"`
	EditedMessageID int       `db:"edited_message_id"`
	IdempotencyKey  string    `db:"idempotency_key"`
	ReceivedAt      time.Time `db:"received_at"`
	CreatedAt       time.Time `db:"created_at"`
}

// IncomingEvents persists normalized ingress events keyed by idempotency key.
type IncomingEvents struct {
	*engine.SQL
	engine.RWLocker
}

// NewIncomingEvents creates a new IncomingEvents storage.
func NewIncomingEvents(ctx context.Context, db *engine.SQL) (*IncomingEvents, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &IncomingEvents{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "incoming_events",
		CreateTable:   CmdCreateIncomingEventsTable,
		CreateIndexes: CmdCreateIncomingEventsIndexes,
		MigrateFunc:   res.migrate,
		QueriesMap:    incomingEventsQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init incoming events storage: %w", err)
	}
	return res, nil
}

func (s *IncomingEvents) migrate(_ context.Context, _ *sqlx.Tx, _ string) error {
	return nil
}

// Record persists the incoming event if its idempotency key has not been seen before.
// It returns false when the same gid/key pair already exists.
func (s *IncomingEvents) Record(ctx context.Context, event moderation.IncomingEvent) (bool, error) {
	if event.IdempotencyKey == "" {
		return false, fmt.Errorf("idempotency key is required")
	}

	s.Lock()
	defer s.Unlock()

	receivedAt := event.ReceivedAt
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	query, err := incomingEventsQueries.Pick(s.Type(), CmdAddIncomingEvent)
	if err != nil {
		return false, fmt.Errorf("failed to get insert query: %w", err)
	}
	query = s.Adopt(query)

	result, err := s.ExecContext(ctx, query,
		s.GID(),
		event.EventID,
		event.CorrelationID,
		event.TenantID,
		event.Source,
		event.UpdateID,
		event.ChatID,
		event.MessageID,
		event.EditedMessageID,
		event.IdempotencyKey,
		receivedAt,
	)
	if err != nil {
		return false, fmt.Errorf("failed to insert incoming event: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to inspect insert result: %w", err)
	}
	return rows > 0, nil
}

// ByIdempotencyKey loads a previously recorded incoming event.
func (s *IncomingEvents) ByIdempotencyKey(ctx context.Context, key string) (IncomingEventRecord, error) {
	s.RLock()
	defer s.RUnlock()

	query, err := incomingEventsQueries.Pick(s.Type(), CmdGetIncomingEventByKey)
	if err != nil {
		return IncomingEventRecord{}, fmt.Errorf("failed to get select query: %w", err)
	}
	query = s.Adopt(query)

	var record IncomingEventRecord
	if err := s.GetContext(ctx, &record, query, s.GID(), key); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return IncomingEventRecord{}, err
		}
		return IncomingEventRecord{}, fmt.Errorf("failed to load incoming event: %w", err)
	}
	return record, nil
}
