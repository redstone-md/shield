package slowpath

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEngineCheckText(t *testing.T) {
	provider := &stubLLMProvider{
		name:   "test",
		result: &ProviderResult{Spam: true, Confidence: 90, Reason: "spam", Provider: "test", Model: "m1"},
	}
	eng := NewEngine(EngineConfig{})
	eng.RegisterProvider(provider, DefaultBreakerConfig())

	result, err := eng.Check(context.Background(), SlowPathRequest{
		EventID: "evt-1",
		Content: Content{Text: "spam"},
	})
	assert.NoError(t, err)
	assert.True(t, result.Spam)
	assert.Equal(t, "evt-1", result.EventID)
	assert.True(t, result.Final)
	assert.Len(t, result.Signals, 1)
	assert.Equal(t, "test", result.Signals[0].Provider)
}

func TestEngineCheckNoProvider(t *testing.T) {
	eng := NewEngine(EngineConfig{})
	_, err := eng.Check(context.Background(), SlowPathRequest{
		EventID: "evt-1",
		Content: Content{Text: "test"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no LLM provider")
}

func TestEngineCheckProviderError(t *testing.T) {
	provider := &stubLLMProvider{name: "fail", err: errors.New("api down")}
	eng := NewEngine(EngineConfig{})
	eng.RegisterProvider(provider, BreakerConfig{Interval: 1})
	_, err := eng.Check(context.Background(), SlowPathRequest{
		EventID: "evt-1",
		Content: Content{Text: "test"},
	})
	assert.Error(t, err)
}

func TestEngineCheckBudgetDenied(t *testing.T) {
	provider := &stubLLMProvider{
		name:   "test",
		result: &ProviderResult{Spam: true, Confidence: 90, Provider: "test"},
	}
	budget := NewInMemoryBudgetTracker()
	budget.SetConfig("tenant-1", BudgetConfig{MaxRequestsPerHour: 1})
	budget.Record("tenant-1", BudgetClassStandard, 0, 0)

	eng := NewEngine(EngineConfig{})
	eng.RegisterProvider(provider, DefaultBreakerConfig())
	eng.SetBudgetTracker(budget)

	result, err := eng.Check(context.Background(), SlowPathRequest{
		EventID:     "evt-1",
		TenantID:    "tenant-1",
		BudgetClass: BudgetClassStandard,
		Content:     Content{Text: "test"},
	})
	assert.NoError(t, err)
	assert.True(t, result.Skipped)
}

func TestEngineCheckBudgetNoConfig(t *testing.T) {
	provider := &stubLLMProvider{
		name:   "test",
		result: &ProviderResult{Spam: true, Confidence: 90, Provider: "test"},
	}
	budget := NewInMemoryBudgetTracker()

	eng := NewEngine(EngineConfig{})
	eng.RegisterProvider(provider, DefaultBreakerConfig())
	eng.SetBudgetTracker(budget)

	result, err := eng.Check(context.Background(), SlowPathRequest{
		EventID:  "evt-1",
		TenantID: "tenant-unknown",
		Content:  Content{Text: "test"},
	})
	assert.NoError(t, err)
	assert.True(t, result.Spam)
}

func TestEngineVision(t *testing.T) {
	vision := &stubVisionProvider{
		name:   "test-vision",
		result: &ProviderResult{Spam: true, Confidence: 85, Reason: "qr scam", Provider: "test-vision"},
	}
	eng := NewEngine(EngineConfig{})
	eng.RegisterVision(vision, DefaultBreakerConfig())

	result, err := eng.Check(context.Background(), SlowPathRequest{
		EventID:   "evt-img",
		ImageData: []byte("fake"),
		ImageMIME: "image/jpeg",
	})
	assert.NoError(t, err)
	assert.True(t, result.Spam)
	assert.Equal(t, "evt-img", result.EventID)
}

func TestEngineVisionFallbackToTextProvider(t *testing.T) {
	provider := &stubLLMProvider{
		name:   "test",
		result: &ProviderResult{Spam: true, Confidence: 80, Provider: "test"},
	}
	eng := NewEngine(EngineConfig{DefaultProvider: "test"})
	eng.RegisterProvider(provider, DefaultBreakerConfig())

	result, err := eng.Check(context.Background(), SlowPathRequest{
		EventID:   "evt-img",
		ImageData: []byte("fake"),
		ImageMIME: "image/jpeg",
		Content:   Content{Text: "check this"},
	})
	assert.NoError(t, err)
	assert.True(t, result.Spam)
}

func TestEngineWithPromptRegistry(t *testing.T) {
	provider := &stubLLMProvider{
		name:   "test",
		result: &ProviderResult{Spam: false, Confidence: 10, Provider: "test"},
	}
	reg := NewInMemoryPromptRegistry()
	reg.Set(PromptEntry{
		Version:      "v2",
		Provider:     "test",
		SystemPrompt: "custom prompt",
		Active:       true,
	})

	eng := NewEngine(EngineConfig{})
	eng.RegisterProvider(provider, DefaultBreakerConfig())
	eng.SetPromptRegistry(reg)

	result, err := eng.Check(context.Background(), SlowPathRequest{
		EventID: "evt-1",
		Content: Content{Text: "test"},
	})
	assert.NoError(t, err)
	assert.False(t, result.Spam)
}

func TestEngineInvocationFromResult(t *testing.T) {
	eng := NewEngine(EngineConfig{CostPerToken: 0.001})
	req := SlowPathRequest{
		EventID:       "evt-1",
		CorrelationID: "corr-1",
		TenantID:      "t1",
		Reason:        EscalationUserReport,
	}
	result := &ProviderResult{
		Provider:      "openai",
		Model:         "gpt-4o",
		InputTokens:   100,
		OutputTokens:  50,
		Latency:       200,
		RawResponse:   `{"spam":true}`,
		PromptVersion: "v1",
		Spam:          true,
		Confidence:    95,
	}
	inv := eng.InvocationFromResult(req, result)
	assert.Equal(t, "evt-1", inv.EventID)
	assert.Equal(t, "openai", inv.Provider)
	assert.Equal(t, 150, inv.InputTokens+inv.OutputTokens)
	assert.Equal(t, 0.15, inv.CostEstimate)
	assert.True(t, inv.Spam)
}

func TestEngineDefaultProvider(t *testing.T) {
	p1 := &stubLLMProvider{
		name:   "openai",
		result: &ProviderResult{Spam: true, Provider: "openai"},
	}
	p2 := &stubLLMProvider{
		name:   "gemini",
		result: &ProviderResult{Spam: false, Provider: "gemini"},
	}
	eng := NewEngine(EngineConfig{DefaultProvider: "gemini"})
	eng.RegisterProvider(p1, DefaultBreakerConfig())
	eng.RegisterProvider(p2, DefaultBreakerConfig())

	result, err := eng.Check(context.Background(), SlowPathRequest{
		EventID: "evt-1",
		Content: Content{Text: "test"},
	})
	assert.NoError(t, err)
	assert.False(t, result.Spam)
	assert.Equal(t, []string{"gemini"}, result.Providers)
}

type stubLLMProvider struct {
	name   string
	result *ProviderResult
	err    error
}

func (s *stubLLMProvider) Name() string { return s.name }
func (s *stubLLMProvider) Check(_ context.Context, _ ProviderRequest) (*ProviderResult, error) {
	return s.result, s.err
}

type stubVisionProvider struct {
	name   string
	result *ProviderResult
	err    error
}

func (s *stubVisionProvider) Name() string { return s.name }
func (s *stubVisionProvider) AnalyzeImage(_ context.Context, _ []byte, _ string, _ string) (*ProviderResult, error) {
	return s.result, s.err
}
