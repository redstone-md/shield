package main

import (
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/redstone-md/shield/app/rules"
	"github.com/redstone-md/shield/app/slowpath"
	"github.com/redstone-md/shield/lib/tgspam"
	"github.com/stretchr/testify/assert"
)

func TestBootstrapRuleSet_SeedsNewFields(t *testing.T) {
	var opts options
	opts.InstanceID = "tg-spam"
	opts.MaxEmoji = 7
	opts.MinMsgLen = 40
	opts.SimilarityThreshold = 0.6
	opts.MinSpamProbability = 55
	opts.MultiLangWords = 3
	opts.CAS.API = "https://api.cas.chat"
	opts.HistoryMinSize = 1234
	opts.FirstMessagesCount = 2
	opts.ParanoidMode = true
	opts.LLM.Mode = "flagged"
	opts.LLM.Consensus = "all"
	opts.OpenAI.Prompt = "strict openai"
	opts.OpenAI.VisionModel = "gpt-4o"
	opts.Gemini.Prompt = "strict gemini"
	opts.Gemini.VisionModel = "gemini-vision"

	rs := bootstrapRuleSet(opts)

	assert.Equal(t, rules.CurrentSchemaVersion, rs.SchemaVersion)
	assert.Equal(t, 7, rs.Detection.MaxEmoji)
	assert.Equal(t, 40, rs.Detection.MinMsgLen)
	assert.InEpsilon(t, 0.6, rs.Detection.SimilarityThreshold, 0.0001)
	assert.InEpsilon(t, 55.0, rs.Detection.MinSpamProbability, 0.0001)
	assert.Equal(t, 3, rs.Detection.MultiLangWords)
	assert.True(t, rs.Detection.CasEnabled)
	assert.Equal(t, 1234, rs.Detection.HistorySize)
	assert.Equal(t, 2, rs.Detection.FirstMessagesCount)
	assert.True(t, rs.Detection.ParanoidMode)
	assert.Equal(t, "flagged", rs.LLM.Mode)
	assert.Equal(t, "all", rs.LLM.Consensus)
	assert.Equal(t, "strict openai", rs.OpenAI.Prompt)
	assert.Equal(t, "gpt-4o", rs.OpenAI.VisionModel)
	assert.Equal(t, "strict gemini", rs.Gemini.Prompt)
	assert.Equal(t, "gemini-vision", rs.Gemini.VisionModel)
}

func TestBootstrapRuleSet_CasDisabledWhenNoAPI(t *testing.T) {
	var opts options
	opts.InstanceID = "tg-spam"
	rs := bootstrapRuleSet(opts)
	assert.False(t, rs.Detection.CasEnabled)
}

func TestBackfillRuleSetSchema_OldRuleSet(t *testing.T) {
	var opts options
	opts.InstanceID = "tg-spam"
	opts.MaxEmoji = 4
	opts.MinMsgLen = 50
	opts.LLM.Mode = "flagged"

	// a ruleset persisted before the new fields existed
	old := rules.RuleSet{WorkspaceID: "tg-spam", Version: 1, SchemaVersion: 0}

	got, changed := backfillRuleSetSchema(old, opts)

	assert.True(t, changed, "an old ruleset must be reported as changed")
	assert.Equal(t, rules.CurrentSchemaVersion, got.SchemaVersion)
	assert.Equal(t, 4, got.Detection.MaxEmoji)
	assert.Equal(t, 50, got.Detection.MinMsgLen)
	assert.Equal(t, "flagged", got.LLM.Mode)
	assert.Equal(t, 1, got.Version, "version and identity fields must be preserved")
}

func TestBackfillRuleSetSchema_CurrentRuleSetUnchanged(t *testing.T) {
	var opts options
	opts.InstanceID = "tg-spam"

	current := rules.RuleSet{
		WorkspaceID:   "tg-spam",
		Version:       5,
		SchemaVersion: rules.CurrentSchemaVersion,
		Detection:     rules.DetectionRules{MaxEmoji: 2},
	}
	got, changed := backfillRuleSetSchema(current, opts)

	assert.False(t, changed, "a current-schema ruleset must not be changed")
	assert.Equal(t, current, got)
}

func TestWireLiveReload_AppliesLLMAndSlowPathPrompts(t *testing.T) {
	var opts options
	opts.InstanceID = "tg-spam"

	a := &runtimeAssembly{
		Detector:       tgspam.NewDetector(tgspam.Config{}),
		SlowPathEngine: slowpath.NewEngine(slowpath.EngineConfig{}),
	}
	rs := rules.RuleSet{LLM: rules.LLMCommonRules{VisionPrompt: "live vision prompt"}}

	applyLiveReload(a, opts, rs)

	assert.Equal(t, "live vision prompt", slowpath.ExportVisionPrompt(a.SlowPathEngine))
}

func TestMakeTelegramListener_UsesActiveRuleSetSlowPathFlag(t *testing.T) {
	tbAPI := &tbapi.BotAPI{Self: tbapi.User{UserName: "bot"}}
	a := &runtimeAssembly{
		SlowPathEngine: slowpath.NewEngine(slowpath.EngineConfig{}),
		ActiveRuleSet:  rules.RuleSet{SlowPathEnabled: false},
	}

	listener := a.makeTelegramListener(options{}, tbAPI)
	assert.False(t, listener.SlowPathEnabled)

	a.ActiveRuleSet.SlowPathEnabled = true
	listener = a.makeTelegramListener(options{}, tbAPI)
	assert.True(t, listener.SlowPathEnabled)
}
