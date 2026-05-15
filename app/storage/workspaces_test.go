package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/storage/engine"
)

func TestWorkspacesAddAndGet(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewWorkspaces(context.Background(), db)
	require.NoError(t, err)

	id, err := store.Add(context.Background(), WorkspaceRecord{
		Name:    "test-workspace",
		OwnerID: "user-1",
		Status:  "active",
	})
	require.NoError(t, err)
	assert.Positive(t, id)

	rec, err := store.Get(context.Background(), "test-workspace")
	require.NoError(t, err)
	assert.Equal(t, "gr1", rec.GID)
	assert.Equal(t, "test-workspace", rec.Name)
	assert.Equal(t, "user-1", rec.OwnerID)
	assert.Equal(t, "active", rec.Status)
}

func TestWorkspacesList(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewWorkspaces(context.Background(), db)
	require.NoError(t, err)

	_, err = store.Add(context.Background(), WorkspaceRecord{Name: "ws-1", OwnerID: "user-1", Status: "active"})
	require.NoError(t, err)
	_, err = store.Add(context.Background(), WorkspaceRecord{Name: "ws-2", OwnerID: "user-2", Status: "active"})
	require.NoError(t, err)

	recs, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, recs, 2)
	assert.Equal(t, "ws-1", recs[0].Name)
	assert.Equal(t, "ws-2", recs[1].Name)
}

func TestWorkspacesUpdate(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewWorkspaces(context.Background(), db)
	require.NoError(t, err)

	id, err := store.Add(context.Background(), WorkspaceRecord{Name: "ws-old", OwnerID: "user-1", Status: "active"})
	require.NoError(t, err)

	err = store.Update(context.Background(), id, "ws-new", "suspended")
	require.NoError(t, err)

	rec, err := store.Get(context.Background(), "ws-new")
	require.NoError(t, err)
	assert.Equal(t, "ws-new", rec.Name)
	assert.Equal(t, "suspended", rec.Status)
}

func TestWorkspacesMembers(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewWorkspaces(context.Background(), db)
	require.NoError(t, err)

	wsID, err := store.Add(context.Background(), WorkspaceRecord{Name: "ws-m", OwnerID: "owner-1", Status: "active"})
	require.NoError(t, err)

	err = store.AddMember(context.Background(), wsID, "admin-1", "admin")
	require.NoError(t, err)
	err = store.AddMember(context.Background(), wsID, "viewer-1", "viewer")
	require.NoError(t, err)

	members, err := store.ListMembers(context.Background(), wsID)
	require.NoError(t, err)
	require.Len(t, members, 2)
	assert.Equal(t, "admin-1", members[0].UserID)
	assert.Equal(t, "admin", members[0].Role)
	assert.Equal(t, "viewer-1", members[1].UserID)
	assert.Equal(t, "viewer", members[1].Role)

	member, err := store.GetMember(context.Background(), wsID, "admin-1")
	require.NoError(t, err)
	assert.Equal(t, "admin", member.Role)

	err = store.UpdateMemberRole(context.Background(), wsID, "admin-1", "owner")
	require.NoError(t, err)
	member, err = store.GetMember(context.Background(), wsID, "admin-1")
	require.NoError(t, err)
	assert.Equal(t, "owner", member.Role)

	err = store.RemoveMember(context.Background(), wsID, "viewer-1")
	require.NoError(t, err)
	members, err = store.ListMembers(context.Background(), wsID)
	require.NoError(t, err)
	assert.Len(t, members, 1)
}

func TestWorkspacesMigrateFromOldSchema(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS workspaces (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	owner_id TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS workspace_members (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id INTEGER NOT NULL,
		user_id TEXT NOT NULL,
		role TEXT NOT NULL,
		granted_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO workspaces (name, owner_id, status, created_at, updated_at) VALUES ('old-ws', 'user-1', 'active', ?, ?)", time.Now(), time.Now())
	require.NoError(t, err)

	store, err := NewWorkspaces(context.Background(), db)
	require.NoError(t, err)

	recs, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, "gr1", recs[0].GID)
}
