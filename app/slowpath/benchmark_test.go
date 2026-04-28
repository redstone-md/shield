package slowpath

import (
	"context"
	"testing"

	"github.com/sashabaranov/go-openai"
	"google.golang.org/genai"
)

func BenchmarkEngineCheck(b *testing.B) {
	provider := &stubLLMProvider{
		name:   "bench",
		result: &ProviderResult{Spam: false, Confidence: 10, Provider: "bench"},
	}
	eng := NewEngine(EngineConfig{DefaultProvider: "bench"})
	eng.RegisterProvider(provider, BreakerConfig{FailuresToTrip: 100})

	req := SlowPathRequest{
		EventID: "bench-evt",
		Content: Content{Text: "benchmark message"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eng.Check(context.Background(), req)
	}
}

func BenchmarkEngineCheckWithBudget(b *testing.B) {
	provider := &stubLLMProvider{
		name:   "bench",
		result: &ProviderResult{Spam: false, Confidence: 10, Provider: "bench"},
	}
	bt := NewInMemoryBudgetTracker()
	bt.SetConfig("tenant-1", BudgetConfig{MaxRequestsPerHour: 100000})

	eng := NewEngine(EngineConfig{DefaultProvider: "bench", CostPerToken: 0.00001})
	eng.RegisterProvider(provider, BreakerConfig{FailuresToTrip: 100})
	eng.SetBudgetTracker(bt)

	req := SlowPathRequest{
		EventID:  "bench-evt",
		TenantID: "tenant-1",
		Content:  Content{Text: "benchmark message"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eng.Check(context.Background(), req)
	}
}

func BenchmarkEngineVision(b *testing.B) {
	vision := &stubVisionProvider{
		name:   "bench-vision",
		result: &ProviderResult{Spam: false, Confidence: 10, Provider: "bench-vision"},
	}
	eng := NewEngine(EngineConfig{DefaultProvider: "bench-vision"})
	eng.RegisterVision(vision, BreakerConfig{FailuresToTrip: 100})

	imgData := make([]byte, 1024)
	req := SlowPathRequest{
		EventID:   "bench-img",
		ImageData: imgData,
		ImageMIME: "image/jpeg",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eng.Check(context.Background(), req)
	}
}

func BenchmarkMergeResults(b *testing.B) {
	fast := DetectionResult{
		Spam:  true,
		Score: 0.8,
		Signals: []DetectionSignal{
			{Name: "stopword", Score: 0.9, Matched: true},
			{Name: "meta", Score: 0.7, Matched: true},
		},
	}
	slow := &SlowPathResult{
		Spam:       true,
		Confidence: 90,
		Final:      true,
		Signals: []ProviderResult{
			{Spam: true, Confidence: 90, Provider: "openai"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MergeResults(fast, slow)
	}
}

func BenchmarkShouldEscalate(b *testing.B) {
	check := EscalationCheck{AmbiguousScore: true}
	for i := 0; i < b.N; i++ {
		ShouldEscalate(check)
	}
}

func BenchmarkBudgetTracker(b *testing.B) {
	bt := NewInMemoryBudgetTracker()
	bt.SetConfig("bench-tenant", BudgetConfig{MaxRequestsPerHour: 1000000})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bt.Allow("bench-tenant", BudgetClassStandard, 100)
		bt.Record("bench-tenant", BudgetClassStandard, 100, 0.001)
	}
}

func BenchmarkParseLLMOutput(b *testing.B) {
	resp := `{"spam":true,"reason":"crypto exchange detected","confidence":95}`
	for i := 0; i < b.N; i++ {
		_, _ = parseLLMOutput(resp)
	}
}

func BenchmarkOpenAIAdapter(b *testing.B) {
	mock := &benchOpenAIMock{}
	a := NewOpenAIAdapter(mock, "gpt-4o-mini", 1024, 8192)
	req := ProviderRequest{Message: "benchmark message"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = a.Check(context.Background(), req)
	}
}

func BenchmarkGeminiAdapter(b *testing.B) {
	mock := &benchGeminiMock{}
	a := NewGeminiAdapter(mock, "gemma-4-31b-it", GeminiAdapterConfig{})
	req := ProviderRequest{Message: "benchmark message"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = a.Check(context.Background(), req)
	}
}

type benchOpenAIMock struct{}

func (m *benchOpenAIMock) CreateChatCompletion(_ context.Context, _ openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{Message: openai.ChatCompletionMessage{Content: `{"spam":false,"reason":"clean","confidence":5}`}},
		},
		Usage: openai.Usage{PromptTokens: 50, CompletionTokens: 20},
	}, nil
}

type benchGeminiMock struct{}

func (m *benchGeminiMock) GenerateContent(_ context.Context, _ string, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{Content: &genai.Content{Parts: []*genai.Part{{Text: `{"spam":false,"reason":"clean","confidence":5}`}}}},
		},
	}, nil
}
