package observability

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatFields_RendersStructFieldsOnSeparateLines(t *testing.T) {
	cfg := sampleConfig{
		MaxTokens:       8192,
		Model:           "root/auto:fast",
		CustomPrompts:   []string{"one", "two"},
		RequestTimeout:  30 * time.Second,
		OptionalBackend: &sampleBackend{},
		Nested: sampleNestedConfig{
			Enabled: true,
			Limit:   3,
		},
	}

	assert.Equal(t, `MaxTokens: 8192
Model: root/auto:fast
CustomPrompts: [one two]
RequestTimeout: 30s
OptionalBackend: *observability.sampleBackend
Nested:
  Enabled: true
  Limit: 3`, FormatFields(cfg))
}

func TestFormatFields_FallsBackForNonStruct(t *testing.T) {
	assert.Equal(t, "plain", FormatFields("plain"))
}

type sampleConfig struct {
	MaxTokens       int
	Model           string
	CustomPrompts   []string
	RequestTimeout  time.Duration
	OptionalBackend *sampleBackend
	Nested          sampleNestedConfig
}

type sampleNestedConfig struct {
	Enabled bool
	Limit   int
}

type sampleBackend struct{}
