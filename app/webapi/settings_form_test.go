package webapi

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/app/rules"
)

func TestRuleSetFromForm_AppliesValues(t *testing.T) {
	base := rules.RuleSet{WorkspaceID: "tg-spam", Version: 7}
	form := url.Values{
		"detection.max_emoji":            {"5"},
		"detection.similarity_threshold": {"0.6"},
		"detection.cas_enabled":          {"on"},
		"detection.chinese_mode":         {"on"},
		"detection.chinese_char_ratio":   {"0.5"},
		"llm.mode":                       {"flagged"},
		"llm.consensus":                  {"any"},
		"llm.history_context_size":       {"5"},
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
	assert.True(t, rs.Detection.ChineseMode)
	assert.InEpsilon(t, 0.5, rs.Detection.ChineseCharRatio, 0.0001)
	assert.Equal(t, "flagged", rs.LLM.Mode)
	assert.Equal(t, 5, rs.LLM.HistoryContextSize)
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

func TestRuleSetFromForm_DCList(t *testing.T) {
	t.Run("parses comma list", func(t *testing.T) {
		rs, errs := ruleSetFromForm(rules.RuleSet{}, url.Values{"join_gate.banned_dcs": {"2,4"}})
		require.Empty(t, errs)
		assert.Equal(t, []int{2, 4}, rs.JoinGate.BannedDCs)
	})
	t.Run("trims surrounding spaces", func(t *testing.T) {
		rs, errs := ruleSetFromForm(rules.RuleSet{}, url.Values{"join_gate.banned_dcs": {" 2 , 4 "}})
		require.Empty(t, errs)
		assert.Equal(t, []int{2, 4}, rs.JoinGate.BannedDCs)
	})
	t.Run("empty disables gate", func(t *testing.T) {
		base := rules.RuleSet{JoinGate: rules.JoinGateRules{BannedDCs: []int{2}}}
		rs, errs := ruleSetFromForm(base, url.Values{"join_gate.banned_dcs": {""}})
		require.Empty(t, errs)
		assert.Nil(t, rs.JoinGate.BannedDCs)
	})
	t.Run("absent preserves base", func(t *testing.T) {
		base := rules.RuleSet{JoinGate: rules.JoinGateRules{BannedDCs: []int{3}}}
		rs, errs := ruleSetFromForm(base, url.Values{})
		require.Empty(t, errs)
		assert.Equal(t, []int{3}, rs.JoinGate.BannedDCs)
	})
	t.Run("out of range rejected", func(t *testing.T) {
		_, errs := ruleSetFromForm(rules.RuleSet{}, url.Values{"join_gate.banned_dcs": {"2,99"}})
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "join_gate.banned_dcs")
	})
	t.Run("non-numeric rejected", func(t *testing.T) {
		_, errs := ruleSetFromForm(rules.RuleSet{}, url.Values{"join_gate.banned_dcs": {"2,x"}})
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "join_gate.banned_dcs")
	})
}
