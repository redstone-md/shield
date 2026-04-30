package controlplane

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage"
)

type RuleSetService struct {
	store       *storage.RuleSets
	cache       RuleSetCache
	tenantID    string
	mu          sync.RWMutex
	subscribers []func(rules.RuleSet)
}

type RuleSetCache interface {
	Get(ctx context.Context, tenantID, workspaceID string) (rules.RuleSet, bool)
	Set(ctx context.Context, tenantID, workspaceID string, ruleSet rules.RuleSet)
	Invalidate(ctx context.Context, tenantID, workspaceID string)
	InvalidateAll(ctx context.Context, tenantID string)
}

func NewRuleSetService(store *storage.RuleSets, tenantID string) *RuleSetService {
	return NewRuleSetServiceWithCache(store, tenantID, newMemoryCache(5*time.Minute))
}

func NewRuleSetServiceWithCache(store *storage.RuleSets, tenantID string, cache RuleSetCache) *RuleSetService {
	if cache == nil {
		cache = newMemoryCache(5 * time.Minute)
	}
	return &RuleSetService{store: store, cache: cache, tenantID: tenantID}
}

func (s *RuleSetService) Get(ctx context.Context, workspaceID string) (rules.RuleSet, error) {
	if cached, ok := s.cache.Get(ctx, s.tenantID, workspaceID); ok {
		return cached, nil
	}

	rs, err := s.store.Active(ctx, workspaceID)
	if err != nil {
		return rules.RuleSet{}, fmt.Errorf("failed to load active rule set: %w", err)
	}

	s.cache.Set(ctx, s.tenantID, workspaceID, rs)
	return rs, nil
}

func (s *RuleSetService) Update(ctx context.Context, workspaceID string, source string, rs rules.RuleSet) (rules.RuleSet, error) {
	rs.WorkspaceID = workspaceID
	rs.Source = source

	newVersion, err := s.store.Update(ctx, rs)
	if err != nil {
		return rules.RuleSet{}, fmt.Errorf("failed to update rule set: %w", err)
	}

	updated, err := s.store.Active(ctx, workspaceID)
	if err != nil {
		return rules.RuleSet{}, fmt.Errorf("failed to reload rule set after update (version %d): %w", newVersion, err)
	}

	s.cache.Invalidate(ctx, s.tenantID, workspaceID)

	s.notify(updated)
	log.Printf("[INFO] rule set updated: workspace=%s version=%d source=%s", workspaceID, updated.Version, source)
	return updated, nil
}

func (s *RuleSetService) OnChange(fn func(rules.RuleSet)) {
	s.mu.Lock()
	s.subscribers = append(s.subscribers, fn)
	s.mu.Unlock()
}

func (s *RuleSetService) Cache() RuleSetCache {
	return s.cache
}

func (s *RuleSetService) Invalidate(ctx context.Context) {
	s.cache.InvalidateAll(ctx, s.tenantID)
}

func (s *RuleSetService) notify(rs rules.RuleSet) {
	s.mu.RLock()
	subs := make([]func(rules.RuleSet), len(s.subscribers))
	copy(subs, s.subscribers)
	s.mu.RUnlock()

	for _, fn := range subs {
		fn(rs)
	}
}
