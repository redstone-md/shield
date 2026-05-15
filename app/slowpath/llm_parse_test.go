package slowpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLLMOutputValid(t *testing.T) {
	resp, err := parseLLMOutput(`{"spam":true,"reason":"crypto","confidence":95}`)
	require.NoError(t, err)
	assert.True(t, resp.IsSpam)
	assert.Equal(t, "crypto", resp.Reason)
	assert.Equal(t, 95, resp.Confidence)
}

func TestParseLLMOutputHam(t *testing.T) {
	resp, err := parseLLMOutput(`{"spam":false,"reason":"clean","confidence":10}`)
	require.NoError(t, err)
	assert.False(t, resp.IsSpam)
}

func TestParseLLMOutputTrailingComma(t *testing.T) {
	resp, err := parseLLMOutput(`{"spam":true,"reason":"spam","confidence":90,}`)
	require.NoError(t, err)
	assert.True(t, resp.IsSpam)
}

func TestParseLLMOutputWithThoughtTags(t *testing.T) {
	input := `<think let me analyze this</think {"spam":false,"reason":"ok","confidence":20}`
	resp, err := parseLLMOutput(input)
	require.NoError(t, err)
	assert.False(t, resp.IsSpam)
}

func TestParseLLMOutputFallbackRegex(t *testing.T) {
	input := `The message is spam: true, reason: "crypto ad", confidence: 85`
	resp, err := parseLLMOutput(input)
	require.NoError(t, err)
	assert.True(t, resp.IsSpam)
	assert.Equal(t, 85, resp.Confidence)
}

func TestParseLLMOutputEmpty(t *testing.T) {
	_, err := parseLLMOutput("")
	assert.Error(t, err)
}

func TestParseLLMOutputGarbage(t *testing.T) {
	_, err := parseLLMOutput("not json at all")
	assert.Error(t, err)
}

func TestExtractFirstJSON(t *testing.T) {
	input := `Some text {"spam":true,"reason":"test","confidence":50} more text`
	result := extractFirstJSON(input)
	assert.JSONEq(t, `{"spam":true,"reason":"test","confidence":50}`, result)
}

func TestExtractFirstJSONNested(t *testing.T) {
	input := `{"outer": {"spam":true},"reason":"x","confidence":1}`
	result := extractFirstJSON(input)
	assert.Equal(t, input, result)
}

func TestFixTrailingComma(t *testing.T) {
	result := fixTrailingComma(`{"a":1,}`)
	assert.Equal(t, `{"a":1}`, result)
}

func TestParseBoolValues(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
		ok       bool
	}{
		{"true", true, true},
		{"false", false, true},
		{"1", true, true},
		{"0", false, true},
		{`"true"`, true, true},
		{"maybe", false, false},
	}
	for _, tc := range cases {
		got, ok := parseBool(tc.input)
		assert.Equal(t, tc.ok, ok, "input: %s", tc.input)
		if ok {
			assert.Equal(t, tc.expected, got, "input: %s", tc.input)
		}
	}
}
