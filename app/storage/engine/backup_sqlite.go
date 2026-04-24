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

func (e *SQL) backupSqlite(ctx context.Context, w io.Writer) error {
	tx, err := e.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if headerErr := e.writeSqliteHeader(w); headerErr != nil {
		return headerErr
	}

	tables, err := e.getSqliteTables(ctx, tx)
	if err != nil {
		return err
	}

	for _, table := range tables {
		if tableErr := e.backupSqliteTable(ctx, tx, w, table); tableErr != nil {
			return tableErr
		}
	}

	return nil
}

func (e *SQL) writeSqliteHeader(w io.Writer) error {
	timestamp := time.Now().Format(time.RFC3339)
	header := fmt.Sprintf("-- SQLite database backup\n-- Generated: %s\n-- GID: %s\n\n", timestamp, e.gid)
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	return nil
}

func (e *SQL) getSqliteTables(ctx context.Context, tx *sqlx.Tx) ([]string, error) {
	var tables []string
	query := "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'"
	if err := tx.SelectContext(ctx, &tables, query); err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}
	return tables, nil
}

func (e *SQL) backupSqliteTable(ctx context.Context, tx *sqlx.Tx, w io.Writer, table string) error {
	if err := e.writeSqliteTableSchema(ctx, tx, w, table); err != nil {
		return err
	}

	if err := e.writeSqliteTableIndices(ctx, tx, w, table); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	columns, err := e.getSqliteColumns(ctx, tx, table)
	if err != nil {
		return err
	}

	if err := e.writeSqliteTableData(ctx, tx, w, table, columns); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

func (e *SQL) writeSqliteTableSchema(ctx context.Context, tx *sqlx.Tx, w io.Writer, table string) error {
	var createStmt string
	query := fmt.Sprintf("SELECT sql FROM sqlite_master WHERE type='table' AND name='%s'", table)
	if err := tx.GetContext(ctx, &createStmt, query); err != nil {
		return fmt.Errorf("failed to get schema for table %s: %w", table, err)
	}

	if _, err := fmt.Fprintf(w, "%s;\n\n", createStmt); err != nil {
		return fmt.Errorf("failed to write schema for table %s: %w", table, err)
	}
	return nil
}

func (e *SQL) writeSqliteTableIndices(ctx context.Context, tx *sqlx.Tx, w io.Writer, table string) error {
	var indices []struct {
		SQL string `db:"sql"`
	}
	query := fmt.Sprintf("SELECT sql FROM sqlite_master WHERE type='index' AND tbl_name='%s' AND sql IS NOT NULL", table)
	if err := tx.SelectContext(ctx, &indices, query); err != nil {
		return fmt.Errorf("failed to get indices for table %s: %w", table, err)
	}

	for _, idx := range indices {
		if _, err := fmt.Fprintf(w, "%s;\n", idx.SQL); err != nil {
			return fmt.Errorf("failed to write index: %w", err)
		}
	}
	return nil
}

func (e *SQL) getSqliteColumns(ctx context.Context, tx *sqlx.Tx, table string) ([]string, error) {
	var columns []string
	query := "SELECT name FROM PRAGMA_TABLE_INFO(?)"
	if err := tx.SelectContext(ctx, &columns, query, table); err != nil {
		return nil, fmt.Errorf("failed to get columns for table %s: %w", table, err)
	}
	return columns, nil
}

func (e *SQL) writeSqliteTableData(ctx context.Context, tx *sqlx.Tx, w io.Writer, table string, columns []string) error {
	query := fmt.Sprintf("SELECT * FROM %s", table)
	rows, err := tx.QueryxContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query data from table %s: %w", table, err)
	}

	defer func() {
		closeErr := rows.Close()
		if closeErr != nil {
			log.Printf("[WARN] failed to close rows: %v", closeErr)
		}
	}()

	for rows.Next() {
		if writeErr := e.writeSqliteRow(w, rows, table, columns); writeErr != nil {
			return writeErr
		}
	}

	if err = rows.Err(); err != nil {
		return fmt.Errorf("error iterating rows: %w", err)
	}

	return nil
}

func (e *SQL) writeSqliteRow(w io.Writer, rows *sqlx.Rows, table string, columns []string) error {
	row := make(map[string]any)
	if err := rows.MapScan(row); err != nil {
		return fmt.Errorf("failed to scan row: %w", err)
	}

	cols := make([]string, 0, len(columns))
	vals := make([]string, 0, len(columns))

	for _, col := range columns {
		value, ok := row[col]
		if !ok {
			continue
		}

		cols = append(cols, col)
		vals = append(vals, e.formatSqliteValue(value))
	}

	insertStmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		table, strings.Join(cols, ", "), strings.Join(vals, ", "))

	if _, err := fmt.Fprintln(w, insertStmt); err != nil {
		return fmt.Errorf("failed to write insert statement: %w", err)
	}

	return nil
}

func (e *SQL) formatSqliteValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "NULL"
	case int, int64, float64:
		return fmt.Sprintf("%v", v)
	case []byte:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(string(v), "'", "''"))
	case string:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
	case time.Time:
		return fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05"))
	default:
		return fmt.Sprintf("'%v'", v)
	}
}
