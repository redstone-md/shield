package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
)

func TestRuleSetService_Get(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewRuleSets(context.Background(), db)
	require.NoError(t, err)

	svc := NewRuleSetService(store)

	_, err = svc.Get(context.Background(), "gr1")
	assert.Error(t, err, "no rule set yet")

	_, err = store.EnsureBootstrap(context.Background(), rules.RuleSet{
		WorkspaceID: "gr1",
		Source:      "bootstrap",
		Meta:        rules.MetaRules{LinksLimit: 3},
	})
	require.NoError(t, err)

	rs, err := svc.Get(context.Background(), "gr1")
	require.NoError(t, err)
	assert.Equal(t, "gr1", rs.WorkspaceID)
	assert.Equal(t, 3, rs.Meta.LinksLimit)
	assert.Equal(t, 1, rs.Version)

	rs2, err := svc.Get(context.Background(), "gr1")
	require.NoError(t, err)
	assert.Equal(t, rs.Version, rs2.Version, "should return cached value")
}

func TestRuleSetService_Update(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewRuleSets(context.Background(), db)
	require.NoError(t, err)

	_, err = store.EnsureBootstrap(context.Background(), rules.RuleSet{
		WorkspaceID: "gr1",
		Source:      "bootstrap",
		Meta:        rules.MetaRules{LinksLimit: 1},
	})
	require.NoError(t, err)

	svc := NewRuleSetService(store)

	updated, err := svc.Update(context.Background(), "gr1", "api", rules.RuleSet{
		Meta: rules.MetaRules{LinksLimit: 5},
		Moderation: rules.ModerationRules{
			FirstStrike:  10 * time.Minute,
			SecondStrike: time.Hour,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Version)
	assert.Equal(t, "api", updated.Source)
	assert.Equal(t, 5, updated.Meta.LinksLimit)

	cached, err := svc.Get(context.Background(), "gr1")
	require.NoError(t, err)
	assert.Equal(t, 2, cached.Version, "cache should be updated after Update")
}

func TestRuleSetService_UsesCacheAndInvalidatesOnUpdate(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewRuleSets(context.Background(), db)
	require.NoError(t, err)

	_, err = store.EnsureBootstrap(context.Background(), rules.RuleSet{
		WorkspaceID: "gr1",
		Source:      "bootstrap",
		Meta:        rules.MetaRules{LinksLimit: 1},
	})
	require.NoError(t, err)

	cache := newMemoryCache(time.Hour)
	svc := NewRuleSetServiceWithCache(store, cache)

	first, err := svc.Get(context.Background(), "gr1")
	require.NoError(t, err)
	assert.Equal(t, 1, first.Version)

	_, err = store.Update(context.Background(), rules.RuleSet{
		WorkspaceID: "gr1",
		Source:      "external",
		Meta:        rules.MetaRules{LinksLimit: 9},
	})
	require.NoError(t, err)

	cached, err := svc.Get(context.Background(), "gr1")
	require.NoError(t, err)
	assert.Equal(t, 1, cached.Version, "service should keep serving cached active rules until invalidated")
	assert.Equal(t, 1, cached.Meta.LinksLimit)

	svc.Invalidate()
	reloaded, err := svc.Get(context.Background(), "gr1")
	require.NoError(t, err)
	assert.Equal(t, 2, reloaded.Version)
	assert.Equal(t, 9, reloaded.Meta.LinksLimit)

	_, err = svc.Update(context.Background(), "gr1", "api", rules.RuleSet{
		Meta: rules.MetaRules{LinksLimit: 11},
	})
	require.NoError(t, err)

	afterUpdate, err := svc.Get(context.Background(), "gr1")
	require.NoError(t, err)
	assert.Equal(t, 3, afterUpdate.Version)
	assert.Equal(t, 11, afterUpdate.Meta.LinksLimit)
}

func TestRuleSetService_OnChange(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewRuleSets(context.Background(), db)
	require.NoError(t, err)

	_, err = store.EnsureBootstrap(context.Background(), rules.RuleSet{
		WorkspaceID: "gr1",
		Source:      "bootstrap",
	})
	require.NoError(t, err)

	svc := NewRuleSetService(store)

	var notified rules.RuleSet
	svc.OnChange(func(rs rules.RuleSet) {
		notified = rs
	})

	_, err = svc.Update(context.Background(), "gr1", "api", rules.RuleSet{
		Meta: rules.MetaRules{LinksLimit: 7},
	})
	require.NoError(t, err)

	assert.Equal(t, "gr1", notified.WorkspaceID)
	assert.Equal(t, 2, notified.Version)
	assert.Equal(t, 7, notified.Meta.LinksLimit)
}

func TestRuleSetService_Invalidate(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := storage.NewRuleSets(context.Background(), db)
	require.NoError(t, err)

	_, err = store.EnsureBootstrap(context.Background(), rules.RuleSet{
		WorkspaceID: "gr1",
		Source:      "bootstrap",
		Meta:        rules.MetaRules{LinksLimit: 2},
	})
	require.NoError(t, err)

	svc := NewRuleSetService(store)

	rs, err := svc.Get(context.Background(), "gr1")
	require.NoError(t, err)
	assert.Equal(t, 1, rs.Version)

	svc.Invalidate()

	rs2, err := svc.Get(context.Background(), "gr1")
	require.NoError(t, err)
	assert.Equal(t, 2, rs2.Meta.LinksLimit, "after invalidate, should reload from store")
}
