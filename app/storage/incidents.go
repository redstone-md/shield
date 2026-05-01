package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/audit"
	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateIncidentsTable engine.DBCmd = iota + 700
	CmdCreateIncidentsIndexes
	CmdInsertIncident
	CmdGetIncident
	CmdGetIncidentByIdempotencyKey
	CmdListIncidents
	CmdUpdateIncidentStatus
	CmdUpdateIncidentSeverity
	CmdInsertIncidentComment
	CmdListIncidentComments
	CmdCreateIncidentCommentsTable
)

var incidentsQueries = engine.NewQueryMap().
	Add(CmdCreateIncidentsTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS incidents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			severity TEXT NOT NULL DEFAULT 'low',
			idempotency_key TEXT NOT NULL DEFAULT '',
			detected_spam_id INTEGER DEFAULT 0,
			report_id INTEGER DEFAULT 0,
			reason_code TEXT NOT NULL DEFAULT 'unknown',
			reason_text TEXT DEFAULT '',
			spam_user_id INTEGER DEFAULT 0,
			spam_user_name TEXT DEFAULT '',
			chat_id INTEGER DEFAULT 0,
			message_text TEXT DEFAULT '',
			resolved_by TEXT DEFAULT '',
			comment TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			resolved_at DATETIME,
			UNIQUE(tenant_id, idempotency_key)
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS incidents (
			id SERIAL PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			severity TEXT NOT NULL DEFAULT 'low',
			idempotency_key TEXT NOT NULL DEFAULT '',
			detected_spam_id BIGINT DEFAULT 0,
			report_id BIGINT DEFAULT 0,
			reason_code TEXT NOT NULL DEFAULT 'unknown',
			reason_text TEXT DEFAULT '',
			spam_user_id BIGINT DEFAULT 0,
			spam_user_name TEXT DEFAULT '',
			chat_id BIGINT DEFAULT 0,
			message_text TEXT DEFAULT '',
			resolved_by TEXT DEFAULT '',
			comment TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			resolved_at TIMESTAMP,
			UNIQUE(tenant_id, idempotency_key)
		)`,
	}).
	AddSame(CmdCreateIncidentsIndexes, `
		CREATE INDEX IF NOT EXISTS idx_incidents_tenant_status ON incidents(tenant_id, status);
		CREATE INDEX IF NOT EXISTS idx_incidents_tenant_source ON incidents(tenant_id, source);
		CREATE INDEX IF NOT EXISTS idx_incidents_tenant_created ON incidents(tenant_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_incidents_tenant_severity ON incidents(tenant_id, severity);
		CREATE INDEX IF NOT EXISTS idx_incidents_idem_key ON incidents(tenant_id, idempotency_key);
		CREATE INDEX IF NOT EXISTS idx_incidents_gid ON incidents(gid)`).
	Add(CmdInsertIncident, engine.Query{
		Sqlite: `INSERT OR IGNORE INTO incidents
			(gid, tenant_id, source, status, severity, idempotency_key, detected_spam_id, report_id,
			 reason_code, reason_text, spam_user_id, spam_user_name, chat_id, message_text, comment)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		Postgres: `INSERT INTO incidents
			(gid, tenant_id, source, status, severity, idempotency_key, detected_spam_id, report_id,
			 reason_code, reason_text, spam_user_id, spam_user_name, chat_id, message_text, comment)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			ON CONFLICT (tenant_id, idempotency_key) DO NOTHING`,
	}).
	AddSame(CmdGetIncident, `SELECT * FROM incidents WHERE tenant_id = ? AND id = ?`).
	AddSame(CmdGetIncidentByIdempotencyKey, `SELECT * FROM incidents WHERE tenant_id = ? AND idempotency_key = ?`).
	AddSame(CmdUpdateIncidentStatus, `UPDATE incidents SET status = ?, resolved_by = ?, updated_at = ?, resolved_at = ? WHERE tenant_id = ? AND id = ?`).
	AddSame(CmdUpdateIncidentSeverity, `UPDATE incidents SET severity = ?, updated_at = ? WHERE tenant_id = ? AND id = ?`).
	Add(CmdCreateIncidentCommentsTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS incident_comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			incident_id INTEGER NOT NULL,
			author_type TEXT NOT NULL,
			author_id TEXT DEFAULT '',
			action TEXT NOT NULL,
			payload TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS incident_comments (
			id SERIAL PRIMARY KEY,
			incident_id BIGINT NOT NULL,
			author_type TEXT NOT NULL,
			author_id TEXT DEFAULT '',
			action TEXT NOT NULL,
			payload TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}).
	Add(CmdInsertIncidentComment, engine.Query{
		Sqlite:   `INSERT INTO incident_comments (incident_id, author_type, author_id, action, payload) VALUES (?, ?, ?, ?, ?)`,
		Postgres: `INSERT INTO incident_comments (incident_id, author_type, author_id, action, payload) VALUES ($1, $2, $3, $4, $5)`,
	}).
	AddSame(CmdListIncidentComments, `SELECT * FROM incident_comments WHERE incident_id = ? ORDER BY created_at ASC`)

var incidentsListQuery = engine.NewQueryMap().
	Add(CmdListIncidents, engine.Query{
		Sqlite:   buildIncidentListQuery("?"),
		Postgres: buildIncidentListQuery("$"),
	})

func buildIncidentListQuery(placeholder string) string {
	conditions := []string{"tenant_id = " + placeholder}
	n := 2
	addIf := func(field string, status string) {
		if status != "" {
			conditions = append(conditions, fmt.Sprintf("%s = %s%d", field, placeholder, n))
			n++
		}
	}
	_ = addIf
	return "PLACEHOLDER_QUERY"
}

type IncidentStorage struct {
	*engine.SQL
	engine.RWLocker
}

func NewIncidentStorage(ctx context.Context, db *engine.SQL) (*IncidentStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &IncidentStorage{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "incidents",
		CreateTable:   CmdCreateIncidentsTable,
		CreateIndexes: CmdCreateIncidentsIndexes,
		MigrateFunc:   res.migrate,
		QueriesMap:    incidentsQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init incidents storage: %w", err)
	}

	if err := res.createCommentsTable(ctx); err != nil {
		return nil, fmt.Errorf("failed to init incident_comments table: %w", err)
	}
	return res, nil
}

func (s *IncidentStorage) createCommentsTable(ctx context.Context) error {
	q, err := incidentsQueries.Pick(s.Type(), CmdCreateIncidentCommentsTable)
	if err != nil {
		return fmt.Errorf("failed to get create comments table query: %w", err)
	}
	_, err = s.ExecContext(ctx, s.Adopt(q))
	if err != nil {
		return fmt.Errorf("failed to create incident_comments table: %w", err)
	}
	return nil
}

func (s *IncidentStorage) migrate(_ context.Context, _ *sqlx.Tx, _ string) error {
	return nil
}

func (s *IncidentStorage) Create(ctx context.Context, incident audit.Incident) (audit.Incident, error) {
	s.Lock()
	defer s.Unlock()

	query, err := incidentsQueries.Pick(s.Type(), CmdInsertIncident)
	if err != nil {
		return audit.Incident{}, fmt.Errorf("failed to get insert query: %w", err)
	}
	query = s.Adopt(query)

	result, err := s.ExecContext(ctx, query,
		s.GID(), s.TenantID(), string(incident.Source), string(incident.Status), string(incident.Severity),
		incident.IdempotencyKey, incident.DetectedSpamID, incident.ReportID,
		string(incident.ReasonCode), incident.ReasonText, incident.SpamUserID, incident.SpamUserName,
		incident.ChatID, incident.MessageText, incident.Comment,
	)
	if err != nil {
		return audit.Incident{}, fmt.Errorf("failed to insert incident: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		existing, gErr := s.getByIdempotencyKeyNoLock(ctx, incident.IdempotencyKey)
		if gErr != nil {
			return audit.Incident{}, fmt.Errorf("incident already exists but failed to retrieve: %w", gErr)
		}
		return existing, nil
	}

	created, rErr := s.getByIdempotencyKeyNoLock(ctx, incident.IdempotencyKey)
	if rErr != nil {
		return audit.Incident{}, fmt.Errorf("incident inserted but failed to retrieve: %w", rErr)
	}
	return created, nil
}

func (s *IncidentStorage) Get(ctx context.Context, id int64) (audit.Incident, error) {
	s.RLock()
	defer s.RUnlock()
	return s.getNoLock(ctx, id)
}

func (s *IncidentStorage) getNoLock(ctx context.Context, id int64) (audit.Incident, error) {
	query := s.Adopt("SELECT * FROM incidents WHERE tenant_id = ? AND id = ?")
	var rec incidentRecord
	if err := s.GetContext(ctx, &rec, query, s.TenantID(), id); err != nil {
		return audit.Incident{}, fmt.Errorf("failed to get incident %d: %w", id, err)
	}
	return rec.toIncident(), nil
}

func (s *IncidentStorage) GetByIdempotencyKey(ctx context.Context, tenantID, key string) (audit.Incident, error) {
	s.RLock()
	defer s.RUnlock()
	return s.getByIdempotencyKeyNoLock(ctx, key)
}

func (s *IncidentStorage) getByIdempotencyKeyNoLock(ctx context.Context, key string) (audit.Incident, error) {
	query := s.Adopt("SELECT * FROM incidents WHERE tenant_id = ? AND idempotency_key = ?")
	var rec incidentRecord
	if err := s.GetContext(ctx, &rec, query, s.TenantID(), key); err != nil {
		return audit.Incident{}, fmt.Errorf("failed to get incident by key %s: %w", key, err)
	}
	return rec.toIncident(), nil
}

func (s *IncidentStorage) List(ctx context.Context, filter audit.IncidentFilter) ([]audit.Incident, error) {
	s.RLock()
	defer s.RUnlock()

	conditions := []string{"tenant_id = ?"}
	args := []any{s.TenantID()}

	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.Source != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, string(filter.Source))
	}
	if filter.Severity != "" {
		conditions = append(conditions, "severity = ?")
		args = append(args, string(filter.Severity))
	}
	if filter.Reason != "" {
		conditions = append(conditions, "reason_code = ?")
		args = append(args, string(filter.Reason))
	}
	if filter.From != (time.Time{}) {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, filter.From)
	}
	if filter.To != (time.Time{}) {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, filter.To)
	}

	where := strings.Join(conditions, " AND ")
	query := fmt.Sprintf("SELECT * FROM incidents WHERE %s ORDER BY created_at DESC", where)

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	query = s.Adopt(query)
	var recs []incidentRecord
	if err := s.SelectContext(ctx, &recs, query, args...); err != nil {
		return nil, fmt.Errorf("failed to list incidents: %w", err)
	}

	result := make([]audit.Incident, len(recs))
	for i, r := range recs {
		result[i] = r.toIncident()
	}
	return result, nil
}

func (s *IncidentStorage) UpdateStatus(ctx context.Context, id int64, status audit.IncidentStatus, resolvedBy string) error {
	s.Lock()
	defer s.Unlock()

	var resolvedAt any
	if status == audit.IncidentStatusResolved || status == audit.IncidentStatusClosed {
		now := time.Now().UTC()
		resolvedAt = now
	}

	query := s.Adopt("UPDATE incidents SET status = ?, resolved_by = ?, updated_at = ?, resolved_at = ? WHERE tenant_id = ? AND id = ?")
	_, err := s.ExecContext(ctx, query, string(status), resolvedBy, time.Now().UTC(), resolvedAt, s.TenantID(), id)
	if err != nil {
		return fmt.Errorf("failed to update incident status: %w", err)
	}
	return nil
}

func (s *IncidentStorage) UpdateSeverity(ctx context.Context, id int64, severity audit.IncidentSeverity) error {
	s.Lock()
	defer s.Unlock()

	query := s.Adopt("UPDATE incidents SET severity = ?, updated_at = ? WHERE tenant_id = ? AND id = ?")
	_, err := s.ExecContext(ctx, query, string(severity), time.Now().UTC(), s.TenantID(), id)
	if err != nil {
		return fmt.Errorf("failed to update incident severity: %w", err)
	}
	return nil
}

func (s *IncidentStorage) AddComment(ctx context.Context, comment audit.IncidentComment) (audit.IncidentComment, error) {
	s.Lock()
	defer s.Unlock()

	query, err := incidentsQueries.Pick(s.Type(), CmdInsertIncidentComment)
	if err != nil {
		return audit.IncidentComment{}, fmt.Errorf("failed to get insert comment query: %w", err)
	}
	query = s.Adopt(query)

	result, err := s.ExecContext(ctx, query, comment.IncidentID, comment.AuthorType, comment.AuthorID, comment.Action, comment.Payload)
	if err != nil {
		return audit.IncidentComment{}, fmt.Errorf("failed to insert incident comment: %w", err)
	}

	id, _ := result.LastInsertId()
	if id > 0 {
		comment.ID = id
	} else {
		var last commentRecord
		readQ := s.Adopt("SELECT * FROM incident_comments WHERE incident_id = ? ORDER BY id DESC LIMIT 1")
		if err := s.GetContext(ctx, &last, readQ, comment.IncidentID); err == nil {
			return last.toComment(), nil
		}
	}
	return comment, nil
}

func (s *IncidentStorage) ListComments(ctx context.Context, incidentID int64) ([]audit.IncidentComment, error) {
	s.RLock()
	defer s.RUnlock()

	query, err := incidentsQueries.Pick(s.Type(), CmdListIncidentComments)
	if err != nil {
		return nil, fmt.Errorf("failed to get list comments query: %w", err)
	}
	query = s.Adopt(query)

	var recs []commentRecord
	if err := s.SelectContext(ctx, &recs, query, incidentID); err != nil {
		return nil, fmt.Errorf("failed to list comments for incident %d: %w", incidentID, err)
	}

	result := make([]audit.IncidentComment, len(recs))
	for i, r := range recs {
		result[i] = r.toComment()
	}
	return result, nil
}

type incidentRecord struct {
	ID             int64        `db:"id"`
	GID            string       `db:"gid"`
	TenantID       string       `db:"tenant_id"`
	Source         string       `db:"source"`
	Status         string       `db:"status"`
	Severity       string       `db:"severity"`
	IdempotencyKey string       `db:"idempotency_key"`
	DetectedSpamID int64        `db:"detected_spam_id"`
	ReportID       int64        `db:"report_id"`
	ReasonCode     string       `db:"reason_code"`
	ReasonText     string       `db:"reason_text"`
	SpamUserID     int64        `db:"spam_user_id"`
	SpamUserName   string       `db:"spam_user_name"`
	ChatID         int64        `db:"chat_id"`
	MessageText    string       `db:"message_text"`
	ResolvedBy     string       `db:"resolved_by"`
	Comment        string       `db:"comment"`
	CreatedAt      time.Time    `db:"created_at"`
	UpdatedAt      time.Time    `db:"updated_at"`
	ResolvedAt     sql.NullTime `db:"resolved_at"`
}

func (r incidentRecord) toIncident() audit.Incident {
	inc := audit.Incident{
		ID:             r.ID,
		GID:            r.GID,
		TenantID:       r.TenantID,
		Source:         audit.IncidentSource(r.Source),
		Status:         audit.IncidentStatus(r.Status),
		Severity:       audit.IncidentSeverity(r.Severity),
		IdempotencyKey: r.IdempotencyKey,
		DetectedSpamID: r.DetectedSpamID,
		ReportID:       r.ReportID,
		ReasonCode:     audit.ReasonCode(r.ReasonCode),
		ReasonText:     r.ReasonText,
		SpamUserID:     r.SpamUserID,
		SpamUserName:   r.SpamUserName,
		ChatID:         r.ChatID,
		MessageText:    r.MessageText,
		ResolvedBy:     r.ResolvedBy,
		Comment:        r.Comment,
		CreatedAt:      r.CreatedAt.Local(),
		UpdatedAt:      r.UpdatedAt.Local(),
	}
	if r.ResolvedAt.Valid {
		t := r.ResolvedAt.Time.Local()
		inc.ResolvedAt = &t
	}
	return inc
}

type commentRecord struct {
	ID         int64     `db:"id"`
	IncidentID int64     `db:"incident_id"`
	AuthorType string    `db:"author_type"`
	AuthorID   string    `db:"author_id"`
	Action     string    `db:"action"`
	Payload    string    `db:"payload"`
	CreatedAt  time.Time `db:"created_at"`
}

func (r commentRecord) toComment() audit.IncidentComment {
	return audit.IncidentComment{
		ID:         r.ID,
		IncidentID: r.IncidentID,
		AuthorType: r.AuthorType,
		AuthorID:   r.AuthorID,
		Action:     r.Action,
		Payload:    r.Payload,
		CreatedAt:  r.CreatedAt.Local(),
	}
}
