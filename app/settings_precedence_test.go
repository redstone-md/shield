package main

import (
	"testing"

	"github.com/redstone-md/shield/app/rules"
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

func TestApplyExplicitOverrides_DetectionFields(t *testing.T) {
	t.Setenv("MAX_EMOJI", "9")
	t.Setenv("LLM_MODE", "always")

	var opts options
	opts.MaxEmoji = 9
	opts.LLM.Mode = "always"

	rs := rules.RuleSet{
		Detection: rules.DetectionRules{MaxEmoji: 2},
		LLM:       rules.LLMCommonRules{Mode: "flagged"},
	}
	applyExplicitRuleSetOverrides(&rs, opts)

	assert.Equal(t, 9, rs.Detection.MaxEmoji, "MAX_EMOJI env must override the ruleset")
	assert.Equal(t, "always", rs.LLM.Mode, "LLM_MODE env must override the ruleset")
}

func TestApplyExplicitOverrides_DetectionFieldsUntouchedWithoutEnv(t *testing.T) {
	var opts options
	opts.MaxEmoji = 9

	rs := rules.RuleSet{Detection: rules.DetectionRules{MaxEmoji: 2}}
	applyExplicitRuleSetOverrides(&rs, opts)

	assert.Equal(t, 2, rs.Detection.MaxEmoji, "no env set: ruleset value must be kept")
}

func TestEnvPinnedKeys(t *testing.T) {
	t.Setenv("MAX_EMOJI", "5")
	t.Setenv("OPENAI_VETO", "true")

	pinned := envPinnedKeys()

	assert.True(t, pinned["detection.max_emoji"], "MAX_EMOJI must be reported as pinned")
	assert.True(t, pinned["openai.veto"], "OPENAI_VETO must be reported as pinned")
	assert.False(t, pinned["detection.min_msg_len"], "unset MIN_MSG_LEN must not be pinned")
}

func TestApplyExplicitOverrides_LLMPrompt(t *testing.T) {
	t.Setenv("OPENAI_PROMPT", "custom openai prompt")
	t.Setenv("GEMINI_PROMPT", "custom gemini prompt")

	var opts options
	opts.OpenAI.Prompt = "custom openai prompt"
	opts.Gemini.Prompt = "custom gemini prompt"

	rs := rules.RuleSet{
		OpenAI: rules.LLMRules{Prompt: "old openai"},
		Gemini: rules.LLMRules{Prompt: "old gemini"},
	}
	applyExplicitRuleSetOverrides(&rs, opts)

	assert.Equal(t, "custom openai prompt", rs.OpenAI.Prompt)
	assert.Equal(t, "custom gemini prompt", rs.Gemini.Prompt)
}

func TestApplyExplicitOverrides_LLMPromptUntouchedWithoutEnv(t *testing.T) {
	var opts options
	opts.OpenAI.Prompt = "custom openai prompt"

	rs := rules.RuleSet{OpenAI: rules.LLMRules{Prompt: "old openai"}}
	applyExplicitRuleSetOverrides(&rs, opts)

	assert.Equal(t, "old openai", rs.OpenAI.Prompt, "no env set: ruleset value kept")
}
