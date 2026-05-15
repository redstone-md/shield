package slowpath

import (
	"sync"
	"time"
)

type BudgetConfig struct {
	MaxRequestsPerHour int
	MaxTokensPerHour   int
	MaxCostPerHour     float64
}

type budgetCounter struct {
	requests int
	tokens   int
	cost     float64
	resetAt  time.Time
}

type InMemoryBudgetTracker struct {
	mu       sync.RWMutex
	configs  map[string]BudgetConfig
	counters map[string]*budgetCounter
}

func NewInMemoryBudgetTracker() *InMemoryBudgetTracker {
	return &InMemoryBudgetTracker{
		configs:  make(map[string]BudgetConfig),
		counters: make(map[string]*budgetCounter),
	}
}

func (t *InMemoryBudgetTracker) SetConfig(tenantID string, cfg BudgetConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.configs[tenantID] = cfg
}

func (t *InMemoryBudgetTracker) Allow(tenantID string, _ BudgetClass, estimatedTokens int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	cfg, ok := t.configs[tenantID]
	if !ok {
		return true
	}

	counter := t.getCounter(tenantID)
	if time.Now().After(counter.resetAt) {
		counter.requests = 0
		counter.tokens = 0
		counter.cost = 0
		counter.resetAt = time.Now().Add(time.Hour)
	}

	if cfg.MaxRequestsPerHour > 0 && counter.requests >= cfg.MaxRequestsPerHour {
		return false
	}
	if cfg.MaxTokensPerHour > 0 && counter.tokens+estimatedTokens > cfg.MaxTokensPerHour {
		return false
	}
	if cfg.MaxCostPerHour > 0 && counter.cost >= cfg.MaxCostPerHour {
		return false
	}

	return true
}

func (t *InMemoryBudgetTracker) Record(tenantID string, _ BudgetClass, tokensUsed int, cost float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	counter := t.getCounter(tenantID)
	counter.requests++
	counter.tokens += tokensUsed
	counter.cost += cost
}

func (t *InMemoryBudgetTracker) Usage(tenantID string) (requests, tokens int, cost float64) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	counter, ok := t.counters[tenantID]
	if !ok {
		return 0, 0, 0
	}
	return counter.requests, counter.tokens, counter.cost
}

func (t *InMemoryBudgetTracker) Reset(tenantID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.counters, tenantID)
}

func (t *InMemoryBudgetTracker) getCounter(tenantID string) *budgetCounter {
	c, ok := t.counters[tenantID]
	if !ok {
		c = &budgetCounter{resetAt: time.Now().Add(time.Hour)}
		t.counters[tenantID] = c
	}
	return c
}
