package controlplane

import (
	"context"
	"strings"
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

func cacheKey(tenantID, workspaceID string) string {
	return tenantID + ":" + workspaceID
}

func (c *memoryCache) Get(_ context.Context, tenantID, workspaceID string) (rules.RuleSet, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := cacheKey(tenantID, workspaceID)
	entry, ok := c.ruleSets[key]
	if !ok {
		return rules.RuleSet{}, false
	}

	if time.Since(entry.cachedAt) > c.ttl {
		return rules.RuleSet{}, false
	}

	if invalidatedAt, ok := c.invalidated[key]; ok && !invalidatedAt.Before(entry.cachedAt) {
		return rules.RuleSet{}, false
	}

	return entry.ruleSet, true
}

func (c *memoryCache) Set(_ context.Context, tenantID, workspaceID string, ruleSet rules.RuleSet) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(tenantID, workspaceID)
	c.ruleSets[key] = cachedRuleSet{
		ruleSet:  ruleSet,
		cachedAt: time.Now(),
	}
}

func (c *memoryCache) Invalidate(_ context.Context, tenantID, workspaceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey(tenantID, workspaceID)
	c.invalidated[key] = time.Now()
}

func (c *memoryCache) InvalidateAll(_ context.Context, tenantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	prefix := tenantID + ":"
	for k := range c.ruleSets {
		if strings.HasPrefix(k, prefix) {
			delete(c.ruleSets, k)
		}
	}
	for k := range c.invalidated {
		if strings.HasPrefix(k, prefix) {
			delete(c.invalidated, k)
		}
	}
}
