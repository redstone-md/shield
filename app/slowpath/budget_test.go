package slowpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInMemoryBudgetTrackerAllow(t *testing.T) {
	bt := NewInMemoryBudgetTracker()
	bt.SetConfig("t1", BudgetConfig{MaxRequestsPerHour: 10, MaxTokensPerHour: 1000})

	assert.True(t, bt.Allow("t1", BudgetClassStandard, 100))
	assert.True(t, bt.Allow("t1", BudgetClassStandard, 100))
}

func TestInMemoryBudgetTrackerDenyRequests(t *testing.T) {
	bt := NewInMemoryBudgetTracker()
	bt.SetConfig("t1", BudgetConfig{MaxRequestsPerHour: 2})

	assert.True(t, bt.Allow("t1", BudgetClassStandard, 0))
	bt.Record("t1", BudgetClassStandard, 10, 0.01)
	assert.True(t, bt.Allow("t1", BudgetClassStandard, 0))
	bt.Record("t1", BudgetClassStandard, 10, 0.01)
	assert.False(t, bt.Allow("t1", BudgetClassStandard, 0))
}

func TestInMemoryBudgetTrackerDenyTokens(t *testing.T) {
	bt := NewInMemoryBudgetTracker()
	bt.SetConfig("t1", BudgetConfig{MaxRequestsPerHour: 100, MaxTokensPerHour: 50})

	assert.False(t, bt.Allow("t1", BudgetClassStandard, 100))
}

func TestInMemoryBudgetTrackerNoConfig(t *testing.T) {
	bt := NewInMemoryBudgetTracker()
	assert.True(t, bt.Allow("unknown", BudgetClassStandard, 1000))
}

func TestInMemoryBudgetTrackerRecord(t *testing.T) {
	bt := NewInMemoryBudgetTracker()
	bt.SetConfig("t1", BudgetConfig{MaxRequestsPerHour: 100, MaxTokensPerHour: 10000})

	bt.Record("t1", BudgetClassStandard, 500, 0.05)
	bt.Record("t1", BudgetClassHigh, 300, 0.03)

	reqs, tokens, cost := bt.Usage("t1")
	assert.Equal(t, 2, reqs)
	assert.Equal(t, 800, tokens)
	assert.InDelta(t, 0.08, cost, 0.001)
}

func TestInMemoryBudgetTrackerUsageUnknown(t *testing.T) {
	bt := NewInMemoryBudgetTracker()
	reqs, tokens, cost := bt.Usage("unknown")
	assert.Equal(t, 0, reqs)
	assert.Equal(t, 0, tokens)
	assert.InDelta(t, 0.0, cost, 0.001)
}

func TestInMemoryBudgetTrackerReset(t *testing.T) {
	bt := NewInMemoryBudgetTracker()
	bt.SetConfig("t1", BudgetConfig{MaxRequestsPerHour: 1})

	bt.Record("t1", BudgetClassStandard, 100, 0.01)
	assert.False(t, bt.Allow("t1", BudgetClassStandard, 0))

	bt.Reset("t1")
	assert.True(t, bt.Allow("t1", BudgetClassStandard, 0))
}

func TestInMemoryBudgetTrackerUpdateConfig(t *testing.T) {
	bt := NewInMemoryBudgetTracker()
	bt.SetConfig("t1", BudgetConfig{MaxRequestsPerHour: 1})

	bt.Record("t1", BudgetClassStandard, 100, 0.01)
	assert.False(t, bt.Allow("t1", BudgetClassStandard, 0))

	bt.SetConfig("t1", BudgetConfig{MaxRequestsPerHour: 10})
	assert.True(t, bt.Allow("t1", BudgetClassStandard, 0))
}
