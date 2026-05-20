package engine

import (
	"context"
	"fmt"
	"github.com/go-pkgz/testutils/containers"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestConverter_GetTableColumns(t *testing.T) {
	ctx := context.Background()

	tmp, err := os.CreateTemp("", "test-columns-*.db")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	db, err := NewSqlite(tmp.Name(), "test")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE column_test (
		id INTEGER PRIMARY KEY,
		text_col TEXT,
		int_col INTEGER,
		real_col REAL,
		bool_col BOOLEAN,
		blob_col BLOB,
		date_col DATETIME
	)`)
	require.NoError(t, err)

	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	converter := NewConverter(db)
	columns, err := converter.getTableColumns(ctx, tx, "column_test")
	require.NoError(t, err)

	expected := []string{"id", "text_col", "int_col", "real_col", "bool_col", "blob_col", "date_col"}
	assert.Equal(t, expected, columns)
}

func TestConverter_ConvertTableSchema_EdgeCases(t *testing.T) {
	converter := NewConverter(&SQL{})

	tests := []struct {
		name       string
		tableName  string
		sqliteStmt string
		expected   string
	}{
		{
			name:       "Multiple INTEGER columns",
			tableName:  "detected_spam",
			sqliteStmt: "CREATE TABLE detected_spam (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER, other_id INTEGER)",
			expected:   "CREATE TABLE detected_spam (id SERIAL PRIMARY KEY, user_id BIGINT, other_id INTEGER)",
		},
		{
			name:       "Multiple BOOLEAN columns",
			tableName:  "test_bools",
			sqliteStmt: "CREATE TABLE test_bools (id INTEGER PRIMARY KEY AUTOINCREMENT, flag1 BOOLEAN DEFAULT 0, flag2 BOOLEAN DEFAULT 1)",
			expected:   "CREATE TABLE test_bools (id SERIAL PRIMARY KEY, flag1 BOOLEAN DEFAULT false, flag2 BOOLEAN DEFAULT true)",
		},
		{
			name:       "Message column in non-samples table",
			tableName:  "other_table",
			sqliteStmt: "CREATE TABLE other_table (id INTEGER PRIMARY KEY AUTOINCREMENT, message TEXT, UNIQUE(message))",
			expected:   "CREATE TABLE other_table (id SERIAL PRIMARY KEY, message TEXT, UNIQUE(message))",
		},
		{
			name:       "Multiple constraints in samples table",
			tableName:  "samples",
			sqliteStmt: "CREATE TABLE samples (id INTEGER PRIMARY KEY AUTOINCREMENT, gid TEXT, message TEXT, type TEXT CHECK(type IN ('spam','ham')), UNIQUE(gid, message))",
			expected:   "CREATE TABLE samples (id SERIAL PRIMARY KEY, gid TEXT, message TEXT, type TEXT CHECK(type IN ('spam','ham')), message_hash TEXT GENERATED ALWAYS AS (encode(sha256(message::bytea), 'hex')) STORED,\n            UNIQUE(gid, message_hash))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := converter.convertTableSchema(tt.tableName, tt.sqliteStmt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConverter_ConvertIndexDefinition_EdgeCases(t *testing.T) {
	converter := NewConverter(&SQL{})

	tests := []struct {
		name       string
		tableName  string
		sqliteStmt string
		expected   string
	}{
		{
			name:       "Index with IF NOT EXISTS",
			tableName:  "test",
			sqliteStmt: "CREATE INDEX IF NOT EXISTS idx_test_name ON test (name)",
			expected:   "CREATE INDEX  idx_test_name ON test (name)",
		},
		{
			name:       "Samples message index",
			tableName:  "samples",
			sqliteStmt: "CREATE INDEX idx_samples_message ON samples(message)",
			expected:   "CREATE INDEX idx_samples_message ON samples(message_hash)",
		},
		{
			name:       "Index without message in samples table",
			tableName:  "samples",
			sqliteStmt: "CREATE INDEX idx_samples_type ON samples(type)",
			expected:   "CREATE INDEX idx_samples_type ON samples(type)",
		},
		{
			name:       "Message index in non-samples table",
			tableName:  "other_table",
			sqliteStmt: "CREATE INDEX idx_other_message ON other_table(message)",
			expected:   "CREATE INDEX idx_other_message ON other_table(message)",
		},
		{
			name:       "Composite index with message first",
			tableName:  "samples",
			sqliteStmt: "CREATE INDEX idx_samples_composite1 ON samples(message, type, origin)",
			expected:   "CREATE INDEX idx_samples_composite1 ON samples(message_hash, type, origin)",
		},
		{
			name:       "Composite index with message in middle",
			tableName:  "samples",
			sqliteStmt: "CREATE INDEX idx_samples_composite2 ON samples(type, message, origin)",
			expected:   "CREATE INDEX idx_samples_composite2 ON samples(type, message_hash, origin)",
		},
		{
			name:       "Composite index with message at end",
			tableName:  "samples",
			sqliteStmt: "CREATE INDEX idx_samples_composite3 ON samples(type, origin, message)",
			expected:   "CREATE INDEX idx_samples_composite3 ON samples(type, origin, message_hash)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := converter.convertIndexDefinition(tt.tableName, tt.sqliteStmt)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// setupPostgresContainer starts a PostgreSQL test container and returns the connection string
func setupPostgresContainer(ctx context.Context, t *testing.T) *containers.PostgresTestContainer {
	t.Log("Starting PostgreSQL test container...")
	postgresContainer := containers.NewPostgresTestContainerWithDB(ctx, t, "tg_spam_test")
	return postgresContainer
}

// createTestSqliteDatabase creates a test SQLite database with all tables used in tg-spam
func createTestSqliteDatabase(t *testing.T) (db *SQL, path string) {

	tmp, err := os.CreateTemp("", "test-convert-integration-*.db")
	require.NoError(t, err)
	sqlitePath := tmp.Name()
	tmp.Close()

	db, err = NewSqlite(sqlitePath, "test")
	require.NoError(t, err)

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

	insertData := []struct {
		gid      string
		text     string
		userID   int64
		userName string
		added    bool
		checks   string
	}{
		{"test", "Test spam message 1", 12345, "user1", false, `[{"name":"test1","spam":true,"details":"test details 1"}]`},
		{"test", "Test spam message 2", 67890, "user2", true, `[{"name":"test2","spam":true,"details":"test details 2"}]`},
		{"test", "Message with special chars: \"\t\n\r'", 99999, "user3", false, `[{"name":"test3","spam":true,"details":"test details 3"}]`},
	}

	for _, d := range insertData {
		addedVal := 0
		if d.added {
			addedVal = 1
		}
		_, err = db.Exec(`INSERT INTO detected_spam (gid, text, user_id, user_name, added, checks) 
			VALUES (?, ?, ?, ?, ?, ?)`,
			d.gid, d.text, d.userID, d.userName, addedVal, d.checks)
		require.NoError(t, err)
	}

	approvedUsers := []struct {
		uid  string
		gid  string
		name string
	}{
		{"user123", "test", "Test User 1"},
		{"user456", "test", "Test User 2"},
		{"user789", "test", "Test User with ' special\" chars"},
	}

	for _, u := range approvedUsers {
		_, err = db.Exec(`INSERT INTO approved_users (uid, gid, name) VALUES (?, ?, ?)`,
			u.uid, u.gid, u.name)
		require.NoError(t, err)
	}

	samples := []struct {
		gid       string
		sampleTyp string
		origin    string
		message   string
	}{
		{"test", "spam", "user", "Sample spam message 1"},
		{"test", "ham", "preset", "Sample ham message 1"},
		{"test", "spam", "preset", "Sample spam with \"quotes\" and 'apostrophes'"},
		{"test", "ham", "user", "Sample ham with special chars \t\n\r"},
	}

	for _, s := range samples {
		_, err = db.Exec(`INSERT INTO samples (gid, type, origin, message) VALUES (?, ?, ?, ?)`,
			s.gid, s.sampleTyp, s.origin, s.message)
		require.NoError(t, err)
	}

	dictEntries := []struct {
		gid     string
		dictTyp string
		data    string
	}{
		{"test", "stop_phrase", "spam phrase 1"},
		{"test", "ignored_word", "ignore1"},
		{"test", "stop_phrase", "spam phrase with \"quotes\" and 'apostrophes'"},
		{"test", "ignored_word", "ignore with special chars \t\n\r"},
	}

	for _, d := range dictEntries {
		_, err = db.Exec(`INSERT INTO dictionary (gid, type, data) VALUES (?, ?, ?)`,
			d.gid, d.dictTyp, d.data)
		require.NoError(t, err)
	}

	_, err = db.Exec(`CREATE INDEX idx_detected_spam_gid ON detected_spam (gid)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE INDEX idx_approved_users_gid ON approved_users (gid)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE INDEX idx_samples_message ON samples(message)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE INDEX idx_dictionary_phrase ON dictionary(data)`)
	require.NoError(t, err)

	return db, sqlitePath
}

// verifyPostgresData verifies that the PostgreSQL database has the correct data after conversion
func verifyPostgresData(ctx context.Context, t *testing.T, pgConn *sqlx.DB) {

	tables := []string{"detected_spam", "approved_users", "samples", "dictionary"}
	for _, table := range tables {
		var count int
		err := pgConn.GetContext(ctx, &count, fmt.Sprintf("SELECT COUNT(*) FROM pg_tables WHERE tablename = '%s'", table))
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Table %s should exist", table)
	}

	// 2. Verify data in detected_spam
	var spamCount int
	err := pgConn.GetContext(ctx, &spamCount, "SELECT COUNT(*) FROM detected_spam")
	require.NoError(t, err)
	assert.Equal(t, 3, spamCount, "Should have 3 rows in detected_spam")

	// check specific detected_spam record
	var spamRecord struct {
		ID       int64  `db:"id"`
		GID      string `db:"gid"`
		Text     string `db:"text"`
		UserID   int64  `db:"user_id"`
		UserName string `db:"user_name"`
		Added    bool   `db:"added"`
		Checks   string `db:"checks"`
	}
	err = pgConn.GetContext(ctx, &spamRecord, "SELECT id, gid, text, user_id, user_name, added, checks FROM detected_spam WHERE user_name = 'user1'")
	require.NoError(t, err)
	assert.Equal(t, "test", spamRecord.GID)
	assert.Equal(t, "Test spam message 1", spamRecord.Text)
	assert.Equal(t, int64(12345), spamRecord.UserID)
	assert.Equal(t, "user1", spamRecord.UserName)
	assert.False(t, spamRecord.Added)
	assert.Contains(t, spamRecord.Checks, "test1")

	// 3. Verify data in approved_users
	var userCount int
	err = pgConn.GetContext(ctx, &userCount, "SELECT COUNT(*) FROM approved_users")
	require.NoError(t, err)
	assert.Equal(t, 3, userCount, "Should have 3 rows in approved_users")

	// check specific approved_users record
	var userRecord struct {
		ID   int64  `db:"id"`
		UID  string `db:"uid"`
		GID  string `db:"gid"`
		Name string `db:"name"`
	}
	err = pgConn.GetContext(ctx, &userRecord, "SELECT id, uid, gid, name FROM approved_users WHERE uid = 'user123'")
	require.NoError(t, err)
	assert.Equal(t, "user123", userRecord.UID)
	assert.Equal(t, "test", userRecord.GID)
	assert.Equal(t, "Test User 1", userRecord.Name)

	// verify approved_users schema
	var userTableCols []string
	err = pgConn.SelectContext(ctx, &userTableCols, "SELECT column_name FROM information_schema.columns WHERE table_name = 'approved_users' ORDER BY ordinal_position")
	require.NoError(t, err)
	assert.Contains(t, userTableCols, "id")
	assert.Contains(t, userTableCols, "uid")
	assert.Contains(t, userTableCols, "gid")
	assert.Contains(t, userTableCols, "name")
	assert.Contains(t, userTableCols, "timestamp")

	// 4. Verify data in samples
	var sampleCount int
	err = pgConn.GetContext(ctx, &sampleCount, "SELECT COUNT(*) FROM samples")
	require.NoError(t, err)
	assert.Equal(t, 4, sampleCount, "Should have 4 rows in samples")

	// check specific samples record
	var sampleRecord struct {
		ID          int64  `db:"id"`
		GID         string `db:"gid"`
		Type        string `db:"type"`
		Origin      string `db:"origin"`
		Message     string `db:"message"`
		MessageHash string `db:"message_hash"`
	}
	err = pgConn.GetContext(ctx, &sampleRecord, "SELECT id, gid, type, origin, message, message_hash FROM samples WHERE message = 'Sample spam message 1'")
	require.NoError(t, err)
	assert.Equal(t, "test", sampleRecord.GID)
	assert.Equal(t, "spam", sampleRecord.Type)
	assert.Equal(t, "user", sampleRecord.Origin)
	assert.NotEmpty(t, sampleRecord.MessageHash, "message_hash should be generated")

	// verify samples schema
	var sampleTableCols []string
	err = pgConn.SelectContext(ctx, &sampleTableCols, "SELECT column_name FROM information_schema.columns WHERE table_name = 'samples' ORDER BY ordinal_position")
	require.NoError(t, err)
	assert.Contains(t, sampleTableCols, "id")
	assert.Contains(t, sampleTableCols, "gid")
	assert.Contains(t, sampleTableCols, "timestamp")
	assert.Contains(t, sampleTableCols, "type")
	assert.Contains(t, sampleTableCols, "origin")
	assert.Contains(t, sampleTableCols, "message")
	assert.Contains(t, sampleTableCols, "message_hash")

	// 5. Verify data in dictionary
	var dictCount int
	err = pgConn.GetContext(ctx, &dictCount, "SELECT COUNT(*) FROM dictionary")
	require.NoError(t, err)
	assert.Equal(t, 4, dictCount, "Should have 4 rows in dictionary")

	// check specific dictionary record
	var dictRecord struct {
		ID   int64  `db:"id"`
		GID  string `db:"gid"`
		Type string `db:"type"`
		Data string `db:"data"`
	}
	err = pgConn.GetContext(ctx, &dictRecord, "SELECT id, gid, type, data FROM dictionary WHERE type = 'stop_phrase' AND data = 'spam phrase 1'")
	require.NoError(t, err)
	assert.Equal(t, "test", dictRecord.GID)
	assert.Equal(t, "stop_phrase", dictRecord.Type)

	// verify dictionary schema
	var dictTableCols []string
	err = pgConn.SelectContext(ctx, &dictTableCols, "SELECT column_name FROM information_schema.columns WHERE table_name = 'dictionary' ORDER BY ordinal_position")
	require.NoError(t, err)
	assert.Contains(t, dictTableCols, "id")
	assert.Contains(t, dictTableCols, "gid")
	assert.Contains(t, dictTableCols, "timestamp")
	assert.Contains(t, dictTableCols, "type")
	assert.Contains(t, dictTableCols, "data")

	// 6. Verify special character handling
	var specialCount int
	err = pgConn.GetContext(ctx, &specialCount, "SELECT COUNT(*) FROM detected_spam WHERE text LIKE '%special chars%'")
	require.NoError(t, err)
	assert.Equal(t, 1, specialCount, "Should have 1 row with special characters")

	// 7. Verify indices
	var indexCount int
	err = pgConn.GetContext(ctx, &indexCount, "SELECT COUNT(*) FROM pg_indexes WHERE tablename IN ('detected_spam', 'approved_users', 'samples', 'dictionary')")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, indexCount, 4, "Should have at least 4 indices")

	// 8. Verify samples message_hash index
	// we know there should be 2 because we have the unique constraint that also contains message_hash
	var sampleHashCount int
	err = pgConn.GetContext(ctx, &sampleHashCount, "SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'samples' AND indexdef LIKE '%message_hash%'")
	require.NoError(t, err)
	assert.Equal(t, 2, sampleHashCount, "Should have 2 indices with message_hash (unique constraint and explicit index)")
}
