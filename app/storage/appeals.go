package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/audit"
	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateAppealsTable engine.DBCmd = iota + 700
	CmdCreateAppealsIndexes
	CmdInsertAppeal
	CmdGetAppealByID
	CmdGetAppealByIncident
	CmdListAppeals
	CmdUpdateAppealStatus
	CmdUpdateAppealReplayResult
)

var appealsQueries = engine.NewQueryMap().
	Add(CmdCreateAppealsTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS appeals (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			incident_id BIGINT NOT NULL,
			appellant_user_id BIGINT DEFAULT 0,
			appellant_name TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'new',
			appeal_text TEXT DEFAULT '',
			resolution_text TEXT DEFAULT '',
			resolved_by TEXT DEFAULT '',
			replay_result TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			resolved_at TIMESTAMP
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS appeals (
			id SERIAL PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			incident_id BIGINT NOT NULL,
			appellant_user_id BIGINT DEFAULT 0,
			appellant_name TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'new',
			appeal_text TEXT DEFAULT '',
			resolution_text TEXT DEFAULT '',
			resolved_by TEXT DEFAULT '',
			replay_result TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			resolved_at TIMESTAMP
		)`,
	}).
	AddSame(CmdCreateAppealsIndexes, `
		CREATE INDEX IF NOT EXISTS idx_appeals_tenant_status ON appeals(tenant_id, status);
		CREATE INDEX IF NOT EXISTS idx_appeals_tenant_incident ON appeals(tenant_id, incident_id);
		CREATE INDEX IF NOT EXISTS idx_appeals_tenant_created ON appeals(tenant_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_appeals_idem_key ON appeals(tenant_id, incident_id);
	`).
	Add(CmdInsertAppeal, engine.Query{
		Sqlite:   `INSERT INTO appeals (gid, tenant_id, incident_id, appellant_user_id, appellant_name, status, appeal_text) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		Postgres: `INSERT INTO appeals (gid, tenant_id, incident_id, appellant_user_id, appellant_name, status, appeal_text) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
	}).
	AddSame(CmdGetAppealByID, `SELECT * FROM appeals WHERE tenant_id = ? AND id = ?`).
	AddSame(CmdGetAppealByIncident, `SELECT * FROM appeals WHERE tenant_id = ? AND incident_id = ? ORDER BY created_at DESC LIMIT 1`).
	Add(CmdListAppeals, engine.Query{
		Sqlite:   `SELECT * FROM appeals WHERE tenant_id = ? AND (%conditions) ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		Postgres: `SELECT * FROM appeals WHERE tenant_id = $1 AND (%conditions) ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
	}).
	Add(CmdUpdateAppealStatus, engine.Query{
		Sqlite:   `UPDATE appeals SET status = ?, resolution_text = ?, resolved_by = ?, updated_at = ?, resolved_at = ? WHERE tenant_id = ? AND id = ?`,
		Postgres: `UPDATE appeals SET status = $1, resolution_text = $2, resolved_by = $3, updated_at = $4, resolved_at = $5 WHERE tenant_id = $6 AND id = $7`,
	}).
	Add(CmdUpdateAppealReplayResult, engine.Query{
		Sqlite:   `UPDATE appeals SET replay_result = ?, updated_at = ? WHERE tenant_id = ? AND id = ?`,
		Postgres: `UPDATE appeals SET replay_result = $1, updated_at = $2 WHERE tenant_id = $3 AND id = $4`,
	})

type AppealStorage struct {
	*engine.SQL
	engine.RWLocker
}

func NewAppealStorage(ctx context.Context, db *engine.SQL) (*AppealStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &AppealStorage{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "appeals",
		CreateTable:   CmdCreateAppealsTable,
		CreateIndexes: CmdCreateAppealsIndexes,
		MigrateFunc:   func(_ context.Context, _ *sqlx.Tx, _ string) error { return nil },
		QueriesMap:    appealsQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init appeals storage: %w", err)
	}
	return res, nil
}

func (s *AppealStorage) Create(ctx context.Context, appeal audit.Appeal) (audit.Appeal, error) {
	s.Lock()
	defer s.Unlock()

	query, err := appealsQueries.Pick(s.Type(), CmdInsertAppeal)
	if err != nil {
		return audit.Appeal{}, fmt.Errorf("failed to get insert query: %w", err)
	}
	query = s.Adopt(query)

	status := string(appeal.Status)
	if status == "" {
		status = string(audit.AppealNew)
	}

	_, err = s.ExecContext(ctx, query,
		s.GID(), s.TenantID(), appeal.IncidentID, appeal.AppellantUserID, appeal.AppellantName,
		status, appeal.AppealText,
	)
	if err != nil {
		return audit.Appeal{}, fmt.Errorf("failed to insert appeal: %w", err)
	}

	created, err := s.getByIncidentNoLock(ctx, appeal.IncidentID)
	if err != nil {
		return audit.Appeal{}, fmt.Errorf("appeal inserted but failed to retrieve: %w", err)
	}
	return created, nil
}

func (s *AppealStorage) Get(ctx context.Context, id int64) (audit.Appeal, error) {
	s.RLock()
	defer s.RUnlock()

	query := s.Adopt("SELECT * FROM appeals WHERE tenant_id = ? AND id = ?")
	var rec appealRecord
	if err := s.GetContext(ctx, &rec, query, s.TenantID(), id); err != nil {
		return audit.Appeal{}, fmt.Errorf("failed to get appeal %d: %w", id, err)
	}
	return rec.toAppeal(), nil
}

func (s *AppealStorage) GetByIncident(ctx context.Context, incidentID int64) (audit.Appeal, error) {
	s.RLock()
	defer s.RUnlock()
	return s.getByIncidentNoLock(ctx, incidentID)
}

func (s *AppealStorage) getByIncidentNoLock(ctx context.Context, incidentID int64) (audit.Appeal, error) {
	query, err := appealsQueries.Pick(s.Type(), CmdGetAppealByIncident)
	if err != nil {
		return audit.Appeal{}, fmt.Errorf("failed to get query: %w", err)
	}
	query = s.Adopt(query)
	var rec appealRecord
	if err := s.GetContext(ctx, &rec, query, s.TenantID(), incidentID); err != nil {
		return audit.Appeal{}, fmt.Errorf("failed to get appeal for incident %d: %w", incidentID, err)
	}
	return rec.toAppeal(), nil
}

func (s *AppealStorage) List(ctx context.Context, filter audit.AppealFilter) ([]audit.Appeal, error) {
	s.RLock()
	defer s.RUnlock()

	conditions := []string{}
	args := []any{}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(filter.Status))
	}
	if len(conditions) == 0 {
		conditions = append(conditions, "1=1")
	}

	where := strings.Join(conditions, " AND ")
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	query := fmt.Sprintf("SELECT * FROM appeals WHERE tenant_id = ? AND (%s) ORDER BY created_at DESC LIMIT %d", where, limit)
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}
	query = s.Adopt(query)

	fullArgs := append([]any{s.TenantID()}, args...)
	var recs []appealRecord
	if err := s.SelectContext(ctx, &recs, query, fullArgs...); err != nil {
		return nil, fmt.Errorf("failed to list appeals: %w", err)
	}

	result := make([]audit.Appeal, len(recs))
	for i, r := range recs {
		result[i] = r.toAppeal()
	}
	return result, nil
}

func (s *AppealStorage) UpdateStatus(
	ctx context.Context, id int64, status audit.AppealStatus, resolvedBy, resolutionText string,
) error {
	s.Lock()
	defer s.Unlock()

	var resolvedAt any
	if status == audit.AppealAccepted || status == audit.AppealRejected {
		resolvedAt = time.Now().UTC()
	}

	query, err := appealsQueries.Pick(s.Type(), CmdUpdateAppealStatus)
	if err != nil {
		return fmt.Errorf("failed to get update query: %w", err)
	}
	query = s.Adopt(query)

	_, err = s.ExecContext(ctx, query, string(status), resolutionText, resolvedBy, time.Now().UTC(), resolvedAt, s.TenantID(), id)
	if err != nil {
		return fmt.Errorf("failed to update appeal status: %w", err)
	}
	return nil
}

func (s *AppealStorage) UpdateReplayResult(ctx context.Context, id int64, result audit.ReplayResult) error {
	s.Lock()
	defer s.Unlock()

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal replay result: %w", err)
	}

	query, err := appealsQueries.Pick(s.Type(), CmdUpdateAppealReplayResult)
	if err != nil {
		return fmt.Errorf("failed to get update replay query: %w", err)
	}
	query = s.Adopt(query)

	_, err = s.ExecContext(ctx, query, string(resultJSON), time.Now().UTC(), s.TenantID(), id)
	if err != nil {
		return fmt.Errorf("failed to update appeal replay result: %w", err)
	}
	return nil
}

type appealRecord struct {
	ID               int64        `db:"id"`
	GID              string       `db:"gid"`
	TenantID         string       `db:"tenant_id"`
	IncidentID       int64        `db:"incident_id"`
	AppellantUserID  int64        `db:"appellant_user_id"`
	AppellantName    string       `db:"appellant_name"`
	Status           string       `db:"status"`
	AppealText       string       `db:"appeal_text"`
	ResolutionText   string       `db:"resolution_text"`
	ResolvedBy       string       `db:"resolved_by"`
	ReplayResultJSON string       `db:"replay_result"`
	CreatedAt        time.Time    `db:"created_at"`
	UpdatedAt        time.Time    `db:"updated_at"`
	ResolvedAt       sql.NullTime `db:"resolved_at"`
}

func (r appealRecord) toAppeal() audit.Appeal {
	a := audit.Appeal{
		ID:              r.ID,
		GID:             r.GID,
		TenantID:        r.TenantID,
		IncidentID:      r.IncidentID,
		AppellantUserID: r.AppellantUserID,
		AppellantName:   r.AppellantName,
		Status:          audit.AppealStatus(r.Status),
		AppealText:      r.AppealText,
		ResolutionText:  r.ResolutionText,
		ResolvedBy:      r.ResolvedBy,
		ReplayResult:    r.ReplayResultJSON,
		CreatedAt:       r.CreatedAt.Local(),
		UpdatedAt:       r.UpdatedAt.Local(),
	}
	if r.ResolvedAt.Valid {
		t := r.ResolvedAt.Time.Local()
		a.ResolvedAt = &t
	}
	return a
}
