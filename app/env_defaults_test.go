package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnsetEmptyOptionEnv(t *testing.T) {
	t.Setenv("MAX_EMOJI", "")
	t.Setenv("OPENAI_VETO", "")
	t.Setenv("TELEGRAM_TOKEN", "")
	t.Setenv("LLM_MODE", "flagged")

	restore := unsetEmptyOptionEnv(options{})

	_, ok := os.LookupEnv("MAX_EMOJI")
	assert.False(t, ok)
	_, ok = os.LookupEnv("OPENAI_VETO")
	assert.False(t, ok)
	_, ok = os.LookupEnv("TELEGRAM_TOKEN")
	assert.False(t, ok)
	assert.Equal(t, "flagged", os.Getenv("LLM_MODE"))

	restore()

	val, ok := os.LookupEnv("MAX_EMOJI")
	assert.True(t, ok)
	assert.Empty(t, val)
	val, ok = os.LookupEnv("OPENAI_VETO")
	assert.True(t, ok)
	assert.Empty(t, val)
	val, ok = os.LookupEnv("TELEGRAM_TOKEN")
	assert.True(t, ok)
	assert.Empty(t, val)
}

func TestCollectOptionEnvKeys(t *testing.T) {
	keys := make(map[string]struct{})
	collectOptionEnvKeys(reflect.TypeFor[options](), "", keys)

	_, hasTopLevel := keys["MAX_EMOJI"]
	_, hasNested := keys["OPENAI_VETO"]
	_, hasDeepNested := keys["LLM_MIN_INPUT_CHARS"]

	assert.True(t, hasTopLevel)
	assert.True(t, hasNested)
	assert.True(t, hasDeepNested)
}

func TestParserIgnoresEmptyOptionEnv(t *testing.T) {
	t.Setenv("MAX_EMOJI", "")

	var opts options
	restore := unsetEmptyOptionEnv(opts)
	defer restore()

	_, err := flags.NewParser(&opts, flags.None).ParseArgs([]string{})
	require.NoError(t, err)
	assert.Equal(t, 2, opts.MaxEmoji)
}
