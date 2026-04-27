package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/umputun/tg-spam/app/rules"
)

func TestMemoryCache_TenantIsolation(t *testing.T) {
	c := newMemoryCache(time.Hour)
	ctx := context.Background()

	rsA := rules.RuleSet{WorkspaceID: "ws1", Version: 1}
	rsB := rules.RuleSet{WorkspaceID: "ws1", Version: 2}

	c.Set(ctx, "tenantA", "ws1", rsA)
	c.Set(ctx, "tenantB", "ws1", rsB)

	gotA, okA := c.Get(ctx, "tenantA", "ws1")
	assert.True(t, okA)
	assert.Equal(t, 1, gotA.Version)

	gotB, okB := c.Get(ctx, "tenantB", "ws1")
	assert.True(t, okB)
	assert.Equal(t, 2, gotB.Version)
}

func TestMemoryCache_InvalidateAll_ScopedToTenant(t *testing.T) {
	c := newMemoryCache(time.Hour)
	ctx := context.Background()

	c.Set(ctx, "tenantA", "ws1", rules.RuleSet{Version: 1})
	c.Set(ctx, "tenantA", "ws2", rules.RuleSet{Version: 2})
	c.Set(ctx, "tenantB", "ws1", rules.RuleSet{Version: 3})

	c.InvalidateAll(ctx, "tenantA")

	_, okA1 := c.Get(ctx, "tenantA", "ws1")
	assert.False(t, okA1, "tenantA ws1 should be invalidated")

	_, okA2 := c.Get(ctx, "tenantA", "ws2")
	assert.False(t, okA2, "tenantA ws2 should be invalidated")

	gotB, okB := c.Get(ctx, "tenantB", "ws1")
	assert.True(t, okB, "tenantB ws1 should remain")
	assert.Equal(t, 3, gotB.Version)
}

func TestMemoryCache_Invalidate_SingleKey(t *testing.T) {
	c := newMemoryCache(time.Hour)
	ctx := context.Background()

	c.Set(ctx, "t1", "ws1", rules.RuleSet{Version: 1})
	c.Set(ctx, "t1", "ws2", rules.RuleSet{Version: 2})

	c.Invalidate(ctx, "t1", "ws1")

	_, ok1 := c.Get(ctx, "t1", "ws1")
	assert.False(t, ok1, "ws1 should be invalidated")

	got2, ok2 := c.Get(ctx, "t1", "ws2")
	assert.True(t, ok2, "ws2 should remain")
	assert.Equal(t, 2, got2.Version)
}

func TestMemoryCache_SameWorkspace_DifferentTenants(t *testing.T) {
	c := newMemoryCache(100 * time.Millisecond)
	ctx := context.Background()

	c.Set(ctx, "t1", "ws1", rules.RuleSet{Version: 10})

	_, ok := c.Get(ctx, "t2", "ws1")
	assert.False(t, ok, "different tenant should not see t1's cache")
}
