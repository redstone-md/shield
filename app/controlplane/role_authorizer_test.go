package controlplane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
)

func TestRoleAuthorizer_Authorize(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewWorkspaces(context.Background(), db)
	require.NoError(t, err)

	wsID, err := store.Add(context.Background(), storage.WorkspaceRecord{
		Name:    "gr1",
		OwnerID: "owner-1",
		Status:  "active",
	})
	require.NoError(t, err)
	require.NoError(t, store.AddMember(context.Background(), wsID, "owner-1", string(RoleOwner)))
	require.NoError(t, store.AddMember(context.Background(), wsID, "admin-1", string(RoleAdmin)))
	require.NoError(t, store.AddMember(context.Background(), wsID, "viewer-1", string(RoleViewer)))

	auth := NewRoleAuthorizer(store)

	require.NoError(t, auth.Authorize(context.Background(), "gr1", "owner-1", AccessWrite))
	require.NoError(t, auth.Authorize(context.Background(), "gr1", "admin-1", AccessWrite))
	require.NoError(t, auth.Authorize(context.Background(), "gr1", "viewer-1", AccessRead))

	err = auth.Authorize(context.Background(), "gr1", "viewer-1", AccessWrite)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `role "viewer" cannot write`)

	err = auth.Authorize(context.Background(), "gr1", "missing", AccessRead)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a workspace member")
}

func TestRoleAuthorizer_Validation(t *testing.T) {
	auth := NewRoleAuthorizer(nil)

	err := auth.Authorize(context.Background(), "gr1", "owner-1", AccessRead)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace store is nil")

	db, dbErr := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, dbErr)
	defer db.Close()

	store, dbErr := storage.NewWorkspaces(context.Background(), db)
	require.NoError(t, dbErr)
	auth = NewRoleAuthorizer(store)

	err = auth.Authorize(context.Background(), "", "owner-1", AccessRead)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace id is required")

	err = auth.Authorize(context.Background(), "gr1", "", AccessRead)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user id is required")
}
