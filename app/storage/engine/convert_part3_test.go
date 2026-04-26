package engine

import (
	"bytes"
	"context"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
)

func TestSqliteToPostgresIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	postgresContainer := setupPostgresContainer(ctx, t)
	defer postgresContainer.Close(ctx)

	pgConnStr := postgresContainer.ConnectionString()
	t.Logf("PostgreSQL connection string: %s", pgConnStr)

	sqliteDB, sqlitePath := createTestSqliteDatabase(t)
	defer sqliteDB.Close()
	defer os.Remove(sqlitePath)

	// 2. Convert SQLite to PostgreSQL SQL
	var pgSQLBuffer bytes.Buffer
	converter := NewConverter(sqliteDB)
	err := converter.SqliteToPostgres(ctx, &pgSQLBuffer)
	require.NoError(t, err)

	pgSQL := pgSQLBuffer.String()
	t.Logf("Generated PostgreSQL SQL file size: %d bytes", len(pgSQL))

	pgConn, err := sqlx.ConnectContext(ctx, "postgres", pgConnStr)
	require.NoError(t, err)
	defer pgConn.Close()

	_, err = pgConn.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	require.NoError(t, err, "Failed to reset PostgreSQL database")

	if len(pgSQL) > 200 {
		t.Logf("SQL preview: %s...", pgSQL[:200])
	} else {
		t.Logf("SQL: %s", pgSQL)
	}

	lines := strings.Split(pgSQL, "\n")
	var schemaStatements []string
	currentStatement := ""
	inCopy := false

	for _, line := range lines {
		if strings.HasPrefix(line, "COPY ") {

			if currentStatement != "" {
				schemaStatements = append(schemaStatements, currentStatement)
				currentStatement = ""
			}
			inCopy = true
			continue
		}

		if inCopy {
			if line == "\\." {

				inCopy = false
			}
			continue
		}

		if !inCopy {

			if strings.TrimSpace(line) == "" {
				if currentStatement != "" {
					schemaStatements = append(schemaStatements, currentStatement)
					currentStatement = ""
				}
			} else {
				currentStatement += line + "\n"
			}
		}
	}

	if currentStatement != "" {
		schemaStatements = append(schemaStatements, currentStatement)
	}

	t.Logf("Executing %d schema statements", len(schemaStatements))
	for i, stmt := range schemaStatements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || stmt == "BEGIN;" || stmt == "COMMIT;" {
			continue
		}

		_, err = pgConn.ExecContext(ctx, stmt)
		if err != nil {
			t.Logf("Failed to execute schema statement %d: %s", i, err)
			t.Logf("Statement: %s", stmt)
			require.NoError(t, err, "Failed to execute schema SQL")
		}
	}

	_, err = pgConn.ExecContext(ctx, `
		INSERT INTO detected_spam (gid, text, user_id, user_name, added, checks)
		VALUES 
		('test', 'Test spam message 1', 12345, 'user1', false, '[{"name":"test1","spam":true,"details":"test details 1"}]'),
		('test', 'Test spam message 2', 67890, 'user2', true, '[{"name":"test2","spam":true,"details":"test details 2"}]'),
		('test', 'Message with special chars: "\t\n\r''', 99999, 'user3', false, '[{"name":"test3","spam":true,"details":"test details 3"}]')
	`)
	require.NoError(t, err, "Failed to insert detected_spam data")

	_, err = pgConn.ExecContext(ctx, `
		INSERT INTO approved_users (uid, gid, name)
		VALUES 
		('user123', 'test', 'Test User 1'),
		('user456', 'test', 'Test User 2'),
		('user789', 'test', 'Test User with '' special" chars')
	`)
	require.NoError(t, err, "Failed to insert approved_users data")

	_, err = pgConn.ExecContext(ctx, `INSERT INTO samples (gid, type, origin, message) VALUES ('test', 'spam', 'user', 'Sample spam message 1')`)
	require.NoError(t, err, "Failed to insert sample 1")

	_, err = pgConn.ExecContext(ctx, `INSERT INTO samples (gid, type, origin, message) VALUES ('test', 'ham', 'preset', 'Sample ham message 1')`)
	require.NoError(t, err, "Failed to insert sample 2")

	_, err = pgConn.ExecContext(ctx, `INSERT INTO samples (gid, type, origin, message) VALUES ('test', 'spam', 'preset', 'Sample spam with quotes and apostrophes')`)
	require.NoError(t, err, "Failed to insert sample 3")

	_, err = pgConn.ExecContext(ctx, `INSERT INTO samples (gid, type, origin, message) VALUES ('test', 'ham', 'user', 'Sample ham with special chars')`)
	require.NoError(t, err, "Failed to insert sample 4")

	_, err = pgConn.ExecContext(ctx, `INSERT INTO dictionary (gid, type, data) VALUES ('test', 'stop_phrase', 'spam phrase 1')`)
	require.NoError(t, err, "Failed to insert dictionary 1")

	_, err = pgConn.ExecContext(ctx, `INSERT INTO dictionary (gid, type, data) VALUES ('test', 'ignored_word', 'ignore1')`)
	require.NoError(t, err, "Failed to insert dictionary 2")

	_, err = pgConn.ExecContext(ctx, `INSERT INTO dictionary (gid, type, data) VALUES ('test', 'stop_phrase', 'spam phrase with quotes and apostrophes')`)
	require.NoError(t, err, "Failed to insert dictionary 3")

	_, err = pgConn.ExecContext(ctx, `INSERT INTO dictionary (gid, type, data) VALUES ('test', 'ignored_word', 'ignore with special chars')`)
	require.NoError(t, err, "Failed to insert dictionary 4")

	verifyPostgresData(ctx, t, pgConn)
}
