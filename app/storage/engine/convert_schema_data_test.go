package engine

import (
	"bytes"
	"context"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
	"time"
)

func TestConverter_SqliteToPostgres(t *testing.T) {
	ctx := context.Background()

	tmp, err := os.CreateTemp("", "test-convert-*.db")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	db, err := NewSqlite(tmp.Name(), "test")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE detected_spam (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		gid TEXT NOT NULL DEFAULT '',
		text TEXT,
		user_id INTEGER,
		user_name TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		added BOOLEAN DEFAULT 0,
		checks TEXT
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE TABLE approved_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uid TEXT,
		gid TEXT DEFAULT '',
		name TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(gid, uid)
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE TABLE samples (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		gid TEXT NOT NULL DEFAULT '',
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		type TEXT CHECK (type IN ('ham', 'spam')),
		origin TEXT CHECK (origin IN ('preset', 'user')),
		message TEXT NOT NULL,
		UNIQUE(gid, message)
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE TABLE dictionary (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		gid TEXT DEFAULT '',
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		type TEXT CHECK (type IN ('stop_phrase', 'ignored_word')),
		data TEXT NOT NULL,
		UNIQUE(gid, data)
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO detected_spam (gid, text, user_id, user_name, added, checks) 
		VALUES (?, ?, ?, ?, ?, ?)`,
		"test", "Test spam message", 12345, "user1", 0, `[{"name":"test","spam":true,"details":"test details"}]`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO detected_spam (gid, text, user_id, user_name, added, checks) 
		VALUES (?, ?, ?, ?, ?, ?)`,
		"test", "Another spam message", 67890, "user2", 1, `[{"name":"test2","spam":true,"details":"test details 2"}]`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO approved_users (uid, gid, name) 
		VALUES (?, ?, ?)`,
		"user123", "test", "Test User")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO samples (gid, type, origin, message) 
		VALUES (?, ?, ?, ?)`,
		"test", "spam", "user", "Sample spam message")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO samples (gid, type, origin, message) 
		VALUES (?, ?, ?, ?)`,
		"test", "ham", "preset", "Sample ham message")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO dictionary (gid, type, data) 
		VALUES (?, ?, ?)`,
		"test", "stop_phrase", "spam phrase")
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO dictionary (gid, type, data) 
		VALUES (?, ?, ?)`,
		"test", "ignored_word", "ignore")
	require.NoError(t, err)

	_, err = db.Exec(`CREATE INDEX idx_detected_spam_gid ON detected_spam (gid)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE INDEX idx_approved_users_gid ON approved_users (gid)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE INDEX idx_samples_message ON samples(message)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE INDEX idx_dictionary_phrase ON dictionary(data)`)
	require.NoError(t, err)

	converter := NewConverter(db)

	// test conversion
	var buf bytes.Buffer
	err = converter.SqliteToPostgres(ctx, &buf)
	require.NoError(t, err)

	result := buf.String()
	t.Logf("Conversion result: %s", result)

	assert.Contains(t, result, "BEGIN;")
	assert.Contains(t, result, "COMMIT;")

	assert.Contains(t, result, "CREATE TABLE detected_spam")
	assert.Contains(t, result, "SERIAL PRIMARY KEY")
	assert.Contains(t, result, "TIMESTAMP")
	assert.Contains(t, result, "user_id BIGINT")
	assert.Contains(t, result, "added BOOLEAN DEFAULT false")

	assert.Contains(t, result, "CREATE TABLE approved_users")
	assert.Contains(t, result, "UNIQUE(gid, uid)")

	assert.Contains(t, result, "CREATE TABLE samples")
	assert.Contains(t, result, "message_hash TEXT GENERATED ALWAYS AS")
	assert.Contains(t, result, "UNIQUE(gid, message_hash)")

	assert.Contains(t, result, "CREATE TABLE dictionary")
	assert.Contains(t, result, "CHECK (type IN ('stop_phrase', 'ignored_word'))")

	assert.Contains(t, result, "CREATE INDEX idx_detected_spam_gid")
	assert.Contains(t, result, "CREATE INDEX idx_approved_users_gid")
	assert.Contains(t, result, "CREATE INDEX idx_samples_message")
	assert.Contains(t, result, "message_hash")
	assert.Contains(t, result, "CREATE INDEX idx_dictionary_phrase")

	assert.Contains(t, result, "COPY detected_spam")
	assert.Contains(t, result, "Test spam message")
	assert.Contains(t, result, "Another spam message")
	assert.Contains(t, result, "COPY approved_users")
	assert.Contains(t, result, "Test User")
	assert.Contains(t, result, "COPY samples")
	assert.Contains(t, result, "Sample spam message")
	assert.Contains(t, result, "Sample ham message")
	assert.Contains(t, result, "COPY dictionary")
	assert.Contains(t, result, "spam phrase")
	assert.Contains(t, result, "ignore")

	t.Run("boolean conversion", func(t *testing.T) {

		lines := strings.Split(result, "\n")
		foundFalse := false
		foundTrue := false

		for _, line := range lines {
			if strings.Contains(line, "Test spam message") {
				assert.Contains(t, line, "\tf\t")
				foundFalse = true
			}
			if strings.Contains(line, "Another spam message") {
				assert.Contains(t, line, "\tt\t")
				foundTrue = true
			}
		}

		assert.True(t, foundFalse, "Should convert '0' to 'f' for boolean values")
		assert.True(t, foundTrue, "Should convert '1' to 't' for boolean values")
	})
}

func TestConverter_SqliteToPostgres_NonSqliteError(t *testing.T) {
	ctx := context.Background()

	mockDB := &SQL{dbType: Postgres, gid: "test"}

	converter := NewConverter(mockDB)

	// test conversion from PostgreSQL should fail
	var buf bytes.Buffer
	err := converter.SqliteToPostgres(ctx, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source database must be SQLite")
}

func TestConverter_ConvertTableSchema(t *testing.T) {
	converter := NewConverter(&SQL{})

	tests := []struct {
		name       string
		tableName  string
		sqliteStmt string
		expected   string
	}{
		{
			name:       "Convert INTEGER PRIMARY KEY",
			tableName:  "test",
			sqliteStmt: "CREATE TABLE test (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)",
			expected:   "CREATE TABLE test (id SERIAL PRIMARY KEY, name TEXT)",
		},
		{
			name:       "Convert DATETIME",
			tableName:  "test",
			sqliteStmt: "CREATE TABLE test (created DATETIME DEFAULT CURRENT_TIMESTAMP)",
			expected:   "CREATE TABLE test (created TIMESTAMP DEFAULT CURRENT_TIMESTAMP)",
		},
		{
			name:       "Convert BLOB",
			tableName:  "test",
			sqliteStmt: "CREATE TABLE test (data BLOB)",
			expected:   "CREATE TABLE test (data BYTEA)",
		},
		{
			name:       "Convert detected_spam table",
			tableName:  "detected_spam",
			sqliteStmt: "CREATE TABLE detected_spam (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER, added BOOLEAN DEFAULT 0)",
			expected:   "CREATE TABLE detected_spam (id SERIAL PRIMARY KEY, user_id BIGINT, added BOOLEAN DEFAULT false)",
		},
		{
			name:       "Convert samples table",
			tableName:  "samples",
			sqliteStmt: "CREATE TABLE samples (id INTEGER PRIMARY KEY AUTOINCREMENT, message TEXT, UNIQUE(gid, message))",
			expected:   "CREATE TABLE samples (id SERIAL PRIMARY KEY, message TEXT, message_hash TEXT GENERATED ALWAYS AS (encode(sha256(message::bytea), 'hex')) STORED,\n            UNIQUE(gid, message_hash))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := converter.convertTableSchema(tt.tableName, tt.sqliteStmt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConverter_ConvertIndexDefinition(t *testing.T) {
	converter := NewConverter(&SQL{})

	tests := []struct {
		name       string
		tableName  string
		sqliteStmt string
		expected   string
	}{
		{
			name:       "Simple index",
			tableName:  "test",
			sqliteStmt: "CREATE INDEX IF NOT EXISTS idx_test ON test (name)",
			expected:   "CREATE INDEX  idx_test ON test (name)",
		},
		{
			name:       "Sample message index",
			tableName:  "samples",
			sqliteStmt: "CREATE INDEX idx_samples_message ON samples(message)",
			expected:   "CREATE INDEX idx_samples_message ON samples(message_hash)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := converter.convertIndexDefinition(tt.tableName, tt.sqliteStmt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConverter_FormatPostgresValue(t *testing.T) {
	converter := NewConverter(&SQL{})

	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{
			name:     "Format nil",
			value:    nil,
			expected: "\\N",
		},
		{
			name:     "Format string",
			value:    "test",
			expected: "test",
		},
		{
			name:     "Format string with escapes",
			value:    "test\nline\twith\rspecial\\chars",
			expected: "test\\nline\\twith\\rspecial\\\\chars",
		},
		{
			name:     "Format bytes",
			value:    []byte("test\ndata"),
			expected: "test\\ndata",
		},
		{
			name:     "Format bool true",
			value:    true,
			expected: "t",
		},
		{
			name:     "Format bool false",
			value:    false,
			expected: "f",
		},
		{
			name:     "Format number",
			value:    42,
			expected: "42",
		},
		{
			name:     "Format time.Time",
			value:    time.Date(2023, 5, 15, 10, 30, 0, 0, time.UTC),
			expected: "2023-05-15 10:30:00",
		},
		{
			name:     "Format tab and newline characters in string",
			value:    "multi\nline\ttext",
			expected: "multi\\nline\\ttext",
		},
		{
			name:     "Format empty string",
			value:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := converter.formatPostgresValue(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConverter_ExportTableData_Empty(t *testing.T) {
	ctx := context.Background()

	tmp, err := os.CreateTemp("", "test-empty-table-*.db")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	db, err := NewSqlite(tmp.Name(), "test")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE empty_table (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT
	)`)
	require.NoError(t, err)

	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	columns, err := (&Converter{db: db}).getTableColumns(ctx, tx, "empty_table")
	require.NoError(t, err)
	assert.Equal(t, []string{"id", "name"}, columns)

	// test exporting empty table
	var buf bytes.Buffer
	converter := NewConverter(db)
	err = converter.exportTableData(ctx, tx, &buf, "empty_table", columns)
	require.NoError(t, err)

	assert.Empty(t, buf.String())
}

func TestConverter_ExportTableData_WithNullValues(t *testing.T) {
	ctx := context.Background()

	tmp, err := os.CreateTemp("", "test-null-values-*.db")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	db, err := NewSqlite(tmp.Name(), "test")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE null_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		text_value TEXT,
		int_value INTEGER,
		null_value TEXT
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO null_test (text_value, int_value, null_value) VALUES (?, ?, ?)`,
		"text", 42, nil)
	require.NoError(t, err)

	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	columns, err := (&Converter{db: db}).getTableColumns(ctx, tx, "null_test")
	require.NoError(t, err)

	// test exporting table with NULL values
	var buf bytes.Buffer
	converter := NewConverter(db)
	err = converter.exportTableData(ctx, tx, &buf, "null_test", columns)
	require.NoError(t, err)

	result := buf.String()
	assert.Contains(t, result, "COPY null_test")
	assert.Contains(t, result, "text")
	assert.Contains(t, result, "42")
	assert.Contains(t, result, "\\N")
}
