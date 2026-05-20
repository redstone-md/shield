package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/redstone-md/shield/app/storage/engine"
)

const (
	CmdCreateWorkspacesTable engine.DBCmd = iota + 800
	CmdCreateWorkspacesIndexes
	CmdAddWorkspace
	CmdGetWorkspace
	CmdListWorkspaces
	CmdUpdateWorkspace
	CmdAddWorkspaceMember
	CmdGetWorkspaceMember
	CmdListWorkspaceMembers
	CmdUpdateWorkspaceMemberRole
	CmdRemoveWorkspaceMember
	CmdAddWorkspacesGIDColumn
	CmdAddWorkspaceMembersGIDColumn
	CmdAddWorkspacesTenantIDColumn
	CmdAddWorkspaceMembersTenantIDColumn
)

var workspacesQueries = engine.NewQueryMap().
	Add(CmdCreateWorkspacesTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS workspaces (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	gid TEXT NOT NULL DEFAULT '',
	tenant_id TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL,
	owner_id TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(tenant_id, name)
);
CREATE TABLE IF NOT EXISTS workspace_members (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	gid TEXT NOT NULL DEFAULT '',
	tenant_id TEXT NOT NULL DEFAULT '',
	workspace_id INTEGER NOT NULL,
	user_id TEXT NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'viewer')),
	granted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(tenant_id, workspace_id, user_id)
)`,
		Postgres: `CREATE TABLE IF NOT EXISTS workspaces (
	id SERIAL PRIMARY KEY,
	gid TEXT NOT NULL DEFAULT '',
	tenant_id TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL,
	owner_id TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(tenant_id, name)
);
CREATE TABLE IF NOT EXISTS workspace_members (
	id SERIAL PRIMARY KEY,
	gid TEXT NOT NULL DEFAULT '',
	tenant_id TEXT NOT NULL DEFAULT '',
	workspace_id BIGINT NOT NULL,
	user_id TEXT NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'viewer')),
	granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(tenant_id, workspace_id, user_id)
)`,
	}).
	AddSame(CmdCreateWorkspacesIndexes, `
	CREATE INDEX IF NOT EXISTS idx_workspaces_gid ON workspaces(gid);
	CREATE INDEX IF NOT EXISTS idx_workspaces_owner ON workspaces(gid, owner_id);
	CREATE INDEX IF NOT EXISTS idx_workspaces_tenant_id ON workspaces(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_workspace_members_gid_ws ON workspace_members(gid, workspace_id);
	CREATE INDEX IF NOT EXISTS idx_workspace_members_tenant_id ON workspace_members(tenant_id);
	`).
	Add(CmdAddWorkspacesGIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE workspaces ADD COLUMN gid TEXT NOT NULL DEFAULT ''",
		Postgres: "ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS gid TEXT NOT NULL DEFAULT ''",
	}).
	Add(CmdAddWorkspaceMembersGIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE workspace_members ADD COLUMN gid TEXT NOT NULL DEFAULT ''",
		Postgres: "ALTER TABLE workspace_members ADD COLUMN IF NOT EXISTS gid TEXT NOT NULL DEFAULT ''",
	}).
	Add(CmdAddWorkspacesTenantIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE workspaces ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''",
		Postgres: "ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT ''",
	}).
	Add(CmdAddWorkspaceMembersTenantIDColumn, engine.Query{
		Sqlite:   "ALTER TABLE workspace_members ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''",
		Postgres: "ALTER TABLE workspace_members ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT ''",
	}).
	AddSame(CmdAddWorkspace, `INSERT INTO workspaces (gid, tenant_id, name, owner_id, status, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)`).
	AddSame(CmdGetWorkspace, `SELECT id, gid, name, owner_id, status, created_at, updated_at
	FROM workspaces WHERE tenant_id = ? AND name = ?`).
	AddSame(CmdListWorkspaces, `SELECT id, gid, name, owner_id, status, created_at, updated_at
	FROM workspaces WHERE tenant_id = ? ORDER BY created_at ASC`).
	AddSame(CmdUpdateWorkspace, `UPDATE workspaces SET name = ?, status = ?, updated_at = ?
	WHERE tenant_id = ? AND id = ?`).
	AddSame(CmdAddWorkspaceMember, `INSERT INTO workspace_members (tenant_id, workspace_id, user_id, role, granted_at)
	VALUES (?, ?, ?, ?, ?)`).
	AddSame(CmdGetWorkspaceMember, `SELECT id, gid, workspace_id, user_id, role, granted_at
	FROM workspace_members WHERE tenant_id = ? AND workspace_id = ? AND user_id = ?`).
	AddSame(CmdListWorkspaceMembers, `SELECT id, gid, workspace_id, user_id, role, granted_at
	FROM workspace_members WHERE tenant_id = ? AND workspace_id = ? ORDER BY granted_at ASC`).
	AddSame(CmdUpdateWorkspaceMemberRole, `UPDATE workspace_members SET role = ? WHERE tenant_id = ? AND workspace_id = ? AND user_id = ?`).
	AddSame(CmdRemoveWorkspaceMember, `DELETE FROM workspace_members WHERE tenant_id = ? AND workspace_id = ? AND user_id = ?`)

type WorkspaceRecord struct {
	ID        int64     `db:"id"`
	GID       string    `db:"gid"`
	TenantID  string    `db:"tenant_id"`
	Name      string    `db:"name"`
	OwnerID   string    `db:"owner_id"`
	Status    string    `db:"status"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type WorkspaceMemberRecord struct {
	ID          int64     `db:"id"`
	GID         string    `db:"gid"`
	TenantID    string    `db:"tenant_id"`
	WorkspaceID int64     `db:"workspace_id"`
	UserID      string    `db:"user_id"`
	Role        string    `db:"role"`
	GrantedAt   time.Time `db:"granted_at"`
}

type Workspaces struct {
	*engine.SQL
	engine.RWLocker
}

func NewWorkspaces(ctx context.Context, db *engine.SQL) (*Workspaces, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &Workspaces{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "workspaces",
		CreateTable:   CmdCreateWorkspacesTable,
		CreateIndexes: CmdCreateWorkspacesIndexes,
		MigrateFunc:   res.migrate,
		QueriesMap:    workspacesQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init workspaces storage: %w", err)
	}
	return res, nil
}

func (w *Workspaces) migrate(ctx context.Context, tx *sqlx.Tx, gid string) error {
	var count int
	err := tx.GetContext(ctx, &count, "SELECT COUNT(*) FROM workspaces WHERE gid = ''")
	if err == nil {
		migrateTenantID(ctx, tx, w.Type(), "workspaces")
		migrateTenantID(ctx, tx, w.Type(), "workspace_members")
		return nil
	}

	for _, cmd := range []engine.DBCmd{CmdAddWorkspacesGIDColumn, CmdAddWorkspaceMembersGIDColumn} {
		query, qErr := workspacesQueries.Pick(w.Type(), cmd)
		if qErr != nil {
			return fmt.Errorf("failed to get workspaces migration query %d: %w", cmd, qErr)
		}
		if _, execErr := tx.ExecContext(ctx, query); execErr != nil && !strings.Contains(execErr.Error(), "duplicate column") {
			return fmt.Errorf("failed to apply workspaces migration %d: %w", cmd, execErr)
		}
	}

	if _, err = tx.ExecContext(ctx, "UPDATE workspaces SET gid = ? WHERE gid = ''", gid); err != nil {
		return fmt.Errorf("failed to update gid for existing workspaces: %w", err)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE workspace_members SET gid = ? WHERE gid = ''", gid); err != nil {
		if !strings.Contains(err.Error(), "no such table") {
			return fmt.Errorf("failed to update gid for existing workspace_members: %w", err)
		}
	}

	migrateTenantID(ctx, tx, w.Type(), "workspaces")
	migrateTenantID(ctx, tx, w.Type(), "workspace_members")

	log.Printf("[DEBUG] workspaces and workspace_members tables migrated")
	return nil
}

func (w *Workspaces) Add(ctx context.Context, rec WorkspaceRecord) (int64, error) {
	w.Lock()
	defer w.Unlock()

	query, err := workspacesQueries.Pick(w.Type(), CmdAddWorkspace)
	if err != nil {
		return 0, fmt.Errorf("failed to get insert query: %w", err)
	}
	query = w.Adopt(query)

	now := time.Now().UTC()
	result, err := w.ExecContext(ctx, query, w.GID(), w.TenantID(), rec.Name, rec.OwnerID, rec.Status, now, now)
	if err != nil {
		return 0, fmt.Errorf("failed to insert workspace: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get inserted workspace id: %w", err)
	}
	return id, nil
}

func (w *Workspaces) Get(ctx context.Context, name string) (WorkspaceRecord, error) {
	w.RLock()
	defer w.RUnlock()

	query, err := workspacesQueries.Pick(w.Type(), CmdGetWorkspace)
	if err != nil {
		return WorkspaceRecord{}, fmt.Errorf("failed to get query: %w", err)
	}
	query = w.Adopt(query)

	var rec WorkspaceRecord
	if err := w.GetContext(ctx, &rec, query, w.TenantID(), name); err != nil {
		if err == sql.ErrNoRows {
			return WorkspaceRecord{}, fmt.Errorf("workspace %q not found: %w", name, err)
		}
		return WorkspaceRecord{}, fmt.Errorf("failed to get workspace: %w", err)
	}
	return rec, nil
}

func (w *Workspaces) List(ctx context.Context) ([]WorkspaceRecord, error) {
	w.RLock()
	defer w.RUnlock()

	query, err := workspacesQueries.Pick(w.Type(), CmdListWorkspaces)
	if err != nil {
		return nil, fmt.Errorf("failed to get list query: %w", err)
	}
	query = w.Adopt(query)

	var recs []WorkspaceRecord
	if err := w.SelectContext(ctx, &recs, query, w.TenantID()); err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}
	return recs, nil
}

func (w *Workspaces) Update(ctx context.Context, id int64, name, status string) error {
	w.Lock()
	defer w.Unlock()

	query, err := workspacesQueries.Pick(w.Type(), CmdUpdateWorkspace)
	if err != nil {
		return fmt.Errorf("failed to get update query: %w", err)
	}
	query = w.Adopt(query)

	if _, err := w.ExecContext(ctx, query, name, status, time.Now().UTC(), w.TenantID(), id); err != nil {
		return fmt.Errorf("failed to update workspace: %w", err)
	}
	return nil
}

func (w *Workspaces) AddMember(ctx context.Context, workspaceID int64, userID, role string) error {
	w.Lock()
	defer w.Unlock()

	query, err := workspacesQueries.Pick(w.Type(), CmdAddWorkspaceMember)
	if err != nil {
		return fmt.Errorf("failed to get add member query: %w", err)
	}
	query = w.Adopt(query)

	if _, err := w.ExecContext(ctx, query, w.TenantID(), workspaceID, userID, role, time.Now().UTC()); err != nil {
		return fmt.Errorf("failed to add workspace member: %w", err)
	}
	return nil
}

func (w *Workspaces) GetMember(ctx context.Context, workspaceID int64, userID string) (WorkspaceMemberRecord, error) {
	w.RLock()
	defer w.RUnlock()

	query, err := workspacesQueries.Pick(w.Type(), CmdGetWorkspaceMember)
	if err != nil {
		return WorkspaceMemberRecord{}, fmt.Errorf("failed to get member query: %w", err)
	}
	query = w.Adopt(query)

	var rec WorkspaceMemberRecord
	if err := w.GetContext(ctx, &rec, query, w.TenantID(), workspaceID, userID); err != nil {
		if err == sql.ErrNoRows {
			return WorkspaceMemberRecord{}, fmt.Errorf("member not found: %w", err)
		}
		return WorkspaceMemberRecord{}, fmt.Errorf("failed to get workspace member: %w", err)
	}
	return rec, nil
}

func (w *Workspaces) ListMembers(ctx context.Context, workspaceID int64) ([]WorkspaceMemberRecord, error) {
	w.RLock()
	defer w.RUnlock()

	query, err := workspacesQueries.Pick(w.Type(), CmdListWorkspaceMembers)
	if err != nil {
		return nil, fmt.Errorf("failed to get list members query: %w", err)
	}
	query = w.Adopt(query)

	var recs []WorkspaceMemberRecord
	if err := w.SelectContext(ctx, &recs, query, w.TenantID(), workspaceID); err != nil {
		return nil, fmt.Errorf("failed to list workspace members: %w", err)
	}
	return recs, nil
}

func (w *Workspaces) UpdateMemberRole(ctx context.Context, workspaceID int64, userID, role string) error {
	w.Lock()
	defer w.Unlock()

	query, err := workspacesQueries.Pick(w.Type(), CmdUpdateWorkspaceMemberRole)
	if err != nil {
		return fmt.Errorf("failed to get update member role query: %w", err)
	}
	query = w.Adopt(query)

	if _, err := w.ExecContext(ctx, query, role, w.TenantID(), workspaceID, userID); err != nil {
		return fmt.Errorf("failed to update workspace member role: %w", err)
	}
	return nil
}

func (w *Workspaces) RemoveMember(ctx context.Context, workspaceID int64, userID string) error {
	w.Lock()
	defer w.Unlock()

	query, err := workspacesQueries.Pick(w.Type(), CmdRemoveWorkspaceMember)
	if err != nil {
		return fmt.Errorf("failed to get remove member query: %w", err)
	}
	query = w.Adopt(query)

	if _, err := w.ExecContext(ctx, query, w.TenantID(), workspaceID, userID); err != nil {
		return fmt.Errorf("failed to remove workspace member: %w", err)
	}
	return nil
}
