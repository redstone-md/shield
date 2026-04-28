package slowpath

import (
	"context"
	"sync"

	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/genai"
)

type MockOpenAIClient struct {
	mu         sync.Mutex
	Resp       openai.ChatCompletionResponse
	Err        error
	CallsCount int
}

func (m *MockOpenAIClient) CreateChatCompletion(_ context.Context, _ openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	m.mu.Lock()
	m.CallsCount++
	m.mu.Unlock()
	return m.Resp, m.Err
}

type MockGeminiClient struct {
	mu         sync.Mutex
	Resp       *genai.GenerateContentResponse
	Err        error
	CallsCount int
}

func (m *MockGeminiClient) GenerateContent(_ context.Context, _ string, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	m.mu.Lock()
	m.CallsCount++
	m.mu.Unlock()
	return m.Resp, m.Err
}
