package storage

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/umputun/tg-spam/app/storage/engine"
)

type RetentionConfig struct {
	IncidentsTTL         time.Duration
	AppealsTTL           time.Duration
	DetectedSpamTTL      time.Duration
	IncomingEventsTTL    time.Duration
	ModerationActionsTTL time.Duration
	LabelsTTL            time.Duration
	CandidatesTTL        time.Duration
	UsageCountersTTL     time.Duration
	Interval             time.Duration
}

type RetentionService struct {
	db     *engine.SQL
	lock   engine.RWLocker
	config RetentionConfig
}

func NewRetentionService(db *engine.SQL, config RetentionConfig) *RetentionService {
	return &RetentionService{db: db, lock: db.MakeLock(), config: config}
}

func (s *RetentionService) Run(ctx context.Context) {
	if s.config.Interval <= 0 {
		log.Printf("[INFO] retention service disabled (no interval)")
		return
	}

	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	log.Printf("[INFO] retention service started, interval=%s", s.config.Interval)
	s.cleanAll(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[INFO] retention service stopped")
			return
		case <-ticker.C:
			s.cleanAll(ctx)
		}
	}
}

func (s *RetentionService) CleanNow(ctx context.Context) (map[string]int, error) {
	results := make(map[string]int)
	cleaners := s.buildCleaners()
	for _, c := range cleaners {
		deleted, err := s.cleanTable(ctx, c.table, c.column, c.ttl)
		if err != nil {
			return results, fmt.Errorf("clean %s: %w", c.table, err)
		}
		results[c.table] = deleted
	}
	return results, nil
}

type cleanSpec struct {
	table  string
	column string
	ttl    time.Duration
}

func (s *RetentionService) buildCleaners() []cleanSpec {
	var cleaners []cleanSpec
	add := func(table, column string, ttl time.Duration) {
		if ttl > 0 {
			cleaners = append(cleaners, cleanSpec{table: table, column: column, ttl: ttl})
		}
	}
	add("incidents", "created_at", s.config.IncidentsTTL)
	add("incident_comments", "created_at", s.config.IncidentsTTL)
	add("appeals", "created_at", s.config.AppealsTTL)
	add("detected_spam", "timestamp", s.config.DetectedSpamTTL)
	add("labels", "created_at", s.config.LabelsTTL)
	add("candidates", "created_at", s.config.CandidatesTTL)
	add("knowledge_snapshots", "created_at", s.config.LabelsTTL)
	add("incoming_events", "timestamp", s.config.IncomingEventsTTL)
	add("moderation_actions", "timestamp", s.config.ModerationActionsTTL)
	add("messages", "timestamp", s.config.IncomingEventsTTL)
	add("spam", "timestamp", s.config.IncomingEventsTTL)
	add("reports", "timestamp", s.config.IncomingEventsTTL)
	add("usage_counters", "window_start", s.config.UsageCountersTTL)
	return cleaners
}

func (s *RetentionService) cleanAll(ctx context.Context) {
	cleaners := s.buildCleaners()
	for _, c := range cleaners {
		deleted, err := s.cleanTable(ctx, c.table, c.column, c.ttl)
		if err != nil {
			log.Printf("[WARN] retention: clean %s failed: %v", c.table, err)
			continue
		}
		if deleted > 0 {
			log.Printf("[INFO] retention: cleaned %d rows from %s (ttl=%s)", deleted, c.table, c.ttl)
		}
	}
}

func (s *RetentionService) cleanTable(ctx context.Context, table, column string, ttl time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-ttl)

	s.lock.Lock()
	defer s.lock.Unlock()

	query := s.db.Adopt(fmt.Sprintf("DELETE FROM %s WHERE %s < ?", table, column))
	result, err := s.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, nil
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}
