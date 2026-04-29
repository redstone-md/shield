package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/feedback"
	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateCandidatesTable engine.DBCmd = iota + 850
	CmdCreateCandidatesIndexes
	CmdInsertCandidate
	CmdGetCandidateByID
	CmdListCandidates
	CmdUpdateCandidateStatus
)

var candidatesQueries = engine.NewQueryMap().
	Add(CmdCreateCandidatesTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS candidates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL,
			value TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_id BIGINT DEFAULT 0,
			score REAL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			reviewed_by TEXT DEFAULT '',
			review_comment TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			reviewed_at DATETIME
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS candidates (
			id SERIAL PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			type TEXT NOT NULL,
			value TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_id BIGINT DEFAULT 0,
			score DOUBLE PRECISION DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			reviewed_by TEXT DEFAULT '',
			review_comment TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			reviewed_at TIMESTAMP
		)`,
	}).
	AddSame(CmdCreateCandidatesIndexes, `
		CREATE INDEX IF NOT EXISTS idx_candidates_tenant ON candidates(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_candidates_status ON candidates(tenant_id, status);
		CREATE INDEX IF NOT EXISTS idx_candidates_type ON candidates(tenant_id, type);
	`).
	Add(CmdInsertCandidate, engine.Query{
		Sqlite:   `INSERT INTO candidates (gid, tenant_id, type, value, source, source_id, score) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		Postgres: `INSERT INTO candidates (gid, tenant_id, type, value, source, source_id, score) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
	}).
	AddSame(CmdGetCandidateByID, `SELECT * FROM candidates WHERE tenant_id = ? AND id = ?`).
	Add(CmdUpdateCandidateStatus, engine.Query{
		Sqlite:   `UPDATE candidates SET status = ?, reviewed_by = ?, review_comment = ?, reviewed_at = ? WHERE tenant_id = ? AND id = ?`,
		Postgres: `UPDATE candidates SET status = $1, reviewed_by = $2, review_comment = $3, reviewed_at = $4 WHERE tenant_id = $5 AND id = $6`,
	})

type CandidateStorage struct {
	*engine.SQL
	engine.RWLocker
}

func NewCandidateStorage(ctx context.Context, db *engine.SQL) (*CandidateStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &CandidateStorage{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "candidates",
		CreateTable:   CmdCreateCandidatesTable,
		CreateIndexes: CmdCreateCandidatesIndexes,
		MigrateFunc:   func(_ context.Context, _ *sqlx.Tx, _ string) error { return nil },
		QueriesMap:    candidatesQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init candidates storage: %w", err)
	}
	return res, nil
}

func (s *CandidateStorage) Create(ctx context.Context, c feedback.CandidateEntry) (feedback.CandidateEntry, error) {
	s.Lock()
	defer s.Unlock()

	query, err := candidatesQueries.Pick(s.Type(), CmdInsertCandidate)
	if err != nil {
		return feedback.CandidateEntry{}, fmt.Errorf("failed to get insert query: %w", err)
	}
	query = s.Adopt(query)

	res, execErr := s.ExecContext(ctx, query,
		s.GID(), s.TenantID(), string(c.Type), c.Value, c.Source, c.SourceID, c.Score,
	)
	if execErr != nil {
		return feedback.CandidateEntry{}, fmt.Errorf("failed to insert candidate: %w", execErr)
	}

	id, _ := res.LastInsertId()
	c.ID = id
	c.GID = s.GID()
	c.TenantID = s.TenantID()
	c.CreatedAt = time.Now()
	return c, nil
}

func (s *CandidateStorage) GetByID(ctx context.Context, id int64) (feedback.CandidateEntry, error) {
	s.RLock()
	defer s.RUnlock()

	query, err := candidatesQueries.Pick(s.Type(), CmdGetCandidateByID)
	if err != nil {
		return feedback.CandidateEntry{}, fmt.Errorf("failed to get query: %w", err)
	}
	query = s.Adopt(query)

	var entry feedback.CandidateEntry
	if qErr := s.GetContext(ctx, &entry, query, s.TenantID(), id); qErr != nil {
		return feedback.CandidateEntry{}, fmt.Errorf("candidate not found: %w", qErr)
	}
	return entry, nil
}

func (s *CandidateStorage) List(ctx context.Context, filter feedback.CandidateFilter) ([]feedback.CandidateEntry, error) {
	s.RLock()
	defer s.RUnlock()

	conditions := []string{"tenant_id = ?"}
	args := []any{s.TenantID()}

	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.Type != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, string(filter.Type))
	}
	if filter.Source != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, filter.Source)
	}
	if filter.SourceID > 0 {
		conditions = append(conditions, "source_id = ?")
		args = append(args, filter.SourceID)
	}

	where := strings.Join(conditions, " AND ")
	query := fmt.Sprintf("SELECT * FROM candidates WHERE %s ORDER BY score DESC, created_at DESC", where)

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	query = s.Adopt(query)
	var entries []feedback.CandidateEntry
	if qErr := s.SelectContext(ctx, &entries, query, args...); qErr != nil {
		return nil, fmt.Errorf("failed to list candidates: %w", qErr)
	}
	return entries, nil
}

func (s *CandidateStorage) UpdateStatus(ctx context.Context, id int64, status feedback.CandidateStatus, reviewedBy, comment string) error {
	s.Lock()
	defer s.Unlock()

	query, err := candidatesQueries.Pick(s.Type(), CmdUpdateCandidateStatus)
	if err != nil {
		return fmt.Errorf("failed to get query: %w", err)
	}
	query = s.Adopt(query)

	var reviewedAt interface{}
	if status != feedback.CandidatePending {
		reviewedAt = time.Now()
	}

	_, execErr := s.ExecContext(ctx, query,
		string(status), reviewedBy, comment, reviewedAt,
		s.TenantID(), id,
	)
	if execErr != nil {
		return fmt.Errorf("failed to update candidate status: %w", execErr)
	}
	return nil
}

func (s *CandidateStorage) FindByValue(ctx context.Context, candidateType feedback.CandidateType, value string) ([]feedback.CandidateEntry, error) {
	s.RLock()
	defer s.RUnlock()

	query := "SELECT * FROM candidates WHERE tenant_id = ? AND type = ? AND value = ? ORDER BY created_at DESC"
	query = s.Adopt(query)

	var entries []feedback.CandidateEntry
	if qErr := s.SelectContext(ctx, &entries, query, s.TenantID(), string(candidateType), value); qErr != nil {
		return nil, fmt.Errorf("failed to find candidates: %w", qErr)
	}
	return entries, nil
}
