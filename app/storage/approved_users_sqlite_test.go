package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/storage/engine"
	"github.com/redstone-md/shield/lib/approved"
)

// verifies Delete is idempotent: removing a user that was never approved (or
// already removed) succeeds instead of surfacing sql.ErrNoRows to callers.
func TestApprovedUsers_Delete_MissingUserIsNoop(t *testing.T) {
	ctx := context.Background()
	db, err := engine.NewSqlite(t.TempDir()+"/approved.db", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewApprovedUsers(ctx, db)
	require.NoError(t, err)

	// never-approved id: no error
	require.NoError(t, store.Delete(ctx, "111"))

	require.NoError(t, store.Write(ctx, approved.UserInfo{UserID: "222", UserName: "bob", Timestamp: time.Now()}))

	// existing id deletes fine
	require.NoError(t, store.Delete(ctx, "222"))
	users, err := store.Read(ctx)
	require.NoError(t, err)
	require.Empty(t, users)

	// second delete of the same id is still a no-op success
	require.NoError(t, store.Delete(ctx, "222"))
}
