package rules

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleSet_NewFieldsJSONRoundTrip(t *testing.T) {
	rs := RuleSet{
		WorkspaceID:   "tg-spam",
		Version:       3,
		SchemaVersion: 1,
		Detection: DetectionRules{
			MaxEmoji:            5,
			MinMsgLen:           50,
			SimilarityThreshold: 0.5,
			MinSpamProbability:  50,
			MultiLangWords:      2,
			CasEnabled:          true,
			HistorySize:         1000,
			FirstMessagesCount:  1,
			ParanoidMode:        false,
		},
		LLM: LLMCommonRules{Mode: "flagged", Consensus: "any"},
		OpenAI: LLMRules{
			Model:       "gpt-4o-mini",
			Prompt:      "be strict",
			VisionModel: "gpt-4o",
		},
	}

	data, err := json.Marshal(rs)
	require.NoError(t, err)

	var got RuleSet
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, rs, got)
}

func TestRuleSet_LegacyPayloadDecodesNewFieldsAsZero(t *testing.T) {
	// payload written before the new fields existed
	legacy := `{"workspace_id":"tg-spam","version":1,"openai":{"model":"gpt-4o-mini"}}`

	var got RuleSet
	require.NoError(t, json.Unmarshal([]byte(legacy), &got))
	assert.Equal(t, 0, got.SchemaVersion)
	assert.Equal(t, DetectionRules{}, got.Detection)
	assert.Equal(t, LLMCommonRules{}, got.LLM)
	assert.Empty(t, got.OpenAI.Prompt)
}

func TestCurrentSchemaVersion(t *testing.T) {
	assert.Equal(t, 4, CurrentSchemaVersion)
}

func TestRuleSet_JoinGateJSONRoundTrip(t *testing.T) {
	rs := RuleSet{JoinGate: JoinGateRules{BannedDCs: []int{2, 4}}}

	data, err := json.Marshal(rs)
	require.NoError(t, err)

	var got RuleSet
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, []int{2, 4}, got.JoinGate.BannedDCs)
}

func TestRuleSet_JoinGateLegacyDecodesAsEmpty(t *testing.T) {
	// payload written before JoinGate existed.
	legacy := `{"workspace_id":"tg-spam","version":1,"join_gate":null}`

	var got RuleSet
	require.NoError(t, json.Unmarshal([]byte(legacy), &got))
	assert.Empty(t, got.JoinGate.BannedDCs)
}

func TestRuleSet_LLMCommonJSONRoundTrip(t *testing.T) {
	rs := RuleSet{LLM: LLMCommonRules{Mode: "flagged", HistoryContextSize: 5, VisionPrompt: "scan this image"}}

	data, err := json.Marshal(rs)
	require.NoError(t, err)

	var got RuleSet
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, 5, got.LLM.HistoryContextSize)
	assert.Equal(t, "scan this image", got.LLM.VisionPrompt)
}
