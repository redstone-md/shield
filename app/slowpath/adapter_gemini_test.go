package slowpath

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

func TestGeminiAdapterName(t *testing.T) {
	a := NewGeminiAdapter(nil, "", GeminiAdapterConfig{})
	assert.Equal(t, "gemini", a.Name())
}

func TestGeminiAdapterDefaults(t *testing.T) {
	a := NewGeminiAdapter(nil, "", GeminiAdapterConfig{})
	assert.Equal(t, "gemma-4-31b-it", a.model)
	assert.Equal(t, int32(1024), a.config.MaxOutputTokens)
	assert.Equal(t, 8192, a.config.MaxSymbolsRequest)
}

func makeGeminiResponse(text string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: text}},
				},
			},
		},
	}
}

func TestGeminiAdapterCheckSpam(t *testing.T) {
	mock := &mockGeminiClient{
		fn: func(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			assert.Equal(t, "gemma-4-31b-it", model)
			return makeGeminiResponse(`{"spam":true,"reason":"crypto","confidence":92}`), nil
		},
	}
	a := NewGeminiAdapter(mock, "gemma-4-31b-it", GeminiAdapterConfig{})
	result, err := a.Check(context.Background(), ProviderRequest{Message: "buy USDT"})
	assert.NoError(t, err)
	assert.True(t, result.Spam)
	assert.Equal(t, 92, result.Confidence)
	assert.Equal(t, "gemini", result.Provider)
}

func TestGeminiAdapterCheckHam(t *testing.T) {
	mock := &mockGeminiClient{
		fn: func(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			return makeGeminiResponse(`{"spam":false,"reason":"clean","confidence":8}`), nil
		},
	}
	a := NewGeminiAdapter(mock, "gemma-4-31b-it", GeminiAdapterConfig{})
	result, err := a.Check(context.Background(), ProviderRequest{Message: "hello"})
	assert.NoError(t, err)
	assert.False(t, result.Spam)
}

func TestGeminiAdapterNoCandidates(t *testing.T) {
	mock := &mockGeminiClient{
		fn: func(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			return &genai.GenerateContentResponse{}, nil
		},
	}
	a := NewGeminiAdapter(mock, "gemma-4-31b-it", GeminiAdapterConfig{})
	_, err := a.Check(context.Background(), ProviderRequest{Message: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no candidates")
}

func TestGeminiAdapterAPIError(t *testing.T) {
	mock := &mockGeminiClient{
		fn: func(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			return nil, context.DeadlineExceeded
		},
	}
	a := NewGeminiAdapter(mock, "gemma-4-31b-it", GeminiAdapterConfig{})
	_, err := a.Check(context.Background(), ProviderRequest{Message: "test"})
	assert.Error(t, err)
}

func TestGeminiAdapterAnalyzeImage(t *testing.T) {
	mock := &mockGeminiClient{
		fn: func(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			return makeGeminiResponse(`{"spam":true,"reason":"qr code scam","confidence":88}`), nil
		},
	}
	a := NewGeminiAdapter(mock, "gemma-4-31b-it", GeminiAdapterConfig{})
	result, err := a.AnalyzeImage(context.Background(), []byte("fake-image"), "image/jpeg", "check for scams")
	assert.NoError(t, err)
	assert.True(t, result.Spam)
	assert.Equal(t, "gemini", result.Provider)
}

type mockGeminiClient struct {
	fn func(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

func (m *mockGeminiClient) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	if m.fn != nil {
		return m.fn(ctx, model, contents, config)
	}
	return nil, nil
}
