package storage

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/redstone-md/shield/app/storage/engine"
)

func migrateTenantID(ctx context.Context, tx *sqlx.Tx, dbType engine.Type, table string) {
	if columnExists(ctx, tx, dbType, table, "tenant_id") {
		return
	}

	sqliteQuery := fmt.Sprintf("ALTER TABLE %s ADD COLUMN tenant_id TEXT NOT NULL DEFAULT ''", table)
	pgQuery := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT ''", table)
	alterQuery := sqliteQuery
	if dbType == engine.Postgres {
		alterQuery = pgQuery
	}
	if _, err := tx.ExecContext(ctx, alterQuery); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		log.Printf("[WARN] failed to add tenant_id column to %s: %v", table, err)
		return
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET tenant_id = gid WHERE tenant_id = ''", table)); err != nil {
		log.Printf("[WARN] failed to backfill tenant_id for %s: %v", table, err)
		return
	}
	log.Printf("[DEBUG] %s table migrated with tenant_id", table)
}

func columnExists(ctx context.Context, tx *sqlx.Tx, dbType engine.Type, table, column string) bool {
	switch dbType {
	case engine.Postgres:
		var count int
		err := tx.GetContext(ctx, &count,
			"SELECT COUNT(*) FROM information_schema.columns WHERE table_name = $1 AND column_name = $2",
			table, column)
		return err == nil && count > 0
	case engine.Sqlite:
		var tid string
		err := tx.QueryRowxContext(ctx, fmt.Sprintf("SELECT %s FROM %s LIMIT 1", column, table)).Scan(&tid)
		if err == nil {
			return true
		}
		return !strings.Contains(err.Error(), "no such column")
	default:
		return false
	}
}
