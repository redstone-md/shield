package storage

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/storage/engine"
)

func migrateTenantID(ctx context.Context, tx *sqlx.Tx, dbType engine.Type, table string) {
	var tid string
	if err := tx.QueryRowxContext(ctx, fmt.Sprintf("SELECT tenant_id FROM %s LIMIT 1", table)).Scan(&tid); err != nil {
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
}
