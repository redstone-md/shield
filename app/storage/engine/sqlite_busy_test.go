package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSqliteDSN(t *testing.T) {
	tests := []struct{ name, path, want string }{
		{name: "plain path", path: "/tmp/a.db", want: "file:/tmp/a.db?_time_format=sqlite&_pragma=busy_timeout(5000)"},
		{name: "memory", path: ":memory:", want: "file::memory:?_time_format=sqlite&_pragma=busy_timeout(5000)"},
		{name: "uri with query", path: "file:a.db?cache=shared", want: "file:a.db?cache=shared&_time_format=sqlite&_pragma=busy_timeout(5000)"},
		{
			name: "own options are kept", path: "file:a.db?_time_format=x&_pragma=busy_timeout(1)",
			want: "file:a.db?_time_format=x&_pragma=busy_timeout(1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sqliteDSN(tt.path))
		})
	}
}

// a write on another pooled connection has to wait for the lock instead of failing with SQLITE_BUSY.
func TestSqliteWriteWaitsForLock(t *testing.T) {
	ctx := context.Background()
	db, err := NewSqlite(filepath.Join(t.TempDir(), "busy.db"), "gr1")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.ExecContext(ctx, "CREATE TABLE t (id INTEGER)")
	require.NoError(t, err)

	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "INSERT INTO t VALUES (1)")
	require.NoError(t, err)

	written := make(chan error, 1)
	go func() {
		_, execErr := db.ExecContext(ctx, "INSERT INTO t VALUES (2)")
		written <- execErr
	}()

	time.Sleep(200 * time.Millisecond)
	require.NoError(t, tx.Commit())

	select {
	case err = <-written:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("write did not finish")
	}
}
