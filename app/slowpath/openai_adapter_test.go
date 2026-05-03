package slowpath

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func TestOpenAIAdapterName(t *testing.T) {
	a := NewOpenAIAdapter(nil, "gpt-4o-mini", 0, 0)
	assert.Equal(t, "openai", a.Name())
}

func TestOpenAIAdapterDefaults(t *testing.T) {
	a := NewOpenAIAdapter(nil, "", 0, 0)
	assert.Equal(t, "gpt-4o-mini", a.model)
	assert.Equal(t, 1024, a.maxTokens)
	assert.Equal(t, 8192, a.maxSymbols)
}

func TestOpenAIAdapterCheckSpam(t *testing.T) {
	mock := &mockOpenAIClient{
		fn: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			assert.Equal(t, "gpt-4o-mini", req.Model)
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: `{"spam":true,"reason":"crypto spam","confidence":95}`}},
				},
				Usage: openai.Usage{PromptTokens: 100, CompletionTokens: 50},
			}, nil
		},
	}
	a := NewOpenAIAdapter(mock, "gpt-4o-mini", 1024, 8192)
	result, err := a.Check(context.Background(), ProviderRequest{Message: "buy USDT now"})
	assert.NoError(t, err)
	assert.True(t, result.Spam)
	assert.Equal(t, 95, result.Confidence)
	assert.Contains(t, result.Reason, "crypto spam")
	assert.Equal(t, 100, result.InputTokens)
	assert.Equal(t, 50, result.OutputTokens)
	assert.Equal(t, "openai", result.Provider)
}

func TestOpenAIAdapterCheckHam(t *testing.T) {
	mock := &mockOpenAIClient{
		fn: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: `{"spam":false,"reason":"clean message","confidence":10}`}},
				},
				Usage: openai.Usage{PromptTokens: 50, CompletionTokens: 20},
			}, nil
		},
	}
	a := NewOpenAIAdapter(mock, "gpt-4o-mini", 1024, 8192)
	result, err := a.Check(context.Background(), ProviderRequest{Message: "hello world"})
	assert.NoError(t, err)
	assert.False(t, result.Spam)
	assert.Equal(t, 10, result.Confidence)
}

func TestOpenAIAdapterCustomPrompts(t *testing.T) {
	mock := &mockOpenAIClient{
		fn: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			assert.Contains(t, req.Messages[0].Content, "Also check for:")
			assert.Contains(t, req.Messages[0].Content, "1. custom rule")
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: `{"spam":false,"reason":"ok","confidence":5}`}},
				},
				Usage: openai.Usage{},
			}, nil
		},
	}
	a := NewOpenAIAdapter(mock, "gpt-4o", 1024, 8192)
	result, err := a.Check(context.Background(), ProviderRequest{
		Message:       "hello",
		CustomPrompts: []string{"custom rule"},
	})
	assert.NoError(t, err)
	assert.False(t, result.Spam)
}

func TestOpenAIAdapterWithHistory(t *testing.T) {
	mock := &mockOpenAIClient{
		fn: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			userMsg := req.Messages[1].Content
			assert.Contains(t, userMsg, "Recent messages:")
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: `{"spam":false,"reason":"ok","confidence":5}`}},
				},
				Usage: openai.Usage{},
			}, nil
		},
	}
	a := NewOpenAIAdapter(mock, "gpt-4o", 1024, 8192)
	_, err := a.Check(context.Background(), ProviderRequest{
		Message: "test",
		History: []HistoryMessage{{UserName: "user1", Text: "msg1"}},
	})
	assert.NoError(t, err)
}

func TestOpenAIAdapterNoChoices(t *testing.T) {
	mock := &mockOpenAIClient{
		fn: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{Choices: []openai.ChatCompletionChoice{}}, nil
		},
	}
	a := NewOpenAIAdapter(mock, "gpt-4o", 1024, 8192)
	_, err := a.Check(context.Background(), ProviderRequest{Message: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestOpenAIAdapterBrokenJSON(t *testing.T) {
	mock := &mockOpenAIClient{
		fn: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: `{"spam":true,"reason":"spam","confidence":90,}`}},
				},
				Usage: openai.Usage{},
			}, nil
		},
	}
	a := NewOpenAIAdapter(mock, "gpt-4o", 1024, 8192)
	result, err := a.Check(context.Background(), ProviderRequest{Message: "test"})
	assert.NoError(t, err)
	assert.True(t, result.Spam)
}

func TestOpenAIAdapterAPIError(t *testing.T) {
	mock := &mockOpenAIClient{
		fn: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{}, context.DeadlineExceeded
		},
	}
	a := NewOpenAIAdapter(mock, "gpt-4o", 1024, 8192)
	_, err := a.Check(context.Background(), ProviderRequest{Message: "test"})
	assert.Error(t, err)
}

func TestOpenAIAdapterAnalyzeImageSendsMultimodalContent(t *testing.T) {
	mock := &mockOpenAIClient{
		fn: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			payload, err := json.Marshal(req.Messages[1])
			assert.NoError(t, err)
			assert.Contains(t, string(payload), `"content":[`)
			assert.NotContains(t, string(payload), `"content":null`)
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: `{"spam":false,"reason":"ok","confidence":5}`}},
				},
				Usage: openai.Usage{},
			}, nil
		},
	}
	a := NewOpenAIAdapter(mock, "gpt-4o", 1024, 8192)
	result, err := a.AnalyzeImage(context.Background(), []byte("fake-image"), "image/jpeg", "check image")
	assert.NoError(t, err)
	assert.False(t, result.Spam)
}

func TestBuildCustomPrompt(t *testing.T) {
	result := buildCustomPrompt("base", []string{"rule1", "rule2"})
	assert.Contains(t, result, "base")
	assert.Contains(t, result, "1. rule1")
	assert.Contains(t, result, "2. rule2")
}

func TestAppendHistory(t *testing.T) {
	result := appendHistory("msg", []HistoryMessage{{UserName: "u", Text: "t"}})
	assert.Contains(t, result, "User message:")
	assert.Contains(t, result, "Recent messages:")
}

func TestAppendHistoryEmpty(t *testing.T) {
	result := appendHistory("msg", nil)
	assert.Equal(t, "msg", result)
}

type mockOpenAIClient struct {
	fn func(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

func (m *mockOpenAIClient) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	if m.fn != nil {
		return m.fn(ctx, req)
	}
	return openai.ChatCompletionResponse{}, nil
}
