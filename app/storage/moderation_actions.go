package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateModerationActionsTable engine.DBCmd = iota + 700
	CmdCreateModerationActionsIndexes
	CmdAddModerationAction
	CmdGetLatestModerationAction
	CmdAddModerationActionsGIDColumn
	CmdAddModerationActionsTenantIDColumn
)

var moderationActionsQueries = engine.NewQueryMap().
	Add(CmdCreateModerationActionsTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS moderation_actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
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
			tenant_id TEXT NOT NULL DEFAULT '',
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
		CREATE INDEX IF NOT EXISTS idx_moderation_actions_tenant_id ON moderation_actions(tenant_id);
	`).
	AddSame(CmdAddModerationAction, `INSERT INTO moderation_actions
		(gid, tenant_id, event_id, correlation_id, idempotency_key, command, status, chat_id, subject_id, message_id, attempt, last_error, created_at)
		VALUES (:gid, :tenant_id, :event_id, :correlation_id, :idempotency_key, :command, :status, :chat_id, :subject_id, :message_id, :attempt, :last_error, :created_at)`).
	AddSame(CmdGetLatestModerationAction, `SELECT
	id, gid, event_id, correlation_id, idempotency_key, command, status,
	chat_id, subject_id, message_id, attempt, last_error, created_at
	FROM moderation_actions
	WHERE tenant_id = ? AND idempotency_key = ? AND command = ? AND chat_id = ? AND subject_id = ? AND message_id = ?
	ORDER BY attempt DESC, id DESC
	LIMIT 1`).
	Add(CmdAddModerationActionsGIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE moderation_actions ADD COLUMN gid TEXT NOT NULL DEFAULT ''",
		Postgres: "ALTER TABLE moderation_actions ADD COLUMN IF NOT EXISTS gid TEXT NOT NULL DEFAULT ''",
	}).
	Add(CmdAddModerationActionsTenantIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE moderation_actions ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''",
		Postgres: "ALTER TABLE moderation_actions ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT ''",
	})

// ModerationActionEntry stores one executor command attempt.
type ModerationActionEntry struct {
	ID             int64     `db:"id"`
	GID            string    `db:"gid"`
	TenantID       string    `db:"tenant_id"`
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

// ModerationActionLookup identifies one idempotent action command target.
type ModerationActionLookup struct {
	IdempotencyKey string
	Command        string
	ChatID         int64
	SubjectID      int64
	MessageID      int
}

// ModerationActionReplay returns the latest persisted action attempt for one target.
type ModerationActionReplay struct {
	Found     bool
	Completed bool
	Attempt   int
	LastError string
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

func (m *ModerationActions) migrate(ctx context.Context, tx *sqlx.Tx, gid string) error {
	var count int
	err := tx.GetContext(ctx, &count, "SELECT COUNT(*) FROM moderation_actions WHERE gid = ''")
	if err != nil {
		addGIDQuery, qErr := moderationActionsQueries.Pick(m.Type(), CmdAddModerationActionsGIDColumn)
		if qErr != nil {
			return fmt.Errorf("failed to get add GID query: %w", qErr)
		}
		if _, execErr := tx.ExecContext(ctx, addGIDQuery); execErr != nil && !strings.Contains(execErr.Error(), "duplicate column") {
			return fmt.Errorf("failed to add gid column to moderation_actions: %w", execErr)
		}

		if _, err = tx.ExecContext(ctx, "UPDATE moderation_actions SET gid = ? WHERE gid = ''", gid); err != nil {
			return fmt.Errorf("failed to update gid for existing moderation_actions: %w", err)
		}
		log.Printf("[DEBUG] moderation_actions table migrated")
	}

	migrateTenantID(ctx, tx, m.Type(), "moderation_actions")
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
	entry.TenantID = m.TenantID()

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

	query := m.Adopt(`SELECT * FROM moderation_actions WHERE tenant_id = ? AND event_id = ? ORDER BY created_at ASC, id ASC`)
	var entries []ModerationActionEntry
	if err := m.SelectContext(ctx, &entries, query, m.TenantID(), eventID); err != nil {
		return nil, fmt.Errorf("failed to get moderation actions: %w", err)
	}
	return entries, nil
}

// Last returns the latest action attempt for the same idempotency key and command target.
func (m *ModerationActions) Last(ctx context.Context, lookup ModerationActionLookup) (ModerationActionReplay, error) {
	m.RLock()
	defer m.RUnlock()

	query, err := moderationActionsQueries.Pick(m.Type(), CmdGetLatestModerationAction)
	if err != nil {
		return ModerationActionReplay{}, fmt.Errorf("failed to get latest moderation action query: %w", err)
	}
	query = m.Adopt(query)

	var entry ModerationActionEntry
	if err := m.GetContext(ctx, &entry, query,
		m.TenantID(),
		lookup.IdempotencyKey,
		lookup.Command,
		lookup.ChatID,
		lookup.SubjectID,
		lookup.MessageID,
	); err != nil {
		if err == sql.ErrNoRows {
			return ModerationActionReplay{}, nil
		}
		return ModerationActionReplay{}, fmt.Errorf("failed to get latest moderation action: %w", err)
	}

	return ModerationActionReplay{
		Found:     true,
		Completed: entry.Status == "completed",
		Attempt:   entry.Attempt,
		LastError: entry.LastError,
	}, nil
}
