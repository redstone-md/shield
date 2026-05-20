package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/rules"
	"github.com/redstone-md/shield/app/storage/engine"
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

func TestRuleSetsMigrateFromOldSchema(t *testing.T) {
	db, err := engine.NewSqlite(":memory:", "gr1")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS rule_sets (
		workspace_id TEXT PRIMARY KEY,
		active_version INTEGER NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS rule_set_versions (
		workspace_id TEXT NOT NULL,
		version INTEGER NOT NULL,
		source TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (workspace_id, version)
	)`)
	require.NoError(t, err)

	_, err = db.Exec("INSERT INTO rule_sets (workspace_id, active_version, updated_at) VALUES ('old-ws', 1, '2026-01-01')")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO rule_set_versions (workspace_id, version, source, payload, created_at) VALUES ('old-ws', 1, 'manual', '{}', '2026-01-01')")
	require.NoError(t, err)

	store, err := NewRuleSets(context.Background(), db)
	require.NoError(t, err)

	var gid string
	err = db.Get(&gid, "SELECT gid FROM rule_sets WHERE workspace_id = 'old-ws'")
	require.NoError(t, err)
	assert.Equal(t, "gr1", gid)

	var versionGID string
	err = db.Get(&versionGID, "SELECT gid FROM rule_set_versions WHERE workspace_id = 'old-ws' AND version = 1")
	require.NoError(t, err)
	assert.Equal(t, "gr1", versionGID)

	ruleSet := rules.RuleSet{
		WorkspaceID: "new-ws",
		Source:      "bootstrap",
	}
	created, err := store.EnsureBootstrap(context.Background(), ruleSet)
	require.NoError(t, err)
	assert.True(t, created)
}
