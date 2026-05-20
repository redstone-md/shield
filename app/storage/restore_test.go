package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/storage/engine"
)

func TestRestoreService_RestoreTenant(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test_restore.db")
	db, err := engine.NewSqlite(dbFile, "gr1")
	require.NoError(t, err)
	defer db.Close()

	tenants, err := NewTenants(context.Background(), db)
	require.NoError(t, err)
	require.NoError(t, tenants.Add(context.Background(), TenantRecord{
		ID: "t1", Name: "original", Status: "active", OwnerID: "owner1",
	}))

	svc := NewRestoreService(db)

	dump := strings.NewReader(`
-- backup
INSERT OR REPLACE INTO tenants (id, gid, tenant_id, name, status, owner_id, created_at, updated_at) VALUES ('t1', 'gr1', 'gr1', 'restored', 'active', 'owner1', '2025-01-01 00:00:00', '2025-01-01 00:00:00');
INSERT OR REPLACE INTO tenants (id, gid, tenant_id, name, status, owner_id, created_at, updated_at) VALUES ('t2', 'gr2', 'gr2', 'other', 'active', 'owner2', '2025-01-01 00:00:00', '2025-01-01 00:00:00');
`)

	err = svc.RestoreTenant(context.Background(), "t1", dump)
	require.NoError(t, err)
}

func TestRestoreService_EmptyTenantID(t *testing.T) {
	svc := NewRestoreService(nil)
	err := svc.RestoreTenant(context.Background(), "", strings.NewReader("INSERT INTO foo VALUES (1);"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant_id is required")
}

func TestRestoreService_NilDB(t *testing.T) {
	svc := NewRestoreService(nil)
	err := svc.RestoreTenant(context.Background(), "t1", strings.NewReader("INSERT INTO foo VALUES (1);"))
	require.Error(t, err)
}

func TestRestoreService_ShouldIncludeStatement(t *testing.T) {
	svc := &RestoreService{}

	assert.False(t, svc.shouldIncludeStatement("CREATE TABLE foo (id INT);", "t1"))
	assert.False(t, svc.shouldIncludeStatement("CREATE INDEX idx ON foo(id);", "t1"))
	assert.False(t, svc.shouldIncludeStatement("DROP TABLE foo;", "t1"))
	assert.True(t, svc.shouldIncludeStatement("INSERT INTO tenants (id, tenant_id) VALUES ('t1', 't1');", "t1"))
	assert.False(t, svc.shouldIncludeStatement("INSERT INTO foo (tenant_id, name) VALUES ('t2', 'other');", "t1"))
	assert.True(t, svc.shouldIncludeStatement("INSERT INTO foo (tenant_id, name) VALUES ('t1', 'mine');", "t1"))
	assert.False(t, svc.shouldIncludeStatement("INSERT INTO foo (id, name) VALUES (1, 'nontenant');", "t1"))
}

func TestRestoreService_IsTableNotExistErr(t *testing.T) {
	assert.True(t, isTableNotExistErr(fmt.Errorf("no such table: foo")))
	assert.True(t, isTableNotExistErr(fmt.Errorf("relation does not exist")))
	assert.False(t, isTableNotExistErr(fmt.Errorf("connection refused")))
}
