package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umputun/tg-spam/app/rules"
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
