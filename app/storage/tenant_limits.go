package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateTenantLimitsTable engine.DBCmd = iota + 950
	CmdCreateTenantLimitsIndexes
	CmdGetTenantLimit
	CmdSetTenantLimit
	CmdIncrementTenantLimit
	CmdResetTenantLimit
)

var tenantLimitsQueries = engine.NewQueryMap().
	Add(CmdCreateTenantLimitsTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS tenant_limits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			limit_type TEXT NOT NULL,
			limit_value INTEGER NOT NULL DEFAULT 0,
			current_usage INTEGER NOT NULL DEFAULT 0,
			window_start DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(tenant_id, limit_type)
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS tenant_limits (
			id SERIAL PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			limit_type TEXT NOT NULL,
			limit_value INTEGER NOT NULL DEFAULT 0,
			current_usage INTEGER NOT NULL DEFAULT 0,
			window_start TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(tenant_id, limit_type)
		)`,
	}).
	AddSame(CmdCreateTenantLimitsIndexes, `
		CREATE INDEX IF NOT EXISTS idx_tenant_limits_type ON tenant_limits(tenant_id, limit_type);
	`).
	AddSame(CmdGetTenantLimit, `SELECT limit_value, current_usage, window_start
		FROM tenant_limits WHERE tenant_id = ? AND limit_type = ?`).
	AddSame(CmdSetTenantLimit, `INSERT INTO tenant_limits (gid, tenant_id, limit_type, limit_value, current_usage, window_start, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT(tenant_id, limit_type) DO UPDATE SET limit_value = excluded.limit_value, updated_at = excluded.updated_at`).
	AddSame(CmdIncrementTenantLimit, `UPDATE tenant_limits SET current_usage = current_usage + 1, updated_at = ?
		WHERE tenant_id = ? AND limit_type = ?`).
	AddSame(CmdResetTenantLimit, `UPDATE tenant_limits SET current_usage = 0, window_start = ?, updated_at = ?
		WHERE tenant_id = ? AND limit_type = ?`)

type TenantLimitRecord struct {
	ID           int64     `db:"id"`
	GID          string    `db:"gid"`
	TenantID     string    `db:"tenant_id"`
	LimitType    string    `db:"limit_type"`
	LimitValue   int       `db:"limit_value"`
	CurrentUsage int       `db:"current_usage"`
	WindowStart  time.Time `db:"window_start"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type TenantLimits struct {
	*engine.SQL
	engine.RWLocker
}

func NewTenantLimits(ctx context.Context, db *engine.SQL) (*TenantLimits, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &TenantLimits{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "tenant_limits",
		CreateTable:   CmdCreateTenantLimitsTable,
		CreateIndexes: CmdCreateTenantLimitsIndexes,
		MigrateFunc:   res.migrate,
		QueriesMap:    tenantLimitsQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init tenant limits storage: %w", err)
	}
	return res, nil
}

func (tl *TenantLimits) migrate(ctx context.Context, tx *sqlx.Tx, _ string) error {
	migrateTenantID(ctx, tx, tl.Type(), "tenant_limits")
	return nil
}

func (tl *TenantLimits) Get(ctx context.Context, limitType string) (TenantLimitRecord, error) {
	tl.RLock()
	defer tl.RUnlock()

	query, err := tenantLimitsQueries.Pick(tl.Type(), CmdGetTenantLimit)
	if err != nil {
		return TenantLimitRecord{}, fmt.Errorf("failed to get query: %w", err)
	}
	query = tl.Adopt(query)

	var rec TenantLimitRecord
	if err := tl.GetContext(ctx, &rec, query, tl.TenantID(), limitType); err != nil {
		if err == sql.ErrNoRows {
			return TenantLimitRecord{}, nil
		}
		return TenantLimitRecord{}, fmt.Errorf("failed to get tenant limit: %w", err)
	}
	return rec, nil
}

func (tl *TenantLimits) Set(ctx context.Context, limitType string, limitValue int) error {
	tl.Lock()
	defer tl.Unlock()

	query, err := tenantLimitsQueries.Pick(tl.Type(), CmdSetTenantLimit)
	if err != nil {
		return fmt.Errorf("failed to get set query: %w", err)
	}
	query = tl.Adopt(query)

	now := time.Now().UTC()
	if _, err := tl.ExecContext(ctx, query, tl.GID(), tl.TenantID(), limitType, limitValue, now, now); err != nil {
		return fmt.Errorf("failed to set tenant limit: %w", err)
	}
	return nil
}

func (tl *TenantLimits) Increment(ctx context.Context, limitType string) error {
	tl.Lock()
	defer tl.Unlock()

	query, err := tenantLimitsQueries.Pick(tl.Type(), CmdIncrementTenantLimit)
	if err != nil {
		return fmt.Errorf("failed to get increment query: %w", err)
	}
	query = tl.Adopt(query)

	if _, err := tl.ExecContext(ctx, query, time.Now().UTC(), tl.TenantID(), limitType); err != nil {
		return fmt.Errorf("failed to increment tenant limit: %w", err)
	}
	return nil
}

func (tl *TenantLimits) Reset(ctx context.Context, limitType string) error {
	tl.Lock()
	defer tl.Unlock()

	query, err := tenantLimitsQueries.Pick(tl.Type(), CmdResetTenantLimit)
	if err != nil {
		return fmt.Errorf("failed to get reset query: %w", err)
	}
	query = tl.Adopt(query)

	now := time.Now().UTC()
	if _, err := tl.ExecContext(ctx, query, now, now, tl.TenantID(), limitType); err != nil {
		return fmt.Errorf("failed to reset tenant limit: %w", err)
	}
	return nil
}
