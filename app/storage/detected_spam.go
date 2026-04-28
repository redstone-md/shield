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

	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

const maxDetectedSpamEntries = 500

// DetectedSpam is a storage for detected spam entries
type DetectedSpam struct {
	*engine.SQL
	engine.RWLocker
}

// DetectedSpamInfo represents information about a detected spam entry.
type DetectedSpamInfo struct {
	ID               int64                `db:"id"`
	GID              string               `db:"gid"`
	TenantID         string               `db:"tenant_id"`
	Text             string               `db:"text"`
	UserID           int64                `db:"user_id"`
	UserName         string               `db:"user_name"`
	Timestamp        time.Time            `db:"timestamp"`
	Added            bool                 `db:"added"`  // added to samples
	ChecksJSON       string               `db:"checks"` // store as JSON
	Checks           []spamcheck.Response `db:"-"`      // don't store in DB directly
	SignalSource     string               `db:"signal_source"`
	Score            float64              `db:"score"`
	MatchedRulesJSON string               `db:"matched_rules"`
	MatchedRules     []string             `db:"-"`
	RuleSetVersion   int                  `db:"rule_set_version"`
	IdempotencyKey   string               `db:"idempotency_key"`
}

// detected spam query commands
const (
	CmdCreateDetectedSpamTable engine.DBCmd = iota + 200
	CmdCreateDetectedSpamIndexes
	CmdAddDetectedSpamSignalSourceColumn
	CmdAddDetectedSpamScoreColumn
	CmdAddDetectedSpamMatchedRulesColumn
	CmdAddDetectedSpamRuleSetVersionColumn
	CmdAddDetectedSpamIdempotencyKeyColumn
	CmdAddDetectedSpamTenantIDColumn
)

// queries holds all detected spam queries
var detectedSpamQueries = engine.NewQueryMap().
	Add(CmdCreateDetectedSpamTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS detected_spam (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            gid TEXT NOT NULL DEFAULT '',
            tenant_id TEXT NOT NULL DEFAULT '',
            text TEXT,
            user_id INTEGER,
            user_name TEXT,
            timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
            added BOOLEAN DEFAULT 0,
            checks TEXT,
            signal_source TEXT DEFAULT '',
            score REAL DEFAULT 0,
            matched_rules TEXT DEFAULT '[]',
            rule_set_version INTEGER DEFAULT 0,
            idempotency_key TEXT DEFAULT ''
        )`,
		Postgres: `CREATE TABLE IF NOT EXISTS detected_spam (
            id SERIAL PRIMARY KEY,
            gid TEXT NOT NULL DEFAULT '',
            tenant_id TEXT NOT NULL DEFAULT '',
            text TEXT,
            user_id BIGINT,
            user_name TEXT,
            timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            added BOOLEAN DEFAULT false,
            checks TEXT,
            signal_source TEXT DEFAULT '',
            score DOUBLE PRECISION DEFAULT 0,
            matched_rules TEXT DEFAULT '[]',
            rule_set_version INTEGER DEFAULT 0,
            idempotency_key TEXT DEFAULT ''
        )`,
	}).
	AddSame(CmdCreateDetectedSpamIndexes, `
      	CREATE INDEX IF NOT EXISTS idx_detected_spam_gid_ts ON detected_spam(gid, timestamp DESC);
        CREATE INDEX IF NOT EXISTS idx_detected_spam_user_id_gid ON detected_spam(user_id, gid);
		CREATE INDEX IF NOT EXISTS idx_spam_gid_time ON detected_spam(gid, timestamp DESC);
        CREATE INDEX IF NOT EXISTS idx_detected_spam_gid ON detected_spam(gid);
        CREATE INDEX IF NOT EXISTS idx_detected_spam_tenant_id_ts ON detected_spam(tenant_id, timestamp DESC);
        CREATE INDEX IF NOT EXISTS idx_detected_spam_user_id_tenant_id ON detected_spam(user_id, tenant_id);
        CREATE INDEX IF NOT EXISTS idx_spam_tenant_id_time ON detected_spam(tenant_id, timestamp DESC);
        CREATE INDEX IF NOT EXISTS idx_detected_spam_tenant_id ON detected_spam(tenant_id)`,
	).
	Add(CmdAddDetectedSpamSignalSourceColumn, engine.Query{
		Sqlite:   "ALTER TABLE detected_spam ADD COLUMN signal_source TEXT DEFAULT ''",
		Postgres: "ALTER TABLE detected_spam ADD COLUMN IF NOT EXISTS signal_source TEXT DEFAULT ''",
	}).
	Add(CmdAddDetectedSpamScoreColumn, engine.Query{
		Sqlite:   "ALTER TABLE detected_spam ADD COLUMN score REAL DEFAULT 0",
		Postgres: "ALTER TABLE detected_spam ADD COLUMN IF NOT EXISTS score DOUBLE PRECISION DEFAULT 0",
	}).
	Add(CmdAddDetectedSpamMatchedRulesColumn, engine.Query{
		Sqlite:   "ALTER TABLE detected_spam ADD COLUMN matched_rules TEXT DEFAULT '[]'",
		Postgres: "ALTER TABLE detected_spam ADD COLUMN IF NOT EXISTS matched_rules TEXT DEFAULT '[]'",
	}).
	Add(CmdAddDetectedSpamRuleSetVersionColumn, engine.Query{
		Sqlite:   "ALTER TABLE detected_spam ADD COLUMN rule_set_version INTEGER DEFAULT 0",
		Postgres: "ALTER TABLE detected_spam ADD COLUMN IF NOT EXISTS rule_set_version INTEGER DEFAULT 0",
	}).
	Add(CmdAddDetectedSpamIdempotencyKeyColumn, engine.Query{
		Sqlite:   "ALTER TABLE detected_spam ADD COLUMN idempotency_key TEXT DEFAULT ''",
		Postgres: "ALTER TABLE detected_spam ADD COLUMN IF NOT EXISTS idempotency_key TEXT DEFAULT ''",
	}).
	Add(CmdAddGIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE detected_spam ADD COLUMN gid TEXT DEFAULT ''",
		Postgres: "ALTER TABLE detected_spam ADD COLUMN IF NOT EXISTS gid TEXT DEFAULT ''",
	}).
	Add(CmdAddDetectedSpamTenantIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE detected_spam ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''",
		Postgres: "ALTER TABLE detected_spam ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT ''",
	})

// NewDetectedSpam creates a new DetectedSpam storage
func NewDetectedSpam(ctx context.Context, db *engine.SQL) (*DetectedSpam, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &DetectedSpam{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "detected_spam",
		CreateTable:   CmdCreateDetectedSpamTable,
		CreateIndexes: CmdCreateDetectedSpamIndexes,
		MigrateFunc:   res.migrate,
		QueriesMap:    detectedSpamQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init detected spam storage: %w", err)
	}
	return res, nil
}

// Write adds a new detected spam entry
func (ds *DetectedSpam) Write(ctx context.Context, entry DetectedSpamInfo, checks []spamcheck.Response) error {
	ds.Lock()
	defer ds.Unlock()

	if entry.GID == "" {
		return fmt.Errorf("missing required GID field")
	}

	checksJSON, err := json.Marshal(checks)
	if err != nil {
		return fmt.Errorf("failed to marshal checks: %w", err)
	}
	matchedRulesJSON, err := json.Marshal(entry.MatchedRules)
	if err != nil {
		return fmt.Errorf("failed to marshal matched rules: %w", err)
	}

	query := ds.Adopt(`INSERT INTO detected_spam
		(gid, tenant_id, text, user_id, user_name, timestamp, checks, signal_source, score, matched_rules, rule_set_version, idempotency_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err = ds.ExecContext(ctx, query,
		entry.GID, ds.TenantID(), entry.Text, entry.UserID, entry.UserName, entry.Timestamp, string(checksJSON),
		entry.SignalSource, entry.Score, string(matchedRulesJSON), entry.RuleSetVersion, entry.IdempotencyKey)
	if err != nil {
		return fmt.Errorf("failed to insert detected spam entry: %w", err)
	}

	observability.Logf(ctx, "[INFO] detected spam entry added for gid:%s, user_id:%d, name:%s",
		entry.GID, entry.UserID, entry.UserName)
	return nil
}

// SetAddedToSamplesFlag sets the added flag to true for the detected spam entry with the given id
func (ds *DetectedSpam) SetAddedToSamplesFlag(ctx context.Context, id int64) error {
	ds.Lock()
	defer ds.Unlock()

	query := ds.Adopt("UPDATE detected_spam SET added = ? WHERE tenant_id = ? AND id = ?")
	if _, err := ds.ExecContext(ctx, query, true, ds.TenantID(), id); err != nil {
		return fmt.Errorf("failed to update added to samples flag: %w", err)
	}
	return nil
}

// Read returns the latest detected spam entries, up to maxDetectedSpamEntries
func (ds *DetectedSpam) Read(ctx context.Context) ([]DetectedSpamInfo, error) {
	ds.RLock()
	defer ds.RUnlock()

	query := ds.Adopt("SELECT * FROM detected_spam WHERE tenant_id = ? ORDER BY timestamp DESC LIMIT ?")
	var entries []DetectedSpamInfo
	err := ds.SelectContext(ctx, &entries, query, ds.TenantID(), maxDetectedSpamEntries)
	if err != nil {
		return nil, fmt.Errorf("failed to get detected spam entries: %w", err)
	}

	for i, entry := range entries {
		var checks []spamcheck.Response
		if err := json.Unmarshal([]byte(entry.ChecksJSON), &checks); err != nil {
			return nil, fmt.Errorf("failed to unmarshal checks for entry %d: %w", i, err)
		}
		var matchedRules []string
		if entry.MatchedRulesJSON != "" {
			if err := json.Unmarshal([]byte(entry.MatchedRulesJSON), &matchedRules); err != nil {
				return nil, fmt.Errorf("failed to unmarshal matched rules for entry %d: %w", i, err)
			}
		}
		entries[i].Checks = checks
		entries[i].MatchedRules = matchedRules
		entries[i].Timestamp = entry.Timestamp.Local()
	}
	return entries, nil
}

// FindByUserID returns the latest detected spam entry for the given user ID
func (ds *DetectedSpam) FindByUserID(ctx context.Context, userID int64) (*DetectedSpamInfo, error) {
	ds.RLock()
	defer ds.RUnlock()

	query := ds.Adopt("SELECT * FROM detected_spam WHERE user_id = ? AND tenant_id = ? ORDER BY timestamp DESC LIMIT 1")
	var entry DetectedSpamInfo
	err := ds.GetContext(ctx, &entry, query, userID, ds.TenantID())
	if errors.Is(err, sql.ErrNoRows) {
		// not found, return nil *DetectedSpamInfo instead of error
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get detected spam entry for user_id %d: %w", userID, err)
	}

	var checks []spamcheck.Response
	if err := json.Unmarshal([]byte(entry.ChecksJSON), &checks); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checks for entry: %w", err)
	}
	var matchedRules []string
	if entry.MatchedRulesJSON != "" {
		if err := json.Unmarshal([]byte(entry.MatchedRulesJSON), &matchedRules); err != nil {
			return nil, fmt.Errorf("failed to unmarshal matched rules for entry: %w", err)
		}
	}
	entry.Checks = checks
	entry.MatchedRules = matchedRules
	entry.Timestamp = entry.Timestamp.Local()
	return &entry, nil
}

// CountByUserID returns the number of detected spam entries for the given user in the current gid.
func (ds *DetectedSpam) CountByUserID(ctx context.Context, userID int64) (int, error) {
	ds.RLock()
	defer ds.RUnlock()

	query := ds.Adopt("SELECT COUNT(*) FROM detected_spam WHERE user_id = ? AND tenant_id = ?")
	var count int
	if err := ds.GetContext(ctx, &count, query, userID, ds.TenantID()); err != nil {
		return 0, fmt.Errorf("failed to count detected spam entries for user_id %d: %w", userID, err)
	}
	return count, nil
}

func (ds *DetectedSpam) migrate(ctx context.Context, tx *sqlx.Tx, gid string) error {
	var count int
	err := tx.GetContext(ctx, &count, "SELECT COUNT(*) FROM detected_spam WHERE gid = '' AND signal_source = ''")
	if err == nil {
		migrateTenantID(ctx, tx, ds.Type(), "detected_spam")
		return nil
	}

	addGIDQuery, err := detectedSpamQueries.Pick(ds.Type(), CmdAddGIDColumn)
	if err != nil {
		return fmt.Errorf("failed to get add GID query: %w", err)
	}

	_, err = tx.ExecContext(ctx, addGIDQuery)
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return fmt.Errorf("failed to add gid column: %w", err)
	}

	for _, cmd := range []engine.DBCmd{
		CmdAddDetectedSpamSignalSourceColumn,
		CmdAddDetectedSpamScoreColumn,
		CmdAddDetectedSpamMatchedRulesColumn,
		CmdAddDetectedSpamRuleSetVersionColumn,
		CmdAddDetectedSpamIdempotencyKeyColumn,
	} {
		query, qErr := detectedSpamQueries.Pick(ds.Type(), cmd)
		if qErr != nil {
			return fmt.Errorf("failed to get migration query: %w", qErr)
		}
		if _, execErr := tx.ExecContext(ctx, query); execErr != nil && !strings.Contains(execErr.Error(), "duplicate column") {
			return fmt.Errorf("failed to apply detected spam migration %d: %w", cmd, execErr)
		}
	}

	if _, err = tx.ExecContext(ctx, "UPDATE detected_spam SET gid = ? WHERE gid = ''", gid); err != nil {
		return fmt.Errorf("failed to update gid for existing records: %w", err)
	}

	migrateTenantID(ctx, tx, ds.Type(), "detected_spam")
	log.Printf("[DEBUG] detected_spam table migrated")
	return nil
}
