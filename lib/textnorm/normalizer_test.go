package textnorm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizerNormalize(t *testing.T) {
	n := Default()
	assert.Equal(t, "hello world test", n.Normalize("  HeLLo\u200B   WORLD \n test\u2062 "))
}

func TestNormalizerScriptFoldHook(t *testing.T) {
	n := New(Options{
		LowerCase:           true,
		Trim:                true,
		CanonicalWhitespace: true,
		ScriptFold: func(text string) string {
			return strings.ReplaceAll(text, "@", "a")
		},
	})
	assert.Equal(t, "a value", n.Normalize("@ Value"))
}
