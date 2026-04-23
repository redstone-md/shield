package controlplane

import (
	"context"
	"sync"
	"time"

	"github.com/umputun/tg-spam/app/rules"
)

type memoryCache struct {
	mu          sync.RWMutex
	ruleSets    map[string]cachedRuleSet
	ttl         time.Duration
	invalidated map[string]time.Time
}

type cachedRuleSet struct {
	ruleSet  rules.RuleSet
	cachedAt time.Time
}

func newMemoryCache(ttl time.Duration) *memoryCache {
	return &memoryCache{
		ruleSets:    make(map[string]cachedRuleSet),
		ttl:         ttl,
		invalidated: make(map[string]time.Time),
	}
}

func (c *memoryCache) Get(ctx context.Context, workspaceID string) (rules.RuleSet, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.ruleSets[workspaceID]
	if !ok {
		return rules.RuleSet{}, false
	}

	if time.Since(entry.cachedAt) > c.ttl {
		return rules.RuleSet{}, false
	}

	if invalidatedAt, ok := c.invalidated[workspaceID]; ok && invalidatedAt.After(entry.cachedAt) {
		return rules.RuleSet{}, false
	}

	return entry.ruleSet, true
}

func (c *memoryCache) Set(ctx context.Context, workspaceID string, ruleSet rules.RuleSet) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ruleSets[workspaceID] = cachedRuleSet{
		ruleSet:  ruleSet,
		cachedAt: time.Now(),
	}
}

func (c *memoryCache) Invalidate(ctx context.Context, workspaceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.invalidated[workspaceID] = time.Now()
}
