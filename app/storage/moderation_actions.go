package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateModerationActionsTable engine.DBCmd = iota + 700
	CmdCreateModerationActionsIndexes
	CmdAddModerationAction
)

var moderationActionsQueries = engine.NewQueryMap().
	Add(CmdCreateModerationActionsTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS moderation_actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			gid TEXT NOT NULL DEFAULT '',
			event_id TEXT NOT NULL,
			correlation_id TEXT NOT NULL DEFAULT '',
			idempotency_key TEXT NOT NULL DEFAULT '',
			command TEXT NOT NULL,
			status TEXT NOT NULL,
			chat_id INTEGER NOT NULL DEFAULT 0,
			subject_id INTEGER NOT NULL DEFAULT 0,
			message_id INTEGER NOT NULL DEFAULT 0,
			attempt INTEGER NOT NULL DEFAULT 1,
			last_error TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS moderation_actions (
			id SERIAL PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			event_id TEXT NOT NULL,
			correlation_id TEXT NOT NULL DEFAULT '',
			idempotency_key TEXT NOT NULL DEFAULT '',
			command TEXT NOT NULL,
			status TEXT NOT NULL,
			chat_id BIGINT NOT NULL DEFAULT 0,
			subject_id BIGINT NOT NULL DEFAULT 0,
			message_id INTEGER NOT NULL DEFAULT 0,
			attempt INTEGER NOT NULL DEFAULT 1,
			last_error TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}).
	AddSame(CmdCreateModerationActionsIndexes, `
		CREATE INDEX IF NOT EXISTS idx_moderation_actions_gid_event ON moderation_actions(gid, event_id);
		CREATE INDEX IF NOT EXISTS idx_moderation_actions_gid_key ON moderation_actions(gid, idempotency_key);
		CREATE INDEX IF NOT EXISTS idx_moderation_actions_gid_created ON moderation_actions(gid, created_at DESC);
	`).
	AddSame(CmdAddModerationAction, `INSERT INTO moderation_actions
		(gid, event_id, correlation_id, idempotency_key, command, status, chat_id, subject_id, message_id, attempt, last_error, created_at)
		VALUES (:gid, :event_id, :correlation_id, :idempotency_key, :command, :status, :chat_id, :subject_id, :message_id, :attempt, :last_error, :created_at)`)

// ModerationActionEntry stores one executor command attempt.
type ModerationActionEntry struct {
	ID             int64     `db:"id"`
	GID            string    `db:"gid"`
	EventID        string    `db:"event_id"`
	CorrelationID  string    `db:"correlation_id"`
	IdempotencyKey string    `db:"idempotency_key"`
	Command        string    `db:"command"`
	Status         string    `db:"status"`
	ChatID         int64     `db:"chat_id"`
	SubjectID      int64     `db:"subject_id"`
	MessageID      int       `db:"message_id"`
	Attempt        int       `db:"attempt"`
	LastError      string    `db:"last_error"`
	CreatedAt      time.Time `db:"created_at"`
}

// ModerationActions persists executor command attempts.
type ModerationActions struct {
	*engine.SQL
	engine.RWLocker
}

// NewModerationActions creates a new ModerationActions storage.
func NewModerationActions(ctx context.Context, db *engine.SQL) (*ModerationActions, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &ModerationActions{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "moderation_actions",
		CreateTable:   CmdCreateModerationActionsTable,
		CreateIndexes: CmdCreateModerationActionsIndexes,
		MigrateFunc:   res.migrate,
		QueriesMap:    moderationActionsQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init moderation actions storage: %w", err)
	}
	return res, nil
}

func (m *ModerationActions) migrate(_ context.Context, _ *sqlx.Tx, _ string) error {
	return nil
}

// Add records one moderation command attempt.
func (m *ModerationActions) Add(ctx context.Context, entry ModerationActionEntry) error {
	m.Lock()
	defer m.Unlock()

	if entry.Attempt == 0 {
		entry.Attempt = 1
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	entry.GID = m.GID()

	query, err := moderationActionsQueries.Pick(m.Type(), CmdAddModerationAction)
	if err != nil {
		return fmt.Errorf("failed to get insert query: %w", err)
	}
	query = m.Adopt(query)

	if _, err := m.NamedExecContext(ctx, query, entry); err != nil {
		return fmt.Errorf("failed to insert moderation action: %w", err)
	}
	return nil
}

// ByEventID returns moderation action attempts for the event ordered by creation.
func (m *ModerationActions) ByEventID(ctx context.Context, eventID string) ([]ModerationActionEntry, error) {
	m.RLock()
	defer m.RUnlock()

	query := m.Adopt(`SELECT * FROM moderation_actions WHERE gid = ? AND event_id = ? ORDER BY created_at ASC, id ASC`)
	var entries []ModerationActionEntry
	if err := m.SelectContext(ctx, &entries, query, m.GID(), eventID); err != nil {
		return nil, fmt.Errorf("failed to get moderation actions: %w", err)
	}
	return entries, nil
}
