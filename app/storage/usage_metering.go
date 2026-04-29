package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateUsageCountersTable engine.DBCmd = iota + 1000
	CmdCreateUsageCountersIndexes
	CmdIncrementUsageCounter
	CmdGetUsageCounter
	CmdGetUsageCountersByWindow
	CmdResetUsageCounters
	CmdCleanupUsageCounters
)

var usageCountersQueries = engine.NewQueryMap().
	Add(CmdCreateUsageCountersTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS usage_counters (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			meter_type TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			window_start DATETIME NOT NULL,
			window_end DATETIME NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(tenant_id, meter_type, window_start)
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS usage_counters (
			id SERIAL PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			meter_type TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 0,
			window_start TIMESTAMP NOT NULL,
			window_end TIMESTAMP NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(tenant_id, meter_type, window_start)
		)`,
	}).
	AddSame(CmdCreateUsageCountersIndexes, `
		CREATE INDEX IF NOT EXISTS idx_usage_counters_tenant_type ON usage_counters(tenant_id, meter_type);
		CREATE INDEX IF NOT EXISTS idx_usage_counters_window ON usage_counters(tenant_id, window_start, window_end);
	`).
	Add(CmdIncrementUsageCounter, engine.Query{
		Sqlite: `INSERT INTO usage_counters (gid, tenant_id, meter_type, count, window_start, window_end, updated_at)
			VALUES (?, ?, ?, 1, ?, ?, ?)
			ON CONFLICT(tenant_id, meter_type, window_start) DO UPDATE SET count = count + 1, updated_at = excluded.updated_at`,
		Postgres: `INSERT INTO usage_counters (gid, tenant_id, meter_type, count, window_start, window_end, updated_at)
			VALUES ($1, $2, $3, 1, $4, $5, $6)
			ON CONFLICT(tenant_id, meter_type, window_start) DO UPDATE SET count = count + 1, updated_at = excluded.updated_at`,
	}).
	AddSame(CmdGetUsageCounter, `SELECT id, tenant_id, meter_type, count, window_start, window_end, updated_at
		FROM usage_counters WHERE tenant_id = ? AND meter_type = ? AND window_start = ?`).
	AddSame(CmdGetUsageCountersByWindow, `SELECT id, tenant_id, meter_type, count, window_start, window_end, updated_at
		FROM usage_counters WHERE tenant_id = ? AND window_start >= ? AND window_end <= ?
		ORDER BY meter_type, window_start`).
	AddSame(CmdResetUsageCounters, `DELETE FROM usage_counters WHERE tenant_id = ? AND window_end < ?`).
	AddSame(CmdCleanupUsageCounters, `DELETE FROM usage_counters WHERE window_end < ?`)

type UsageCounter struct {
	ID          int64     `db:"id" json:"id"`
	GID         string    `db:"gid" json:"-"`
	TenantID    string    `db:"tenant_id" json:"-"`
	MeterType   string    `db:"meter_type" json:"meter_type"`
	Count       int       `db:"count" json:"count"`
	WindowStart time.Time `db:"window_start" json:"window_start"`
	WindowEnd   time.Time `db:"window_end" json:"window_end"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

type UsageMetering struct {
	*engine.SQL
	engine.RWLocker
}

func NewUsageMetering(ctx context.Context, db *engine.SQL) (*UsageMetering, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &UsageMetering{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "usage_counters",
		CreateTable:   CmdCreateUsageCountersTable,
		CreateIndexes: CmdCreateUsageCountersIndexes,
		MigrateFunc:   res.migrate,
		QueriesMap:    usageCountersQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init usage metering storage: %w", err)
	}
	return res, nil
}

func (u *UsageMetering) migrate(ctx context.Context, tx *sqlx.Tx, _ string) error {
	migrateTenantID(ctx, tx, u.Type(), "usage_counters")
	return nil
}

func (u *UsageMetering) Increment(ctx context.Context, meterType string, windowStart, windowEnd time.Time) error {
	u.Lock()
	defer u.Unlock()

	query, err := usageCountersQueries.Pick(u.Type(), CmdIncrementUsageCounter)
	if err != nil {
		return fmt.Errorf("failed to get query: %w", err)
	}
	query = u.Adopt(query)

	now := time.Now().UTC()
	if _, err := u.ExecContext(ctx, query, u.GID(), u.TenantID(), meterType, windowStart.UTC(), windowEnd.UTC(), now); err != nil {
		return fmt.Errorf("failed to increment usage counter: %w", err)
	}
	return nil
}

func (u *UsageMetering) Get(ctx context.Context, meterType string, windowStart time.Time) (UsageCounter, error) {
	u.RLock()
	defer u.RUnlock()

	query, err := usageCountersQueries.Pick(u.Type(), CmdGetUsageCounter)
	if err != nil {
		return UsageCounter{}, fmt.Errorf("failed to get query: %w", err)
	}
	query = u.Adopt(query)

	var counter UsageCounter
	if err := u.GetContext(ctx, &counter, query, u.TenantID(), meterType, windowStart.UTC()); err != nil {
		return UsageCounter{}, fmt.Errorf("failed to get usage counter: %w", err)
	}
	return counter, nil
}

func (u *UsageMetering) ListByWindow(ctx context.Context, from, to time.Time) ([]UsageCounter, error) {
	u.RLock()
	defer u.RUnlock()

	query, err := usageCountersQueries.Pick(u.Type(), CmdGetUsageCountersByWindow)
	if err != nil {
		return nil, fmt.Errorf("failed to get query: %w", err)
	}
	query = u.Adopt(query)

	var counters []UsageCounter
	if err := u.SelectContext(ctx, &counters, query, u.TenantID(), from.UTC(), to.UTC()); err != nil {
		return nil, fmt.Errorf("failed to list usage counters: %w", err)
	}
	return counters, nil
}

func (u *UsageMetering) Cleanup(ctx context.Context, olderThan time.Time) error {
	u.Lock()
	defer u.Unlock()

	query, err := usageCountersQueries.Pick(u.Type(), CmdCleanupUsageCounters)
	if err != nil {
		return fmt.Errorf("failed to get query: %w", err)
	}
	query = u.Adopt(query)

	if _, err := u.ExecContext(ctx, query, olderThan.UTC()); err != nil {
		return fmt.Errorf("failed to cleanup usage counters: %w", err)
	}
	return nil
}

func (u *UsageMetering) ResetTenant(ctx context.Context, olderThan time.Time) error {
	u.Lock()
	defer u.Unlock()

	query, err := usageCountersQueries.Pick(u.Type(), CmdResetUsageCounters)
	if err != nil {
		return fmt.Errorf("failed to get query: %w", err)
	}
	query = u.Adopt(query)

	if _, err := u.ExecContext(ctx, query, u.TenantID(), olderThan.UTC()); err != nil {
		return fmt.Errorf("failed to reset tenant usage counters: %w", err)
	}
	return nil
}
