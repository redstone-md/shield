package webapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_EnvPinnedKeysField(t *testing.T) {
	cfg := Config{EnvPinnedKeys: map[string]bool{"detection.max_emoji": true}}
	assert.True(t, cfg.EnvPinnedKeys["detection.max_emoji"])
	assert.False(t, cfg.EnvPinnedKeys["detection.min_msg_len"])
}

func TestTemplateDictHelper(t *testing.T) {
	m, err := templateDict("A", 1, "B", "two")
	require.NoError(t, err)
	assert.Equal(t, 1, m["A"])
	assert.Equal(t, "two", m["B"])

	_, err = templateDict("oddNumberOfArgs")
	require.Error(t, err)
}
