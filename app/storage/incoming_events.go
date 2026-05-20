package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/redstone-md/shield/app/moderation"
	"github.com/redstone-md/shield/app/storage/engine"
)

const (
	CmdCreateIncomingEventsTable engine.DBCmd = iota + 600
	CmdCreateIncomingEventsIndexes
	CmdAddIncomingEvent
	CmdGetIncomingEventByKey
	CmdAddDecisionActionColumn
	CmdAddDecisionReasonColumn
	CmdAddDecisionScoreColumn
	CmdAddActionAppliedColumn
	CmdAddActionErrorColumn
	CmdAddProcessedAtColumn
	CmdCompleteIncomingEvent
	CmdAddIncomingEventsTenantIDColumn
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
			decision_action TEXT DEFAULT '',
			decision_reason TEXT DEFAULT '',
			decision_score REAL DEFAULT 0,
			action_applied BOOLEAN,
			action_error TEXT DEFAULT '',
			processed_at DATETIME,
			received_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(tenant_id, idempotency_key)
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
			decision_action TEXT DEFAULT '',
			decision_reason TEXT DEFAULT '',
			decision_score DOUBLE PRECISION DEFAULT 0,
			action_applied BOOLEAN,
			action_error TEXT DEFAULT '',
			processed_at TIMESTAMP,
			received_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(tenant_id, idempotency_key)
		)`,
	}).
	AddSame(CmdCreateIncomingEventsIndexes, `
		CREATE INDEX IF NOT EXISTS idx_incoming_events_gid_key ON incoming_events(gid, idempotency_key);
		CREATE INDEX IF NOT EXISTS idx_incoming_events_gid_received ON incoming_events(gid, received_at DESC);
		CREATE INDEX IF NOT EXISTS idx_incoming_events_gid_event ON incoming_events(gid, event_id);
		CREATE INDEX IF NOT EXISTS idx_incoming_events_tenant_id ON incoming_events(tenant_id);
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
			ON CONFLICT (tenant_id, idempotency_key) DO NOTHING`,
	}).
	AddSame(CmdGetIncomingEventByKey, `SELECT
			id, gid, event_id, correlation_id, tenant_id, source, update_id, chat_id, message_id,
			edited_message_id, idempotency_key, decision_action, decision_reason, decision_score,
			action_applied, action_error, processed_at, received_at, created_at
		FROM incoming_events
		WHERE tenant_id = ? AND idempotency_key = ?`).
	Add(CmdAddDecisionActionColumn, engine.Query{
		Sqlite:   "ALTER TABLE incoming_events ADD COLUMN decision_action TEXT DEFAULT ''",
		Postgres: "ALTER TABLE incoming_events ADD COLUMN IF NOT EXISTS decision_action TEXT DEFAULT ''",
	}).
	Add(CmdAddDecisionReasonColumn, engine.Query{
		Sqlite:   "ALTER TABLE incoming_events ADD COLUMN decision_reason TEXT DEFAULT ''",
		Postgres: "ALTER TABLE incoming_events ADD COLUMN IF NOT EXISTS decision_reason TEXT DEFAULT ''",
	}).
	Add(CmdAddDecisionScoreColumn, engine.Query{
		Sqlite:   "ALTER TABLE incoming_events ADD COLUMN decision_score REAL DEFAULT 0",
		Postgres: "ALTER TABLE incoming_events ADD COLUMN IF NOT EXISTS decision_score DOUBLE PRECISION DEFAULT 0",
	}).
	Add(CmdAddActionAppliedColumn, engine.Query{
		Sqlite:   "ALTER TABLE incoming_events ADD COLUMN action_applied BOOLEAN",
		Postgres: "ALTER TABLE incoming_events ADD COLUMN IF NOT EXISTS action_applied BOOLEAN",
	}).
	Add(CmdAddActionErrorColumn, engine.Query{
		Sqlite:   "ALTER TABLE incoming_events ADD COLUMN action_error TEXT DEFAULT ''",
		Postgres: "ALTER TABLE incoming_events ADD COLUMN IF NOT EXISTS action_error TEXT DEFAULT ''",
	}).
	Add(CmdAddProcessedAtColumn, engine.Query{
		Sqlite:   "ALTER TABLE incoming_events ADD COLUMN processed_at DATETIME",
		Postgres: "ALTER TABLE incoming_events ADD COLUMN IF NOT EXISTS processed_at TIMESTAMP",
	}).
	AddSame(CmdCompleteIncomingEvent, `UPDATE incoming_events
		SET decision_action = ?, decision_reason = ?, decision_score = ?, action_applied = ?, action_error = ?, processed_at = ?
		WHERE tenant_id = ? AND idempotency_key = ? AND processed_at IS NULL`).
	Add(CmdAddIncomingEventsTenantIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE incoming_events ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''",
		Postgres: "ALTER TABLE incoming_events ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT ''",
	})

// IncomingEventRecord stores one normalized Telegram ingress event.
type IncomingEventRecord struct {
	ID              int64        `db:"id"`
	GID             string       `db:"gid"`
	EventID         string       `db:"event_id"`
	CorrelationID   string       `db:"correlation_id"`
	TenantID        string       `db:"tenant_id"`
	Source          string       `db:"source"`
	UpdateID        int          `db:"update_id"`
	ChatID          int64        `db:"chat_id"`
	MessageID       int          `db:"message_id"`
	EditedMessageID int          `db:"edited_message_id"`
	IdempotencyKey  string       `db:"idempotency_key"`
	DecisionAction  string       `db:"decision_action"`
	DecisionReason  string       `db:"decision_reason"`
	DecisionScore   float64      `db:"decision_score"`
	ActionApplied   sql.NullBool `db:"action_applied"`
	ActionError     string       `db:"action_error"`
	ProcessedAt     sql.NullTime `db:"processed_at"`
	ReceivedAt      time.Time    `db:"received_at"`
	CreatedAt       time.Time    `db:"created_at"`
}

// IncomingEventReplay is the replay state for one idempotency key.
type IncomingEventReplay struct {
	Recorded     bool
	Processed    bool
	Decision     moderation.PolicyDecision
	ActionResult moderation.ModerationActionResult
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

func (s *IncomingEvents) migrate(ctx context.Context, tx *sqlx.Tx, _ string) error {
	var count int
	err := tx.GetContext(ctx, &count, "SELECT COUNT(*) FROM incoming_events WHERE processed_at IS NULL")
	if err != nil {
		for _, cmd := range []engine.DBCmd{
			CmdAddDecisionActionColumn,
			CmdAddDecisionReasonColumn,
			CmdAddDecisionScoreColumn,
			CmdAddActionAppliedColumn,
			CmdAddActionErrorColumn,
			CmdAddProcessedAtColumn,
		} {
			query, pickErr := incomingEventsQueries.Pick(s.Type(), cmd)
			if pickErr != nil {
				return fmt.Errorf("failed to get migration query %d: %w", cmd, pickErr)
			}
			if _, execErr := tx.ExecContext(ctx, query); execErr != nil && !strings.Contains(execErr.Error(), "duplicate column") {
				return fmt.Errorf("failed to apply incoming events migration %d: %w", cmd, execErr)
			}
		}
	}

	migrateTenantID(ctx, tx, s.Type(), "incoming_events")

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
		s.TenantID(),
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

// Reserve records the event or returns the previously completed replay state for the same key.
func (s *IncomingEvents) Reserve(ctx context.Context, event moderation.IncomingEvent) (IncomingEventReplay, error) {
	created, err := s.Record(ctx, event)
	if err != nil {
		return IncomingEventReplay{}, err
	}
	if created {
		return IncomingEventReplay{Recorded: true}, nil
	}

	record, err := s.ByIdempotencyKey(ctx, event.IdempotencyKey)
	if err != nil {
		return IncomingEventReplay{}, err
	}

	replay := IncomingEventReplay{
		Recorded:  false,
		Processed: record.ProcessedAt.Valid,
	}
	if !record.ProcessedAt.Valid && (record.DecisionAction != "" || record.ActionError != "" || record.ActionApplied.Valid) {
		replay.Recorded = true
	}
	if record.ProcessedAt.Valid {
		replay.Decision = moderation.PolicyDecision{
			EventID:       record.EventID,
			CorrelationID: record.CorrelationID,
			Action:        moderation.Action(record.DecisionAction),
			Reason:        record.DecisionReason,
			Score:         record.DecisionScore,
			DecidedAt:     record.ProcessedAt.Time,
		}
		replay.ActionResult = moderation.ModerationActionResult{
			EventID:       record.EventID,
			CorrelationID: record.CorrelationID,
			Action:        moderation.Action(record.DecisionAction),
			Applied:       record.ActionApplied.Valid && record.ActionApplied.Bool,
			Provider:      "telegram",
			Error:         record.ActionError,
			AppliedAt:     record.ProcessedAt.Time,
		}
	}
	return replay, nil
}

// Complete persists the final decision/action snapshot for replay-safe duplicate suppression.
func (s *IncomingEvents) Complete(ctx context.Context, idempotencyKey string,
	decision moderation.PolicyDecision, actionResult moderation.ModerationActionResult,
) error {
	s.Lock()
	defer s.Unlock()

	query, err := incomingEventsQueries.Pick(s.Type(), CmdCompleteIncomingEvent)
	if err != nil {
		return fmt.Errorf("failed to get complete query: %w", err)
	}
	query = s.Adopt(query)

	markProcessed := shouldMarkProcessed(decision, actionResult)
	processedAt := actionResult.AppliedAt
	if processedAt.IsZero() && markProcessed {
		processedAt = decision.DecidedAt
	}
	if processedAt.IsZero() && markProcessed {
		processedAt = time.Now().UTC()
	}
	var processedAtValue any
	if markProcessed {
		processedAtValue = processedAt
	}

	_, err = s.ExecContext(ctx, query,
		string(decision.Action),
		decision.Reason,
		decision.Score,
		actionResult.Applied,
		actionResult.Error,
		processedAtValue,
		s.TenantID(),
		idempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("failed to complete incoming event: %w", err)
	}
	return nil
}

func shouldMarkProcessed(decision moderation.PolicyDecision, actionResult moderation.ModerationActionResult) bool {
	if actionResult.Error != "" {
		return false
	}
	if actionResult.Applied {
		return true
	}
	return decision.Action == moderation.ActionAllow
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
	if err := s.GetContext(ctx, &record, query, s.TenantID(), key); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return IncomingEventRecord{}, err
		}
		return IncomingEventRecord{}, fmt.Errorf("failed to load incoming event: %w", err)
	}
	return record, nil
}
