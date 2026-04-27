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
	CmdCreateTenantsTable engine.DBCmd = iota + 900
	CmdCreateTenantsIndexes
	CmdAddTenant
	CmdGetTenant
	CmdGetTenantByName
	CmdListTenants
	CmdUpdateTenantStatus
	CmdUpdateTenantName
)

var tenantsQueries = engine.NewQueryMap().
	Add(CmdCreateTenantsTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
			owner_id TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(gid, name)
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
			owner_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(gid, name)
		)`,
	}).
	AddSame(CmdCreateTenantsIndexes, `
		CREATE INDEX IF NOT EXISTS idx_tenants_gid ON tenants(gid);
		CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(gid, status);
		CREATE INDEX IF NOT EXISTS idx_tenants_name ON tenants(gid, name);
	`).
	AddSame(CmdAddTenant, `INSERT INTO tenants (id, gid, tenant_id, name, status, owner_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`).
	AddSame(CmdGetTenant, `SELECT id, gid, name, status, owner_id, created_at, updated_at
		FROM tenants WHERE tenant_id = ? AND id = ?`).
	AddSame(CmdGetTenantByName, `SELECT id, gid, name, status, owner_id, created_at, updated_at
		FROM tenants WHERE tenant_id = ? AND name = ?`).
	AddSame(CmdListTenants, `SELECT id, gid, name, status, owner_id, created_at, updated_at
		FROM tenants WHERE tenant_id = ? ORDER BY created_at ASC`).
	AddSame(CmdUpdateTenantStatus, `UPDATE tenants SET status = ?, updated_at = ?
		WHERE tenant_id = ? AND id = ?`).
	AddSame(CmdUpdateTenantName, `UPDATE tenants SET name = ?, updated_at = ?
		WHERE tenant_id = ? AND id = ?`)

type TenantRecord struct {
	ID        string    `db:"id"`
	GID       string    `db:"gid"`
	Name      string    `db:"name"`
	Status    string    `db:"status"`
	OwnerID   string    `db:"owner_id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type Tenants struct {
	*engine.SQL
	engine.RWLocker
}

func NewTenants(ctx context.Context, db *engine.SQL) (*Tenants, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &Tenants{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "tenants",
		CreateTable:   CmdCreateTenantsTable,
		CreateIndexes: CmdCreateTenantsIndexes,
		MigrateFunc:   res.migrate,
		QueriesMap:    tenantsQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init tenants storage: %w", err)
	}
	return res, nil
}

func (t *Tenants) migrate(_ context.Context, _ *sqlx.Tx, _ string) error {
	log.Printf("[DEBUG] tenants table migration check (no-op, new table)")
	return nil
}

func (t *Tenants) Add(ctx context.Context, rec TenantRecord) error {
	t.Lock()
	defer t.Unlock()

	query, err := tenantsQueries.Pick(t.Type(), CmdAddTenant)
	if err != nil {
		return fmt.Errorf("failed to get insert query: %w", err)
	}
	query = t.Adopt(query)

	now := time.Now().UTC()
	if _, err := t.ExecContext(ctx, query, rec.ID, t.GID(), t.TenantID(), rec.Name, rec.Status, rec.OwnerID, now, now); err != nil {
		return fmt.Errorf("failed to insert tenant: %w", err)
	}
	return nil
}

func (t *Tenants) Get(ctx context.Context, id string) (TenantRecord, error) {
	t.RLock()
	defer t.RUnlock()

	query, err := tenantsQueries.Pick(t.Type(), CmdGetTenant)
	if err != nil {
		return TenantRecord{}, fmt.Errorf("failed to get query: %w", err)
	}
	query = t.Adopt(query)

	var rec TenantRecord
	if err := t.GetContext(ctx, &rec, query, 	t.TenantID(), id); err != nil {
		if err == sql.ErrNoRows {
			return TenantRecord{}, fmt.Errorf("tenant %q not found: %w", id, err)
		}
		return TenantRecord{}, fmt.Errorf("failed to get tenant: %w", err)
	}
	return rec, nil
}

func (t *Tenants) GetByName(ctx context.Context, name string) (TenantRecord, error) {
	t.RLock()
	defer t.RUnlock()

	query, err := tenantsQueries.Pick(t.Type(), CmdGetTenantByName)
	if err != nil {
		return TenantRecord{}, fmt.Errorf("failed to get query: %w", err)
	}
	query = t.Adopt(query)

	var rec TenantRecord
	if err := t.GetContext(ctx, &rec, query, 	t.TenantID(), name); err != nil {
		if err == sql.ErrNoRows {
			return TenantRecord{}, fmt.Errorf("tenant %q not found: %w", name, err)
		}
		return TenantRecord{}, fmt.Errorf("failed to get tenant by name: %w", err)
	}
	return rec, nil
}

func (t *Tenants) List(ctx context.Context) ([]TenantRecord, error) {
	t.RLock()
	defer t.RUnlock()

	query, err := tenantsQueries.Pick(t.Type(), CmdListTenants)
	if err != nil {
		return nil, fmt.Errorf("failed to get list query: %w", err)
	}
	query = t.Adopt(query)

	var recs []TenantRecord
	if err := t.SelectContext(ctx, &recs, query, 	t.TenantID()); err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	return recs, nil
}

func (t *Tenants) UpdateStatus(ctx context.Context, id, status string) error {
	t.Lock()
	defer t.Unlock()

	query, err := tenantsQueries.Pick(t.Type(), CmdUpdateTenantStatus)
	if err != nil {
		return fmt.Errorf("failed to get update status query: %w", err)
	}
	query = t.Adopt(query)

	if _, err := t.ExecContext(ctx, query, status, time.Now().UTC(), 	t.TenantID(), id); err != nil {
		return fmt.Errorf("failed to update tenant status: %w", err)
	}
	return nil
}

func (t *Tenants) UpdateName(ctx context.Context, id, name string) error {
	t.Lock()
	defer t.Unlock()

	query, err := tenantsQueries.Pick(t.Type(), CmdUpdateTenantName)
	if err != nil {
		return fmt.Errorf("failed to get update name query: %w", err)
	}
	query = t.Adopt(query)

	if _, err := t.ExecContext(ctx, query, name, time.Now().UTC(), 	t.TenantID(), id); err != nil {
		return fmt.Errorf("failed to update tenant name: %w", err)
	}
	return nil
}

func (t *Tenants) BootstrapDefault(ctx context.Context, id, name, ownerID string) error {
	_, err := t.Get(ctx, id)
	if err == nil {
		return nil
	}

	if !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("failed to check existing tenant: %w", err)
	}

	rec := TenantRecord{
		ID:      id,
		Name:    name,
		Status:  "active",
		OwnerID: ownerID,
	}
	if err := t.Add(ctx, rec); err != nil {
		return fmt.Errorf("failed to bootstrap default tenant: %w", err)
	}
	log.Printf("[INFO] bootstrapped default tenant: id=%s name=%s", id, name)
	return nil
}
