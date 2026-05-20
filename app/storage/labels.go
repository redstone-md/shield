package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/redstone-md/shield/app/feedback"
	"github.com/redstone-md/shield/app/storage/engine"
)

const (
	CmdCreateLabelsTable engine.DBCmd = iota + 800
	CmdCreateLabelsIndexes
	CmdInsertLabel
	CmdGetLabelByID
	CmdGetLabelsByDetectedSpamID
	CmdGetLabelsByIncidentID
	CmdListLabels
	CmdLabelStats
)

var labelsQueries = engine.NewQueryMap().
	Add(CmdCreateLabelsTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS labels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			detected_spam_id INTEGER DEFAULT 0,
			incident_id INTEGER DEFAULT 0,
			label TEXT NOT NULL,
			labeled_by TEXT DEFAULT '',
			comment TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS labels (
			id SERIAL PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			detected_spam_id BIGINT DEFAULT 0,
			incident_id BIGINT DEFAULT 0,
			label TEXT NOT NULL,
			labeled_by TEXT DEFAULT '',
			comment TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}).
	AddSame(CmdCreateLabelsIndexes, `
		CREATE INDEX IF NOT EXISTS idx_labels_tenant ON labels(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_labels_spam_id ON labels(tenant_id, detected_spam_id);
		CREATE INDEX IF NOT EXISTS idx_labels_incident_id ON labels(tenant_id, incident_id);
		CREATE INDEX IF NOT EXISTS idx_labels_label ON labels(tenant_id, label);
	`).
	Add(CmdInsertLabel, engine.Query{
		Sqlite:   `INSERT INTO labels (gid, tenant_id, detected_spam_id, incident_id, label, labeled_by, comment) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		Postgres: `INSERT INTO labels (gid, tenant_id, detected_spam_id, incident_id, label, labeled_by, comment) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
	}).
	AddSame(CmdGetLabelByID, `SELECT * FROM labels WHERE tenant_id = ? AND id = ?`).
	AddSame(CmdGetLabelsByDetectedSpamID, `SELECT * FROM labels WHERE tenant_id = ? AND detected_spam_id = ? ORDER BY created_at DESC`).
	AddSame(CmdGetLabelsByIncidentID, `SELECT * FROM labels WHERE tenant_id = ? AND incident_id = ? ORDER BY created_at DESC`).
	Add(CmdListLabels, engine.Query{
		Sqlite:   `SELECT * FROM labels WHERE tenant_id = ? AND (%conditions) ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		Postgres: `SELECT * FROM labels WHERE tenant_id = $1 AND (%conditions) ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
	}).
	Add(CmdLabelStats, engine.Query{
		Sqlite:   `SELECT label, COUNT(*) as cnt FROM labels WHERE tenant_id = ? GROUP BY label`,
		Postgres: `SELECT label, COUNT(*) as cnt FROM labels WHERE tenant_id = $1 GROUP BY label`,
	})

type LabelStorage struct {
	*engine.SQL
	engine.RWLocker
}

func NewLabelStorage(ctx context.Context, db *engine.SQL) (*LabelStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &LabelStorage{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "labels",
		CreateTable:   CmdCreateLabelsTable,
		CreateIndexes: CmdCreateLabelsIndexes,
		MigrateFunc:   func(_ context.Context, _ *sqlx.Tx, _ string) error { return nil },
		QueriesMap:    labelsQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init labels storage: %w", err)
	}
	return res, nil
}

func (s *LabelStorage) Create(ctx context.Context, entry feedback.LabelEntry) (feedback.LabelEntry, error) {
	s.Lock()
	defer s.Unlock()

	query, err := labelsQueries.Pick(s.Type(), CmdInsertLabel)
	if err != nil {
		return feedback.LabelEntry{}, fmt.Errorf("failed to get insert query: %w", err)
	}
	query = s.Adopt(query)

	res, execErr := s.ExecContext(ctx, query,
		s.GID(), s.TenantID(), entry.DetectedSpamID, entry.IncidentID,
		string(entry.Label), entry.LabeledBy, entry.Comment,
	)
	if execErr != nil {
		return feedback.LabelEntry{}, fmt.Errorf("failed to insert label: %w", execErr)
	}

	id, _ := res.LastInsertId()
	entry.ID = id
	entry.GID = s.GID()
	entry.TenantID = s.TenantID()
	entry.CreatedAt = time.Now()
	return entry, nil
}

func (s *LabelStorage) GetByID(ctx context.Context, id int64) (feedback.LabelEntry, error) {
	s.RLock()
	defer s.RUnlock()

	query, err := labelsQueries.Pick(s.Type(), CmdGetLabelByID)
	if err != nil {
		return feedback.LabelEntry{}, fmt.Errorf("failed to get query: %w", err)
	}
	query = s.Adopt(query)

	var entry feedback.LabelEntry
	if qErr := s.GetContext(ctx, &entry, query, s.TenantID(), id); qErr != nil {
		return feedback.LabelEntry{}, fmt.Errorf("label not found: %w", qErr)
	}
	return entry, nil
}

func (s *LabelStorage) GetByDetectedSpamID(ctx context.Context, spamID int64) ([]feedback.LabelEntry, error) {
	s.RLock()
	defer s.RUnlock()

	query, err := labelsQueries.Pick(s.Type(), CmdGetLabelsByDetectedSpamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get query: %w", err)
	}
	query = s.Adopt(query)

	var entries []feedback.LabelEntry
	if qErr := s.SelectContext(ctx, &entries, query, s.TenantID(), spamID); qErr != nil {
		return nil, fmt.Errorf("failed to get labels: %w", qErr)
	}
	return entries, nil
}

func (s *LabelStorage) GetByIncidentID(ctx context.Context, incidentID int64) ([]feedback.LabelEntry, error) {
	s.RLock()
	defer s.RUnlock()

	query, err := labelsQueries.Pick(s.Type(), CmdGetLabelsByIncidentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get query: %w", err)
	}
	query = s.Adopt(query)

	var entries []feedback.LabelEntry
	if qErr := s.SelectContext(ctx, &entries, query, s.TenantID(), incidentID); qErr != nil {
		return nil, fmt.Errorf("failed to get labels: %w", qErr)
	}
	return entries, nil
}

func (s *LabelStorage) List(ctx context.Context, filter feedback.LabelFilter) ([]feedback.LabelEntry, error) {
	s.RLock()
	defer s.RUnlock()

	conditions := []string{"tenant_id = ?"}
	args := []any{s.TenantID()}

	if filter.Label != "" {
		conditions = append(conditions, "label = ?")
		args = append(args, string(filter.Label))
	}
	if filter.LabeledBy != "" {
		conditions = append(conditions, "labeled_by = ?")
		args = append(args, filter.LabeledBy)
	}

	where := strings.Join(conditions, " AND ")
	query := fmt.Sprintf("SELECT * FROM labels WHERE %s ORDER BY created_at DESC", where)

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	query = s.Adopt(query)
	var entries []feedback.LabelEntry
	if qErr := s.SelectContext(ctx, &entries, query, args...); qErr != nil {
		return nil, fmt.Errorf("failed to list labels: %w", qErr)
	}
	return entries, nil
}

func (s *LabelStorage) Stats(ctx context.Context) (map[feedback.Label]int, error) {
	s.RLock()
	defer s.RUnlock()

	query, err := labelsQueries.Pick(s.Type(), CmdLabelStats)
	if err != nil {
		return nil, fmt.Errorf("failed to get query: %w", err)
	}
	query = s.Adopt(query)

	rows, qErr := s.QueryxContext(ctx, query, s.TenantID())
	if qErr != nil {
		return nil, fmt.Errorf("failed to get label stats: %w", qErr)
	}
	defer rows.Close()

	result := make(map[feedback.Label]int)
	for rows.Next() {
		var label string
		var cnt int
		if scanErr := rows.Scan(&label, &cnt); scanErr != nil {
			return nil, fmt.Errorf("failed to scan label stats: %w", scanErr)
		}
		result[feedback.Label(label)] = cnt
	}
	return result, nil
}
