package slowpath

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscalationReasonValues(t *testing.T) {
	assert.Equal(t, EscalationReason("ambiguous_fast"), EscalationAmbiguousFast)
	assert.Equal(t, EscalationReason("image_content"), EscalationImageContent)
	assert.Equal(t, EscalationReason("user_report"), EscalationUserReport)
	assert.Equal(t, EscalationReason("high_risk_policy"), EscalationHighRiskPolicy)
	assert.Equal(t, EscalationReason("force_llm"), EscalationForceLLM)
}

func TestBudgetClassValues(t *testing.T) {
	assert.Equal(t, BudgetClass("standard"), BudgetClassStandard)
	assert.Equal(t, BudgetClass("high"), BudgetClassHigh)
	assert.Equal(t, BudgetClass("premium"), BudgetClassPremium)
}

func TestProviderResultDefaults(t *testing.T) {
	r := &ProviderResult{}
	assert.False(t, r.Spam)
	assert.Equal(t, 0, r.Confidence)
	assert.Empty(t, r.Provider)
	assert.Empty(t, r.Model)
}

func TestSlowPathRequestFields(t *testing.T) {
	req := SlowPathRequest{
		EventID:       "evt-1",
		CorrelationID: "corr-1",
		TenantID:      "tenant-1",
		Reason:        EscalationAmbiguousFast,
		PromptVersion: "v1",
		BudgetClass:   BudgetClassStandard,
	}
	assert.Equal(t, "evt-1", req.EventID)
	assert.Equal(t, EscalationAmbiguousFast, req.Reason)
	assert.Equal(t, BudgetClassStandard, req.BudgetClass)
	assert.Nil(t, req.ImageData)
	assert.Empty(t, req.ImageMIME)
}

func TestSlowPathInvocationFields(t *testing.T) {
	inv := SlowPathInvocation{
		EventID:       "evt-1",
		Provider:      "gemini",
		Model:         "gemma-4-31b-it",
		PromptVersion: "v2",
		InputTokens:   100,
		OutputTokens:  50,
		LatencyMs:     200,
		RawResponse:   `{"spam":true}`,
	}
	assert.Equal(t, "gemini", inv.Provider)
	assert.Equal(t, "v2", inv.PromptVersion)
	assert.Equal(t, `{"spam":true}`, inv.RawResponse)
}

func TestLLMProviderInterface(t *testing.T) {
	var _ LLMProvider = (*mockLLMProvider)(nil)
}

func TestVisionProviderInterface(t *testing.T) {
	var _ VisionProvider = (*mockVisionProvider)(nil)
}

type mockLLMProvider struct{}

func (m *mockLLMProvider) Name() string { return "mock" }
func (m *mockLLMProvider) Check(_ context.Context, _ ProviderRequest) (*ProviderResult, error) {
	return &ProviderResult{Spam: false, Provider: "mock"}, nil
}

type mockVisionProvider struct{}

func (m *mockVisionProvider) Name() string { return "mock-vision" }
func (m *mockVisionProvider) AnalyzeImage(_ context.Context, _ []byte, _ string, _ string) (*ProviderResult, error) {
	return &ProviderResult{Spam: false, Provider: "mock-vision"}, nil
}

func TestPromptEntryFields(t *testing.T) {
	pe := PromptEntry{
		ID:           "openai-v1",
		Version:      "v1",
		Provider:     "openai",
		SystemPrompt: "test prompt",
		Active:       true,
	}
	assert.Equal(t, "openai-v1", pe.ID)
	assert.True(t, pe.Active)
}
