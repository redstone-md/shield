package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umputun/tg-spam/app/rules"
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
