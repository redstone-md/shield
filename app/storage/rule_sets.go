package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateRuleSetsTables engine.DBCmd = iota + 500
	CmdCreateRuleSetsIndexes
)

var ruleSetQueries = engine.NewQueryMap().
	Add(CmdCreateRuleSetsTables, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS rule_sets (
			workspace_id TEXT PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			active_version INTEGER NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS rule_set_versions (
			workspace_id TEXT NOT NULL,
			gid TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL,
			source TEXT NOT NULL,
			payload TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (workspace_id, version)
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS rule_sets (
			workspace_id TEXT PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			active_version INTEGER NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS rule_set_versions (
			workspace_id TEXT NOT NULL,
			gid TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL,
			source TEXT NOT NULL,
			payload TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (workspace_id, version)
		)`,
	}).
	Add(CmdCreateRuleSetsIndexes, engine.Query{
		Sqlite: `
			CREATE INDEX IF NOT EXISTS idx_rule_sets_gid ON rule_sets(gid);
			CREATE INDEX IF NOT EXISTS idx_rule_set_versions_gid ON rule_set_versions(gid);
			CREATE INDEX IF NOT EXISTS idx_rule_set_versions_workspace_created
				ON rule_set_versions(workspace_id, created_at DESC)`,
		Postgres: `
			CREATE INDEX IF NOT EXISTS idx_rule_sets_gid ON rule_sets(gid);
			CREATE INDEX IF NOT EXISTS idx_rule_set_versions_gid ON rule_set_versions(gid);
			CREATE INDEX IF NOT EXISTS idx_rule_set_versions_workspace_created
				ON rule_set_versions(workspace_id, created_at DESC)`,
	})

// RuleSets stores active and versioned moderation rule sets.
type RuleSets struct {
	*engine.SQL
	engine.RWLocker
}

// NewRuleSets creates a new RuleSets storage.
func NewRuleSets(ctx context.Context, db *engine.SQL) (*RuleSets, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &RuleSets{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "rule_sets",
		CreateTable:   CmdCreateRuleSetsTables,
		CreateIndexes: CmdCreateRuleSetsIndexes,
		MigrateFunc:   res.migrate,
		QueriesMap:    ruleSetQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init rule sets storage: %w", err)
	}
	return res, nil
}

func (rs *RuleSets) migrate(_ context.Context, _ *sqlx.Tx, _ string) error {
	return nil
}

// EnsureBootstrap inserts the initial RuleSet if the workspace has none yet.
// Returns true when a new bootstrap snapshot was persisted.
func (rs *RuleSets) EnsureBootstrap(ctx context.Context, ruleSet rules.RuleSet) (bool, error) {
	if ruleSet.WorkspaceID == "" {
		return false, fmt.Errorf("workspace id is required")
	}

	rs.Lock()
	defer rs.Unlock()

	var activeVersion int
	query := rs.Adopt(`SELECT active_version FROM rule_sets WHERE workspace_id = ? AND gid = ?`)
	err := rs.GetContext(ctx, &activeVersion, query, ruleSet.WorkspaceID, rs.GID())
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("failed to query active rule set: %w", err)
	}

	payload, err := json.Marshal(ruleSet)
	if err != nil {
		return false, fmt.Errorf("failed to marshal rule set: %w", err)
	}

	tx, err := rs.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	insertVersion := rs.Adopt(`INSERT INTO rule_set_versions (workspace_id, gid, version, source, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if _, err = tx.ExecContext(ctx, insertVersion, ruleSet.WorkspaceID, rs.GID(), 1, ruleSet.Source, string(payload), time.Now()); err != nil {
		return false, fmt.Errorf("failed to insert bootstrap rule set version: %w", err)
	}

	insertActive := upsertRuleSetQuery(rs.Type())
	if _, err = tx.ExecContext(ctx, insertActive, ruleSet.WorkspaceID, rs.GID(), 1, time.Now()); err != nil {
		return false, fmt.Errorf("failed to upsert active rule set: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit bootstrap rule set: %w", err)
	}
	return true, nil
}

// Active returns the active RuleSet for a workspace.
func (rs *RuleSets) Active(ctx context.Context, workspaceID string) (rules.RuleSet, error) {
	rs.RLock()
	defer rs.RUnlock()

	var row struct {
		ActiveVersion int       `db:"active_version"`
		Payload       string    `db:"payload"`
		CreatedAt     time.Time `db:"created_at"`
	}
	query := rs.Adopt(`SELECT rs.active_version, rsv.payload, rsv.created_at
		FROM rule_sets rs
		JOIN rule_set_versions rsv
		  ON rsv.workspace_id = rs.workspace_id
		 AND rsv.version = rs.active_version
		WHERE rs.workspace_id = ? AND rs.gid = ? AND rsv.gid = ?`)
	if err := rs.GetContext(ctx, &row, query, workspaceID, rs.GID(), rs.GID()); err != nil {
		return rules.RuleSet{}, fmt.Errorf("failed to get active rule set: %w", err)
	}

	var ruleSet rules.RuleSet
	if err := json.Unmarshal([]byte(row.Payload), &ruleSet); err != nil {
		return rules.RuleSet{}, fmt.Errorf("failed to decode active rule set: %w", err)
	}
	ruleSet.Version = row.ActiveVersion
	ruleSet.CreatedAt = row.CreatedAt
	return ruleSet, nil
}

func upsertRuleSetQuery(dbType engine.Type) string {
	if dbType == engine.Postgres {
		return `INSERT INTO rule_sets (workspace_id, gid, active_version, updated_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (workspace_id) DO UPDATE
			SET gid = EXCLUDED.gid, active_version = EXCLUDED.active_version, updated_at = EXCLUDED.updated_at`
	}
	return `INSERT OR REPLACE INTO rule_sets (workspace_id, gid, active_version, updated_at)
		VALUES (?, ?, ?, ?)`
}
