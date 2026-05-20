package storage

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/redstone-md/shield/app/storage/engine"
)

type RestoreService struct {
	db *engine.SQL
}

func NewRestoreService(db *engine.SQL) *RestoreService {
	return &RestoreService{db: db}
}

func (s *RestoreService) RestoreTenant(ctx context.Context, tenantID string, r io.Reader) error {
	if tenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var executed, skipped int
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "--") || strings.EqualFold(line, "BEGIN;") || strings.EqualFold(line, "COMMIT;") {
			continue
		}

		if !s.shouldIncludeStatement(line, tenantID) {
			skipped++
			continue
		}

		if _, err := tx.ExecContext(ctx, line); err != nil {
			if isTableNotExistErr(err) {
				log.Printf("[WARN] restore skipping statement for missing table: %v", err)
				skipped++
				continue
			}
			return fmt.Errorf("failed to execute restore statement: %w\nstatement: %s", err, truncate(line, 200))
		}
		executed++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading restore input: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit restore transaction: %w", err)
	}

	log.Printf("[INFO] tenant %s restore complete: %d statements executed, %d skipped", tenantID, executed, skipped)
	return nil
}

func (s *RestoreService) shouldIncludeStatement(line, tenantID string) bool {
	upper := strings.ToUpper(line)

	if strings.HasPrefix(upper, "CREATE") || strings.HasPrefix(upper, "DROP") {
		return false
	}

	if !strings.HasPrefix(upper, "INSERT") {
		return true
	}

	if !strings.Contains(line, "tenant_id") {
		return false
	}

	return strings.Contains(line, fmt.Sprintf("'%s'", tenantID))
}

func isTableNotExistErr(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "relation") && strings.Contains(msg, "not exist")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
