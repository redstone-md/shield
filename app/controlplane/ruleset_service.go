package controlplane

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage"
)

type RuleSetService struct {
	store       *storage.RuleSets
	mu          sync.RWMutex
	cached      rules.RuleSet
	subscribers []func(rules.RuleSet)
}

func NewRuleSetService(store *storage.RuleSets) *RuleSetService {
	return &RuleSetService{store: store}
}

func (s *RuleSetService) Get(ctx context.Context, workspaceID string) (rules.RuleSet, error) {
	s.mu.RLock()
	if s.cached.WorkspaceID == workspaceID && s.cached.Version > 0 {
		cached := s.cached
		s.mu.RUnlock()
		return cached, nil
	}
	s.mu.RUnlock()

	rs, err := s.store.Active(ctx, workspaceID)
	if err != nil {
		return rules.RuleSet{}, fmt.Errorf("failed to load active rule set: %w", err)
	}

	s.mu.Lock()
	s.cached = rs
	s.mu.Unlock()
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

	s.mu.Lock()
	s.cached = updated
	s.mu.Unlock()

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
	s.mu.Lock()
	s.cached = rules.RuleSet{}
	s.mu.Unlock()
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
