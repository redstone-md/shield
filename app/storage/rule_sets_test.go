package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage/engine"
)

func TestRuleSetsBootstrap(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	store, err := NewRuleSets(context.Background(), db)
	require.NoError(t, err)

	ruleSet := rules.RuleSet{
		WorkspaceID: "gr1",
		Source:      "bootstrap",
		Meta: rules.MetaRules{
			LinksLimit: 2,
		},
		Moderation: rules.ModerationRules{
			FirstStrike:  30 * time.Minute,
			SecondStrike: 6 * time.Hour,
		},
	}

	created, err := store.EnsureBootstrap(context.Background(), ruleSet)
	require.NoError(t, err)
	assert.True(t, created)

	created, err = store.EnsureBootstrap(context.Background(), ruleSet)
	require.NoError(t, err)
	assert.False(t, created)

	active, err := store.Active(context.Background(), "gr1")
	require.NoError(t, err)
	assert.Equal(t, "gr1", active.WorkspaceID)
	assert.Equal(t, 1, active.Version)
	assert.Equal(t, "bootstrap", active.Source)
	assert.Equal(t, 2, active.Meta.LinksLimit)
	assert.Equal(t, 30*time.Minute, active.Moderation.FirstStrike)
}
