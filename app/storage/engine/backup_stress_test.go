package engine

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackup_EmptyDatabase(t *testing.T) {
	ctx := context.Background()

	db, err := NewSqlite(":memory:", "test_gid")
	require.NoError(t, err)
	defer db.Close()

	var buf bytes.Buffer
	err = db.Backup(ctx, &buf)
	require.NoError(t, err)

	backup := buf.String()
	assert.Contains(t, backup, "-- SQLite database backup")
	assert.Contains(t, backup, "-- GID: test_gid")
	assert.NotContains(t, backup, "CREATE TABLE")
	assert.NotContains(t, backup, "INSERT INTO")
}

func TestBackup_SpecialCharacters(t *testing.T) {
	ctx := context.Background()

	db, err := NewSqlite(":memory:", "gid1")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE special_chars (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		gid TEXT NOT NULL,
		name TEXT NOT NULL,
		value TEXT
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO special_chars (gid, name, value) VALUES (?, ?, ?)`,
		"gid1", "O'Brien", "line1\nline2")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO special_chars (gid, name, value) VALUES (?, ?, ?)`,
		"gid1", "tab_name", "col1\tcol2")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = db.Backup(ctx, &buf)
	require.NoError(t, err)

	backup := buf.String()
	assert.Contains(t, backup, "O''Brien")
	assert.Contains(t, backup, "line1\nline2")
	assert.Contains(t, backup, "col1\tcol2")
	assert.Contains(t, backup, "CREATE TABLE special_chars")
}

func TestBackup_NullValues(t *testing.T) {
	ctx := context.Background()

	db, err := NewSqlite(":memory:", "gid1")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE nullable_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		gid TEXT NOT NULL,
		name TEXT NOT NULL,
		value TEXT
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO nullable_test (gid, name, value) VALUES (?, ?, ?)`,
		"gid1", "with_null", nil)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO nullable_test (gid, name, value) VALUES (?, ?, ?)`,
		"gid1", "with_value", "something")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = db.Backup(ctx, &buf)
	require.NoError(t, err)

	backup := buf.String()
	assert.Contains(t, backup, "NULL")
	assert.Contains(t, backup, "with_null")
	assert.Contains(t, backup, "something")
}

func TestBackup_Roundtrip(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	dbPath1 := filepath.Join(tmpDir, "source.db")
	db1, err := NewSqlite(dbPath1, "gr1")
	require.NoError(t, err)

	_, err = db1.Exec(`CREATE TABLE roundtrip_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		value TEXT
	)`)
	require.NoError(t, err)

	for i := range 5 {
		_, err = db1.Exec(`INSERT INTO roundtrip_test (tenant_id, name, value) VALUES (?, ?, ?)`,
			"gr1", fmt.Sprintf("name_gr1_%d", i), fmt.Sprintf("val_%d", i))
		require.NoError(t, err)
	}

	for i := range 2 {
		_, err = db1.Exec(`INSERT INTO roundtrip_test (tenant_id, name, value) VALUES (?, ?, ?)`,
			"other", fmt.Sprintf("name_other_%d", i), fmt.Sprintf("other_val_%d", i))
		require.NoError(t, err)
	}

	var buf bytes.Buffer
	err = db1.Backup(ctx, &buf)
	require.NoError(t, err)
	db1.Close()

	backup := buf.String()

	dbPath2 := filepath.Join(tmpDir, "restored.db")
	db2, err := NewSqlite(dbPath2, "gr1")
	require.NoError(t, err)
	defer db2.Close()

	lines := strings.Split(backup, "\n")
	var stmt strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		stmt.WriteString(line)
		stmt.WriteString("\n")
		if strings.HasSuffix(trimmed, ";") {
			_, err = db2.Exec(stmt.String())
			require.NoError(t, err, "failed to execute: %s", stmt.String())
			stmt.Reset()
		}
	}

	var count int
	err = db2.Get(&count, "SELECT COUNT(*) FROM roundtrip_test")
	require.NoError(t, err)
	assert.Equal(t, 5, count, "only rows for gr1 tenant should be restored")
}

func TestBackup_TablesWithoutTenantID(t *testing.T) {
	ctx := context.Background()

	db, err := NewSqlite(":memory:", "gid1")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE with_tenant (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id TEXT NOT NULL DEFAULT '',
		data TEXT NOT NULL
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO with_tenant (tenant_id, data) VALUES (?, ?)`, "gid1", "owned_data")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO with_tenant (tenant_id, data) VALUES (?, ?)`, "other", "other_data")
	require.NoError(t, err)

	_, err = db.Exec(`CREATE TABLE no_tenant (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		info TEXT NOT NULL
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO no_tenant (info) VALUES (?)`, "global_info")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = db.Backup(ctx, &buf)
	require.NoError(t, err)

	backup := buf.String()
	assert.Contains(t, backup, "CREATE TABLE with_tenant")
	assert.Contains(t, backup, "CREATE TABLE no_tenant")
	assert.Contains(t, backup, "owned_data")
	assert.NotContains(t, backup, "other_data")
	assert.Contains(t, backup, "global_info")
}
