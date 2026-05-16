package webapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_EnvPinnedKeysField(t *testing.T) {
	cfg := Config{EnvPinnedKeys: map[string]bool{"detection.max_emoji": true}}
	assert.True(t, cfg.EnvPinnedKeys["detection.max_emoji"])
	assert.False(t, cfg.EnvPinnedKeys["detection.min_msg_len"])
}
