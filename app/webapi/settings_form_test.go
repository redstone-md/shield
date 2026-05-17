package webapi

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/rules"
)

func TestRuleSetFromForm_AppliesValues(t *testing.T) {
	base := rules.RuleSet{WorkspaceID: "tg-spam", Version: 7}
	form := url.Values{
		"detection.max_emoji":            {"5"},
		"detection.similarity_threshold": {"0.6"},
		"detection.cas_enabled":          {"on"},
		"llm.mode":                       {"flagged"},
		"llm.consensus":                  {"any"},
		"llm.vision_prompt":              {"scan image"},
		"openai.veto":                    {"on"},
		"openai.model":                   {"gpt-4o-mini"},
		"openai.prompt":                  {"be strict"},
		"slow_path_enabled":              {"on"},
	}

	rs, errs := ruleSetFromForm(base, form)

	require.Empty(t, errs)
	assert.Equal(t, "tg-spam", rs.WorkspaceID, "identity preserved")
	assert.Equal(t, 7, rs.Version, "version preserved")
	assert.Equal(t, 5, rs.Detection.MaxEmoji)
	assert.InEpsilon(t, 0.6, rs.Detection.SimilarityThreshold, 0.0001)
	assert.True(t, rs.Detection.CasEnabled)
	assert.Equal(t, "flagged", rs.LLM.Mode)
	assert.Equal(t, "scan image", rs.LLM.VisionPrompt)
	assert.True(t, rs.OpenAI.Veto)
	assert.Equal(t, "be strict", rs.OpenAI.Prompt)
	assert.True(t, rs.SlowPathEnabled)
}

func TestRuleSetFromForm_UncheckedCheckboxIsFalse(t *testing.T) {
	base := rules.RuleSet{Detection: rules.DetectionRules{CasEnabled: true}}
	rs, errs := ruleSetFromForm(base, url.Values{})
	require.Empty(t, errs)
	assert.False(t, rs.Detection.CasEnabled)
}

func TestRuleSetFromForm_InvalidIntRejected(t *testing.T) {
	_, errs := ruleSetFromForm(rules.RuleSet{}, url.Values{"detection.max_emoji": {"abc"}})
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "detection.max_emoji")
}

func TestRuleSetFromForm_NegativeThresholdRejected(t *testing.T) {
	_, errs := ruleSetFromForm(rules.RuleSet{}, url.Values{"detection.similarity_threshold": {"-1"}})
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "similarity_threshold")
}

func TestRuleSetFromForm_InvalidLLMModeRejected(t *testing.T) {
	_, errs := ruleSetFromForm(rules.RuleSet{}, url.Values{"llm.mode": {"bogus"}})
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "llm.mode")
}
