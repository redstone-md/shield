package slowpath

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_FastToSlowEscalation(t *testing.T) {
	provider := &stubLLMProvider{
		name: "openai",
		result: &ProviderResult{
			Spam: true, Confidence: 92, Reason: "crypto scam", Provider: "openai", Model: "gpt-4o-mini",
			InputTokens: 100, OutputTokens: 50, Latency: 200 * time.Millisecond,
		},
	}

	eng := NewEngine(EngineConfig{DefaultProvider: "openai", CostPerToken: 0.00001})
	eng.RegisterProvider(provider, DefaultBreakerConfig())

	check := EscalationCheck{AmbiguousScore: true}
	shouldEscalate, reason := ShouldEscalate(check)
	assert.True(t, shouldEscalate)
	assert.Equal(t, EscalationAmbiguousFast, reason)

	req := SlowPathRequest{
		EventID:  "evt-1",
		TenantID: "tenant-1",
		Reason:   reason,
		Content:  Content{Text: "buy USDT cheap"},
		FastResult: DetectionResult{
			Spam:  false,
			Score: 0.45,
			Signals: []DetectionSignal{
				{Name: "stopword", Score: 0.45, Matched: false},
			},
		},
		BudgetClass: BudgetClassStandard,
	}

	result, err := eng.Check(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.Spam)
	assert.Equal(t, 92, result.Confidence)
	assert.Equal(t, "openai", result.Providers[0])
	assert.True(t, result.Final)

	fast := req.FastResult
	merged := MergeResults(fast, result)
	assert.True(t, merged.Spam)
	assert.Len(t, merged.Signals, 2)
	assert.Equal(t, "openai", merged.Signals[1].Name)
}

func TestIntegration_VisionEscalation(t *testing.T) {
	vision := &stubVisionProvider{
		name: "gemini",
		result: &ProviderResult{
			Spam: true, Confidence: 88, Reason: "QR code scam detected", Provider: "gemini",
		},
	}

	eng := NewEngine(EngineConfig{DefaultProvider: "gemini"})
	eng.RegisterVision(vision, DefaultBreakerConfig())

	req := SlowPathRequest{
		EventID:   "evt-img-1",
		TenantID:  "tenant-1",
		Reason:    EscalationImageContent,
		Content:   Content{Text: "check this image", HasMedia: true},
		ImageData: []byte("fake-jpeg-data"),
		ImageMIME: "image/jpeg",
	}

	result, err := eng.Check(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.Spam)
	assert.Equal(t, 88, result.Confidence)
	assert.Equal(t, "gemini", result.Providers[0])
}

func TestIntegration_BudgetEnforcement(t *testing.T) {
	provider := &stubLLMProvider{
		name: "openai",
		result: &ProviderResult{
			Spam: false, Confidence: 10, Provider: "openai",
			InputTokens: 50, OutputTokens: 20,
		},
	}

	budget := NewInMemoryBudgetTracker()
	budget.SetConfig("tenant-1", BudgetConfig{MaxRequestsPerHour: 2})

	eng := NewEngine(EngineConfig{DefaultProvider: "openai", CostPerToken: 0.00001})
	eng.RegisterProvider(provider, DefaultBreakerConfig())
	eng.SetBudgetTracker(budget)

	req := SlowPathRequest{
		EventID: "evt-1", TenantID: "tenant-1",
		Content: Content{Text: "test"}, BudgetClass: BudgetClassStandard,
	}

	result, err := eng.Check(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.Skipped)

	result, err = eng.Check(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.Skipped)

	result, err = eng.Check(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.Skipped)
}

func TestIntegration_CircuitBreakerTrips(t *testing.T) {
	var callCount int
	provider := &callableProvider{
		name: "failing",
		fn: func(ctx context.Context, req ProviderRequest) (*ProviderResult, error) {
			callCount++
			return nil, context.DeadlineExceeded
		},
	}

	cfg := BreakerConfig{FailuresToTrip: 3, Interval: time.Second, Timeout: time.Second, MaxRequests: 1}
	eng := NewEngine(EngineConfig{DefaultProvider: "failing"})
	eng.RegisterProvider(provider, cfg)

	req := SlowPathRequest{EventID: "evt-1", Content: Content{Text: "test"}}

	for range 3 {
		_, _ = eng.Check(context.Background(), req)
	}
	assert.Equal(t, 3, callCount)

	_, err := eng.Check(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker")
}

type callableProvider struct {
	name string
	fn   func(context.Context, ProviderRequest) (*ProviderResult, error)
}

func (c *callableProvider) Name() string { return c.name }
func (c *callableProvider) Check(ctx context.Context, req ProviderRequest) (*ProviderResult, error) {
	return c.fn(ctx, req)
}

func TestIntegration_SystemPromptWithEngine(t *testing.T) {
	provider := &stubLLMProvider{
		name:   "openai",
		result: &ProviderResult{Spam: false, Confidence: 10, Provider: "openai"},
	}

	eng := NewEngine(EngineConfig{DefaultProvider: "openai"})
	eng.RegisterProvider(provider, DefaultBreakerConfig())
	eng.SetSystemPrompt("openai", "custom system prompt")

	req := SlowPathRequest{
		EventID: "evt-1", Content: Content{Text: "test"},
	}

	result, err := eng.Check(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.Spam)
	assert.Equal(t, "custom system prompt", provider.req.SystemPrompt)
}

func TestIntegration_FullFlowMergeWithPolicy(t *testing.T) {
	fast := DetectionResult{
		Spam:  false,
		Score: 0.42,
		Signals: []DetectionSignal{
			{Name: "stopword", Score: 0.42, Matched: false, Reason: "ambiguous"},
		},
	}

	slow := &SlowPathResult{
		Spam:       true,
		Confidence: 91,
		Reason:     "LLM: crypto scam detected",
		Providers:  []string{"openai"},
		Final:      true,
		Signals: []ProviderResult{
			{Spam: true, Confidence: 91, Provider: "openai", Reason: "crypto scam"},
		},
	}

	merged := MergeResults(fast, slow)
	assert.True(t, merged.Spam)
	assert.GreaterOrEqual(t, merged.Score, 0.42)
	assert.Len(t, merged.Signals, 2)

	assert.Equal(t, "stopword", merged.Signals[0].Name)
	assert.Equal(t, "openai", merged.Signals[1].Name)
	assert.Equal(t, "crypto scam", merged.Signals[1].Reason)
}

func TestIntegration_SlowPathSkippedOnBudget(t *testing.T) {
	fast := DetectionResult{
		Spam:  false,
		Score: 0.48,
		Signals: []DetectionSignal{
			{Name: "stopword", Score: 0.48, Matched: false},
		},
	}

	slow := &SlowPathResult{Skipped: true}

	merged := MergeResults(fast, slow)
	assert.False(t, merged.Spam)
	assert.InDelta(t, 0.48, merged.Score, 1e-9)
	assert.Len(t, merged.Signals, 1)
}

func TestIntegration_InvocationFromResult(t *testing.T) {
	eng := NewEngine(EngineConfig{CostPerToken: 0.00003})

	req := SlowPathRequest{
		EventID:       "evt-1",
		CorrelationID: "corr-1",
		TenantID:      "tenant-1",
		Reason:        EscalationAmbiguousFast,
	}

	result := &ProviderResult{
		Spam: true, Confidence: 90, Reason: "scam", Provider: "openai", Model: "gpt-4o",
		InputTokens: 100, OutputTokens: 50, Latency: 300 * time.Millisecond,
		RawResponse: `{"spam":true}`, PromptVersion: "v2",
	}

	inv := eng.InvocationFromResult(req, result)
	assert.Equal(t, "evt-1", inv.EventID)
	assert.Equal(t, "corr-1", inv.CorrelationID)
	assert.Equal(t, "tenant-1", inv.TenantID)
	assert.Equal(t, "openai", inv.Provider)
	assert.Equal(t, "gpt-4o", inv.Model)
	assert.Equal(t, 100, inv.InputTokens)
	assert.Equal(t, 50, inv.OutputTokens)
	assert.Equal(t, int64(300), inv.LatencyMs)
	assert.True(t, inv.Spam)
	assert.Equal(t, 90, inv.Confidence)
	assert.Equal(t, EscalationAmbiguousFast, inv.Reason)
	assert.InDelta(t, 0.0045, inv.CostEstimate, 0.0001)
}

func TestIntegration_AllEscalationReasons(t *testing.T) {
	checks := []struct {
		check  EscalationCheck
		reason EscalationReason
	}{
		{EscalationCheck{ForceLLM: true}, EscalationForceLLM},
		{EscalationCheck{UserReport: true}, EscalationUserReport},
		{EscalationCheck{HighRiskPolicy: true}, EscalationHighRiskPolicy},
		{EscalationCheck{HasImages: true}, EscalationImageContent},
		{EscalationCheck{AmbiguousScore: true}, EscalationAmbiguousFast},
	}

	for _, tc := range checks {
		ok, reason := ShouldEscalate(tc.check)
		assert.True(t, ok, "expected escalation for %+v", tc.check)
		assert.Equal(t, tc.reason, reason)
	}
}
