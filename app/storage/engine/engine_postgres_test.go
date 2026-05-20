package engine

import (
	"context"
	"fmt"
	"github.com/go-pkgz/testutils/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestNewPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx := context.Background()

	t.Log("starting postgres container")
	pgContainer := containers.NewPostgresTestContainerWithDB(ctx, t, "postgres")
	defer pgContainer.Close(ctx)
	t.Log("postgres container started")

	connStr := pgContainer.ConnectionString()

	// parse the connection string to extract the host and port
	// example connection string: postgres://postgres:secret@localhost:32768/postgres?sslmode=disable
	var host, port string
	parts := strings.Split(connStr, "@")
	if len(parts) > 1 {
		hostPortParts := strings.Split(parts[1], "/")
		if len(hostPortParts) > 0 {
			hostPort := strings.Split(hostPortParts[0], ":")
			if len(hostPort) > 1 {
				host = hostPort[0]
				port = hostPort[1]
			}
		}
	}

	tests := []struct {
		name    string
		connStr string
		wantErr string
	}{
		{
			name:    "create new database",
			connStr: fmt.Sprintf("postgres://postgres:secret@%s:%s/test_db1?sslmode=disable", host, port),
		},
		{
			name:    "connect to existing database",
			connStr: fmt.Sprintf("postgres://postgres:secret@%s:%s/test_db1?sslmode=disable", host, port),
		},
		{
			name:    "invalid url",
			connStr: "postgres://invalid::url",
			wantErr: "invalid postgres connection url",
		},
		{
			name:    "empty database name",
			connStr: fmt.Sprintf("postgres://postgres:secret@%s:%s/?sslmode=disable", host, port),
			wantErr: "database name not specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := NewPostgres(ctx, tt.connStr, "test")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			defer db.Close()

			// verify we can execute queries
			var result int
			err = db.Get(&result, "SELECT 1")
			require.NoError(t, err)
			assert.Equal(t, 1, result)
		})
	}
}
