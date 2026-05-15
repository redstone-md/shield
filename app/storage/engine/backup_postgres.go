package engine

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

func (e *SQL) backupPostgres(ctx context.Context, w io.Writer) error {
	tx, err := e.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if headerErr := e.writePostgresHeader(ctx, tx, w); headerErr != nil {
		return headerErr
	}

	tables, err := e.getPostgresTables(ctx, tx)
	if err != nil {
		return err
	}

	for _, table := range tables {
		if tableErr := e.backupPostgresTable(ctx, tx, w, table); tableErr != nil {
			return tableErr
		}
	}

	if _, writeErr := io.WriteString(w, "COMMIT;\n"); writeErr != nil {
		return fmt.Errorf("failed to write transaction commit: %w", writeErr)
	}

	return nil
}

func (e *SQL) writePostgresHeader(ctx context.Context, tx *sqlx.Tx, w io.Writer) error {
	timestamp := time.Now().Format(time.RFC3339)
	header := fmt.Sprintf("-- PostgreSQL database backup\n-- Generated: %s\n-- GID: %s\n\n", timestamp, e.gid)
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	var version string
	if err := tx.GetContext(ctx, &version, "SELECT version()"); err != nil {
		return fmt.Errorf("failed to get PostgreSQL version: %w", err)
	}
	if _, err := fmt.Fprintf(w, "-- PostgreSQL Version: %s\n\n", version); err != nil {
		return fmt.Errorf("failed to write version: %w", err)
	}

	if _, err := io.WriteString(w, "BEGIN;\n\n"); err != nil {
		return fmt.Errorf("failed to write transaction start: %w", err)
	}

	return nil
}

func (e *SQL) getPostgresTables(ctx context.Context, tx *sqlx.Tx) ([]string, error) {
	var tables []string
	query := `
SELECT tablename
FROM pg_catalog.pg_tables
WHERE schemaname != 'pg_catalog'
AND schemaname != 'information_schema'
`
	if err := tx.SelectContext(ctx, &tables, query); err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}
	return tables, nil
}

func (e *SQL) backupPostgresTable(ctx context.Context, tx *sqlx.Tx, w io.Writer, table string) error {
	if err := e.writePostgresTableSchema(ctx, tx, w, table); err != nil {
		return err
	}

	hasTenantID, columns, err := e.getPostgresTableInfo(ctx, tx, table)
	if err != nil {
		return err
	}

	if err := e.writePgTableData(ctx, tx, w, table, columns, hasTenantID); err != nil {
		if !strings.Contains(err.Error(), "no rows in result set") {
			return err
		}
	}

	if err := e.writePostgresTableIndices(ctx, tx, w, table); err != nil {
		return err
	}

	return nil
}

func (e *SQL) writePostgresTableSchema(ctx context.Context, tx *sqlx.Tx, w io.Writer, table string) error {
	var createStmt string
	query := fmt.Sprintf(`
SELECT
'CREATE TABLE ' || table_name || ' (' ||
array_to_string(
array_agg(
column_name || ' ' ||
data_type ||
CASE
WHEN character_maximum_length IS NOT NULL THEN '(' || character_maximum_length || ')'
ELSE ''
END ||
CASE
WHEN is_nullable = 'NO' THEN ' NOT NULL'
ELSE ''
END
), ', '
) || ');' as create_statement
FROM information_schema.columns
WHERE table_name = '%s'
GROUP BY table_name
`, table)
	if err := tx.GetContext(ctx, &createStmt, query); err != nil {
		return fmt.Errorf("failed to get schema for table %s: %w", table, err)
	}

	if _, err := fmt.Fprintf(w, "%s\n\n", createStmt); err != nil {
		return fmt.Errorf("failed to write schema for table %s: %w", table, err)
	}

	return nil
}

func (e *SQL) getPostgresTableInfo(
	ctx context.Context, tx *sqlx.Tx, table string,
) (hasTenantID bool, coumns []string, err error) {
	query := `
SELECT EXISTS (
SELECT 1
FROM information_schema.columns
WHERE table_name = $1
AND column_name = 'tenant_id'
)
`
	if err := tx.GetContext(ctx, &hasTenantID, query, table); err != nil {
		return false, nil, fmt.Errorf("failed to check if table %s has tenant_id column: %w", table, err)
	}

	var columns []string
	query = `
SELECT column_name
FROM information_schema.columns
WHERE table_name = $1
ORDER BY ordinal_position
`
	if err := tx.SelectContext(ctx, &columns, query, table); err != nil {
		return false, nil, fmt.Errorf("failed to get columns for table %s: %w", table, err)
	}

	return hasTenantID, columns, nil
}

func (e *SQL) writePgTableData(ctx context.Context, tx *sqlx.Tx, w io.Writer, tbl string, cols []string, hasTenantID bool) error {
	dataQuery := fmt.Sprintf("SELECT * FROM %s", tbl)
	if hasTenantID {
		dataQuery = fmt.Sprintf("SELECT * FROM %s WHERE tenant_id = $1", tbl)
	}

	var count int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM (%s) as subq", dataQuery)
	var err error
	if hasTenantID {
		err = tx.GetContext(ctx, &count, countQuery, e.TenantID())
	} else {
		err = tx.GetContext(ctx, &count, countQuery)
	}
	if err != nil {
		return fmt.Errorf("failed to check row count for table %s: %w", tbl, err)
	}

	if count == 0 {
		return nil
	}

	if _, commentErr := fmt.Fprintf(w, "-- Data for table %s\n", tbl); commentErr != nil {
		return fmt.Errorf("failed to write comment: %w", commentErr)
	}

	if _, copyErr := fmt.Fprintf(w, "COPY %s (%s) FROM stdin;\n", tbl, strings.Join(cols, ", ")); copyErr != nil {
		return fmt.Errorf("failed to write COPY statement: %w", copyErr)
	}

	var rows *sqlx.Rows
	if hasTenantID {
		rows, err = tx.QueryxContext(ctx, dataQuery, e.TenantID())
	} else {
		rows, err = tx.QueryxContext(ctx, dataQuery)
	}
	if err != nil {
		return fmt.Errorf("failed to query data from table %s: %w", tbl, err)
	}

	defer func() {
		closeErr := rows.Close()
		if closeErr != nil {
			log.Printf("[WARN] failed to close rows: %v", closeErr)
		}
	}()

	for rows.Next() {
		if writeErr := e.writePostgresRow(w, rows, cols); writeErr != nil {
			return writeErr
		}
	}

	if rowErr := rows.Err(); rowErr != nil {
		return fmt.Errorf("error iterating rows: %w", rowErr)
	}

	if _, err := io.WriteString(w, "\\.\n\n"); err != nil {
		return fmt.Errorf("failed to write COPY end: %w", err)
	}

	return nil
}

func (e *SQL) writePostgresRow(w io.Writer, rows *sqlx.Rows, columns []string) error {
	row := make(map[string]any)
	if err := rows.MapScan(row); err != nil {
		return fmt.Errorf("failed to scan row: %w", err)
	}

	values := make([]string, 0, len(columns))
	for _, col := range columns {
		val, ok := row[col]
		if !ok {
			values = append(values, "\\N")
			continue
		}
		values = append(values, e.formatPostgresValue(val))
	}

	if _, err := fmt.Fprintf(w, "%s\n", strings.Join(values, "\t")); err != nil {
		return fmt.Errorf("failed to write data row: %w", err)
	}

	return nil
}

func (e *SQL) formatPostgresValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "\\N"
	case []byte:
		s := string(v)
		s = strings.ReplaceAll(s, "\\", "\\\\")
		s = strings.ReplaceAll(s, "\t", "\\t")
		s = strings.ReplaceAll(s, "\n", "\\n")
		s = strings.ReplaceAll(s, "\r", "\\r")
		return s
	case string:
		s := v
		s = strings.ReplaceAll(s, "\\", "\\\\")
		s = strings.ReplaceAll(s, "\t", "\\t")
		s = strings.ReplaceAll(s, "\n", "\\n")
		s = strings.ReplaceAll(s, "\r", "\\r")
		return s
	case time.Time:
		return v.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (e *SQL) writePostgresTableIndices(ctx context.Context, tx *sqlx.Tx, w io.Writer, table string) error {
	var indices []struct {
		Name       string `db:"indexname"`
		Definition string `db:"indexdef"`
	}
	query := `
SELECT indexname, indexdef
FROM pg_indexes
WHERE tablename = $1
AND indexname NOT LIKE '%_pkey'
`
	if err := tx.SelectContext(ctx, &indices, query, table); err != nil {
		return fmt.Errorf("failed to get indices for table %s: %w", table, err)
	}

	for _, idx := range indices {
		if _, err := fmt.Fprintf(w, "%s;\n", idx.Definition); err != nil {
			return fmt.Errorf("failed to write index: %w", err)
		}
	}

	if len(indices) > 0 {
		if _, err := io.WriteString(w, "\n"); err != nil {
			return fmt.Errorf("failed to write newline: %w", err)
		}
	}

	return nil
}
