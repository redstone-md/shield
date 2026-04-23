package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateRuleSetsTables engine.DBCmd = iota + 500
	CmdCreateRuleSetsIndexes
	CmdAddRuleSetsGIDColumn
	CmdAddRuleSetVersionsGIDColumn
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
	}).
	Add(CmdAddRuleSetsGIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE rule_sets ADD COLUMN gid TEXT NOT NULL DEFAULT ''",
		Postgres: "ALTER TABLE rule_sets ADD COLUMN IF NOT EXISTS gid TEXT NOT NULL DEFAULT ''",
	}).
	Add(CmdAddRuleSetVersionsGIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE rule_set_versions ADD COLUMN gid TEXT NOT NULL DEFAULT ''",
		Postgres: "ALTER TABLE rule_set_versions ADD COLUMN IF NOT EXISTS gid TEXT NOT NULL DEFAULT ''",
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

func (rs *RuleSets) migrate(ctx context.Context, tx *sqlx.Tx, gid string) error {
	var count int
	err := tx.GetContext(ctx, &count, "SELECT COUNT(*) FROM rule_sets WHERE gid = ''")
	if err == nil {
		return nil
	}

	for _, cmd := range []engine.DBCmd{CmdAddRuleSetsGIDColumn, CmdAddRuleSetVersionsGIDColumn} {
		query, qErr := ruleSetQueries.Pick(rs.Type(), cmd)
		if qErr != nil {
			return fmt.Errorf("failed to get rule sets migration query %d: %w", cmd, qErr)
		}
		if _, execErr := tx.ExecContext(ctx, query); execErr != nil && !strings.Contains(execErr.Error(), "duplicate column") {
			return fmt.Errorf("failed to apply rule sets migration %d: %w", cmd, execErr)
		}
	}

	if _, err = tx.ExecContext(ctx, "UPDATE rule_sets SET gid = ? WHERE gid = ''", gid); err != nil {
		return fmt.Errorf("failed to update gid for existing rule_sets: %w", err)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE rule_set_versions SET gid = ? WHERE gid = ''", gid); err != nil {
		return fmt.Errorf("failed to update gid for existing rule_set_versions: %w", err)
	}

	log.Printf("[DEBUG] rule_sets and rule_set_versions tables migrated")
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

// Update persists a new version of the RuleSet and makes it active.
// Returns the new version number.
func (rs *RuleSets) Update(ctx context.Context, ruleSet rules.RuleSet) (int, error) {
	if ruleSet.WorkspaceID == "" {
		return 0, fmt.Errorf("workspace id is required")
	}

	rs.Lock()
	defer rs.Unlock()

	var currentVersion int
	query := rs.Adopt(`SELECT active_version FROM rule_sets WHERE workspace_id = ? AND gid = ?`)
	err := rs.GetContext(ctx, &currentVersion, query, ruleSet.WorkspaceID, rs.GID())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("failed to query current rule set version: %w", err)
	}
	newVersion := currentVersion + 1

	payload, err := json.Marshal(ruleSet)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal rule set: %w", err)
	}

	tx, err := rs.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	insertVersion := rs.Adopt(`INSERT INTO rule_set_versions (workspace_id, gid, version, source, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if _, err = tx.ExecContext(ctx, insertVersion, ruleSet.WorkspaceID, rs.GID(), newVersion, ruleSet.Source, string(payload), time.Now()); err != nil {
		return 0, fmt.Errorf("failed to insert rule set version %d: %w", newVersion, err)
	}

	upsertActive := upsertRuleSetQuery(rs.Type())
	if _, err = tx.ExecContext(ctx, upsertActive, ruleSet.WorkspaceID, rs.GID(), newVersion, time.Now()); err != nil {
		return 0, fmt.Errorf("failed to upsert active rule set: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit rule set update: %w", err)
	}
	return newVersion, nil
}

// History returns recent versions for a workspace, ordered by version descending.
func (rs *RuleSets) History(ctx context.Context, workspaceID string, limit int) ([]rules.RuleSet, error) {
	rs.RLock()
	defer rs.RUnlock()

	if limit <= 0 {
		limit = 10
	}
	query := rs.Adopt(`SELECT version, source, payload, created_at
		FROM rule_set_versions
		WHERE workspace_id = ? AND gid = ?
		ORDER BY version DESC LIMIT ?`)

	var rows []struct {
		Version   int       `db:"version"`
		Source    string    `db:"source"`
		Payload   string    `db:"payload"`
		CreatedAt time.Time `db:"created_at"`
	}
	if err := rs.SelectContext(ctx, &rows, query, workspaceID, rs.GID(), limit); err != nil {
		return nil, fmt.Errorf("failed to get rule set history: %w", err)
	}

	result := make([]rules.RuleSet, 0, len(rows))
	for _, row := range rows {
		var rs_ rules.RuleSet
		if err := json.Unmarshal([]byte(row.Payload), &rs_); err != nil {
			return nil, fmt.Errorf("failed to decode rule set version %d: %w", row.Version, err)
		}
		rs_.Version = row.Version
		rs_.Source = row.Source
		rs_.CreatedAt = row.CreatedAt
		result = append(result, rs_)
	}
	return result, nil
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
