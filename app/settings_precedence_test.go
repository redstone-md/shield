package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigured_EmptyEnvIsNotSet(t *testing.T) {
	t.Setenv("MAX_EMOJI", "")
	assert.False(t, configured("max-emoji", "MAX_EMOJI"),
		"an env var present but empty must count as not configured")
}

func TestConfigured_NonEmptyEnvIsSet(t *testing.T) {
	t.Setenv("MAX_EMOJI", "5")
	assert.True(t, configured("max-emoji", "MAX_EMOJI"))
}
