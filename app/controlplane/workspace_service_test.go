package controlplane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/app/storage/engine"
)

func TestWorkspaceService_EnsureDefaultWorkspace(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewWorkspaces(context.Background(), db)
	require.NoError(t, err)
	svc := NewWorkspaceService(store)

	ws, err := svc.EnsureDefaultWorkspace(context.Background(), WorkspaceBootstrap{
		Name:    "gr1",
		OwnerID: "tg-spam",
	})
	require.NoError(t, err)
	assert.Equal(t, "gr1", ws.Name)
	assert.Equal(t, "tg-spam", ws.OwnerID)
	assert.Equal(t, "active", ws.Status)

	member, err := store.GetMember(context.Background(), ws.ID, "tg-spam")
	require.NoError(t, err)
	assert.Equal(t, string(RoleOwner), member.Role)

	again, err := svc.EnsureDefaultWorkspace(context.Background(), WorkspaceBootstrap{
		Name:    "gr1",
		OwnerID: "tg-spam",
	})
	require.NoError(t, err)
	assert.Equal(t, ws.ID, again.ID)

	workspaces, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, workspaces, 1)

	members, err := store.ListMembers(context.Background(), ws.ID)
	require.NoError(t, err)
	assert.Len(t, members, 1)
}

func TestWorkspaceService_EnsureMemberValidation(t *testing.T) {
	svc := NewWorkspaceService(nil)

	_, err := svc.EnsureDefaultWorkspace(context.Background(), WorkspaceBootstrap{Name: "gr1", OwnerID: "tg-spam"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace store is nil")

	db, dbErr := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, dbErr)
	defer db.Close()

	store, dbErr := storage.NewWorkspaces(context.Background(), db)
	require.NoError(t, dbErr)
	svc = NewWorkspaceService(store)

	_, err = svc.EnsureDefaultWorkspace(context.Background(), WorkspaceBootstrap{Name: "", OwnerID: "tg-spam"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace name is required")

	_, err = svc.EnsureDefaultWorkspace(context.Background(), WorkspaceBootstrap{Name: "gr1", OwnerID: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace owner id is required")

	err = svc.EnsureMember(context.Background(), 0, "user-1", RoleAdmin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace id is required")

	err = svc.EnsureMember(context.Background(), 1, "user-1", WorkspaceRole("editor"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid workspace role")
}

func TestWorkspaceService_EnsureMemberDoesNotSilentlyChangeRole(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewWorkspaces(context.Background(), db)
	require.NoError(t, err)
	svc := NewWorkspaceService(store)

	ws, err := svc.EnsureDefaultWorkspace(context.Background(), WorkspaceBootstrap{Name: "gr1", OwnerID: "owner-1"})
	require.NoError(t, err)

	err = svc.EnsureMember(context.Background(), ws.ID, "viewer-1", RoleViewer)
	require.NoError(t, err)

	err = svc.EnsureMember(context.Background(), ws.ID, "viewer-1", RoleAdmin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `already has role "viewer"`)
}
