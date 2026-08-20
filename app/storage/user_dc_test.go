package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/storage/engine"
)

// TestUserDC_Store exercises the user_dc table migration and the Set/Get methods
// on sqlite. It is a standalone test (not part of the postgres-backed
// StorageTestSuite) so it runs without Docker.
func TestUserDC_Store(t *testing.T) {
	f, err := os.CreateTemp("", "userdc")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	db, err := engine.NewSqlite(f.Name(), "gr1")
	require.NoError(t, err)

	loc, err := NewLocator(context.Background(), 10*time.Minute, 100, db)
	require.NoError(t, err)

	ctx := context.Background()

	// not found before any write
	dc, ok := loc.GetUserDC(ctx, 42)
	assert.False(t, ok)
	assert.Equal(t, 0, dc)

	// insert
	require.NoError(t, loc.SetUserDC(ctx, 42, 2))
	dc, ok = loc.GetUserDC(ctx, 42)
	require.True(t, ok)
	assert.Equal(t, 2, dc)

	// upsert (same user, different dc)
	require.NoError(t, loc.SetUserDC(ctx, 42, 4))
	dc, ok = loc.GetUserDC(ctx, 42)
	require.True(t, ok)
	assert.Equal(t, 4, dc)

	// second user independent
	require.NoError(t, loc.SetUserDC(ctx, 7, 1))
	dc, ok = loc.GetUserDC(ctx, 7)
	require.True(t, ok)
	assert.Equal(t, 1, dc)
	dc, ok = loc.GetUserDC(ctx, 42)
	require.True(t, ok)
	assert.Equal(t, 4, dc, "first user unaffected by second insert")
}
