package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/umputun/tg-spam/app/feedback"
	"github.com/umputun/tg-spam/app/storage/engine"
)

const (
	CmdCreateKnowledgeSnapshotsTable engine.DBCmd = iota + 900
	CmdCreateKnowledgeSnapshotsIndexes
	CmdInsertKnowledgeSnapshot
	CmdGetKnowledgeSnapshotByID
	CmdListKnowledgeSnapshots
)

var knowledgeSnapshotsQueries = engine.NewQueryMap().
	Add(CmdCreateKnowledgeSnapshotsTable, engine.Query{
		Sqlite: `CREATE TABLE IF NOT EXISTS knowledge_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			version INTEGER DEFAULT 0,
			data_json TEXT NOT NULL DEFAULT '{}',
			created_by TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		Postgres: `CREATE TABLE IF NOT EXISTS knowledge_snapshots (
			id SERIAL PRIMARY KEY,
			gid TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT '',
			version INTEGER DEFAULT 0,
			data_json TEXT NOT NULL DEFAULT '{}',
			created_by TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}).
	AddSame(CmdCreateKnowledgeSnapshotsIndexes, `
		CREATE INDEX IF NOT EXISTS idx_knowledge_snapshots_tenant ON knowledge_snapshots(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_knowledge_snapshots_created ON knowledge_snapshots(tenant_id, created_at DESC);
	`).
	Add(CmdInsertKnowledgeSnapshot, engine.Query{
		Sqlite:   `INSERT INTO knowledge_snapshots (gid, tenant_id, version, data_json, created_by) VALUES (?, ?, ?, ?, ?)`,
		Postgres: `INSERT INTO knowledge_snapshots (gid, tenant_id, version, data_json, created_by) VALUES ($1, $2, $3, $4, $5)`,
	}).
	AddSame(CmdGetKnowledgeSnapshotByID, `SELECT * FROM knowledge_snapshots WHERE tenant_id = ? AND id = ?`).
	Add(CmdListKnowledgeSnapshots, engine.Query{
		Sqlite:   `SELECT * FROM knowledge_snapshots WHERE tenant_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		Postgres: `SELECT * FROM knowledge_snapshots WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
	})

type KnowledgeSnapshotStorage struct {
	*engine.SQL
	engine.RWLocker
}

func NewKnowledgeSnapshotStorage(ctx context.Context, db *engine.SQL) (*KnowledgeSnapshotStorage, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}
	res := &KnowledgeSnapshotStorage{SQL: db, RWLocker: db.MakeLock()}
	cfg := engine.TableConfig{
		Name:          "knowledge_snapshots",
		CreateTable:   CmdCreateKnowledgeSnapshotsTable,
		CreateIndexes: CmdCreateKnowledgeSnapshotsIndexes,
		MigrateFunc:   func(_ context.Context, _ *sqlx.Tx, _ string) error { return nil },
		QueriesMap:    knowledgeSnapshotsQueries,
	}
	if err := engine.InitTable(ctx, db, cfg); err != nil {
		return nil, fmt.Errorf("failed to init knowledge_snapshots storage: %w", err)
	}
	return res, nil
}

func (s *KnowledgeSnapshotStorage) Create(
	ctx context.Context, snap feedback.KnowledgeSnapshot,
) (feedback.KnowledgeSnapshot, error) {
	s.Lock()
	defer s.Unlock()

	query, err := knowledgeSnapshotsQueries.Pick(s.Type(), CmdInsertKnowledgeSnapshot)
	if err != nil {
		return feedback.KnowledgeSnapshot{}, fmt.Errorf("failed to get insert query: %w", err)
	}
	query = s.Adopt(query)

	res, execErr := s.ExecContext(ctx, query,
		s.GID(), s.TenantID(), 0, snap.DataJSON, snap.CreatedBy,
	)
	if execErr != nil {
		return feedback.KnowledgeSnapshot{}, fmt.Errorf("failed to insert snapshot: %w", execErr)
	}

	id, _ := res.LastInsertId()
	snap.ID = id
	snap.GID = s.GID()
	snap.TenantID = s.TenantID()
	snap.CreatedAt = time.Now()
	return snap, nil
}

func (s *KnowledgeSnapshotStorage) GetByID(ctx context.Context, id int64) (feedback.KnowledgeSnapshot, error) {
	s.RLock()
	defer s.RUnlock()

	query, err := knowledgeSnapshotsQueries.Pick(s.Type(), CmdGetKnowledgeSnapshotByID)
	if err != nil {
		return feedback.KnowledgeSnapshot{}, fmt.Errorf("failed to get query: %w", err)
	}
	query = s.Adopt(query)

	var snap feedback.KnowledgeSnapshot
	if qErr := s.GetContext(ctx, &snap, query, s.TenantID(), id); qErr != nil {
		return feedback.KnowledgeSnapshot{}, fmt.Errorf("snapshot not found: %w", qErr)
	}
	return snap, nil
}

func (s *KnowledgeSnapshotStorage) List(ctx context.Context, limit, offset int) ([]feedback.KnowledgeSnapshot, error) {
	s.RLock()
	defer s.RUnlock()

	query, err := knowledgeSnapshotsQueries.Pick(s.Type(), CmdListKnowledgeSnapshots)
	if err != nil {
		return nil, fmt.Errorf("failed to get query: %w", err)
	}
	query = s.Adopt(query)

	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var snaps []feedback.KnowledgeSnapshot
	if qErr := s.SelectContext(ctx, &snaps, query, s.TenantID(), limit, offset); qErr != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", qErr)
	}
	return snaps, nil
}
