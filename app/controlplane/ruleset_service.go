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
	mu          sync.RWMutex
	subscribers []func(rules.RuleSet)
}

type RuleSetCache interface {
	Get(ctx context.Context, workspaceID string) (rules.RuleSet, bool)
	Set(ctx context.Context, workspaceID string, ruleSet rules.RuleSet)
	Invalidate(ctx context.Context, workspaceID string)
	InvalidateAll(ctx context.Context)
}

func NewRuleSetService(store *storage.RuleSets) *RuleSetService {
	return NewRuleSetServiceWithCache(store, newMemoryCache(5*time.Minute))
}

func NewRuleSetServiceWithCache(store *storage.RuleSets, cache RuleSetCache) *RuleSetService {
	if cache == nil {
		cache = newMemoryCache(5 * time.Minute)
	}
	return &RuleSetService{store: store, cache: cache}
}

func (s *RuleSetService) Get(ctx context.Context, workspaceID string) (rules.RuleSet, error) {
	if cached, ok := s.cache.Get(ctx, workspaceID); ok {
		return cached, nil
	}

	rs, err := s.store.Active(ctx, workspaceID)
	if err != nil {
		return rules.RuleSet{}, fmt.Errorf("failed to load active rule set: %w", err)
	}

	s.cache.Set(ctx, workspaceID, rs)
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

	s.cache.Invalidate(ctx, workspaceID)

	s.notify(updated)
	log.Printf("[INFO] rule set updated: workspace=%s version=%d source=%s", workspaceID, updated.Version, source)
	return updated, nil
}

func (s *RuleSetService) OnChange(fn func(rules.RuleSet)) {
	s.mu.Lock()
	s.subscribers = append(s.subscribers, fn)
	s.mu.Unlock()
}

func (s *RuleSetService) Invalidate() {
	s.cache.InvalidateAll(context.Background())
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
