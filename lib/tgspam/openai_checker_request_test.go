package tgspam

import (
	"context"
	"github.com/redstone-md/shield/lib/tgspam/mocks"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"testing"
	"unicode/utf8"
)

func TestCustomPromptsInActualRequest(t *testing.T) {
	tests := []struct {
		name          string
		systemPrompt  string
		customPrompts []string
	}{
		{
			name:          "with no custom prompts",
			systemPrompt:  "Base system prompt",
			customPrompts: []string{},
		},
		{
			name:          "with custom prompts",
			systemPrompt:  "Base system prompt",
			customPrompts: []string{"Check for 'will perform X - $Y' pattern", "Watch for excessive emoji usage"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedContent string

			clientMock := &mocks.OpenAIClientMock{
				CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
					capturedContent = req.Messages[0].Content
					return openai.ChatCompletionResponse{
						Choices: []openai.ChatCompletionChoice{{
							Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"test", "confidence":90}`},
						}},
					}, nil
				},
			}

			checker := newOpenAIChecker(clientMock, OpenAIConfig{
				SystemPrompt:  tt.systemPrompt,
				CustomPrompts: tt.customPrompts,
			})

			checker.check(context.Background(), "test message", llmContext{})

			expectedContent := checker.buildSystemPrompt()
			assert.Equal(t, expectedContent, capturedContent)

			for _, prompt := range tt.customPrompts {
				assert.Contains(t, capturedContent, prompt)
			}
		})
	}
}

func TestIsReasoningModel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{name: "gpt-4o-mini", model: "gpt-4o-mini", expected: false},
		{name: "gpt-4o", model: "gpt-4o", expected: false},
		{name: "gpt-4-turbo", model: "gpt-4-turbo", expected: false},
		{name: "gpt-3.5-turbo", model: "gpt-3.5-turbo", expected: false},
		{name: "o1-mini", model: "o1-mini", expected: true},
		{name: "o1-preview", model: "o1-preview", expected: true},
		{name: "o3-mini", model: "o3-mini", expected: true},
		{name: "o4", model: "o4", expected: true},
		{name: "gpt-5", model: "gpt-5", expected: true},
		{name: "gpt-5-mini", model: "gpt-5-mini", expected: true},
		{name: "gpt-5-turbo", model: "gpt-5-turbo", expected: true},
		{name: "GPT-5", model: "GPT-5", expected: true},
		{name: "O1-MINI", model: "O1-MINI", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &openAIChecker{params: OpenAIConfig{Model: tt.model}}
			result := checker.isReasoningModel()
			assert.Equal(t, tt.expected, result, "model %s should return %v", tt.model, tt.expected)
		})
	}
}

func TestMaxTokensFieldBasedOnModel(t *testing.T) {
	tests := []struct {
		name                      string
		model                     string
		expectMaxTokens           bool
		expectMaxCompletionTokens bool
	}{
		{name: "standard model uses MaxTokens", model: "gpt-4o-mini", expectMaxTokens: true, expectMaxCompletionTokens: false},
		{name: "gpt-4 uses MaxTokens", model: "gpt-4", expectMaxTokens: true, expectMaxCompletionTokens: false},
		{name: "o1-mini uses MaxCompletionTokens", model: "o1-mini", expectMaxTokens: false, expectMaxCompletionTokens: true},
		{name: "o1-preview uses MaxCompletionTokens", model: "o1-preview", expectMaxTokens: false, expectMaxCompletionTokens: true},
		{name: "gpt-5 uses MaxCompletionTokens", model: "gpt-5", expectMaxTokens: false, expectMaxCompletionTokens: true},
		{name: "gpt-5-mini uses MaxCompletionTokens", model: "gpt-5-mini", expectMaxTokens: false, expectMaxCompletionTokens: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedRequest openai.ChatCompletionRequest

			clientMock := &mocks.OpenAIClientMock{
				CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
					capturedRequest = req
					return openai.ChatCompletionResponse{
						Choices: []openai.ChatCompletionChoice{{
							Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"test", "confidence":90}`},
						}},
					}, nil
				},
			}

			checker := newOpenAIChecker(clientMock, OpenAIConfig{
				Model:             tt.model,
				MaxTokensResponse: 100,
			})

			checker.check(context.Background(), "test message", llmContext{})

			if tt.expectMaxTokens {
				assert.Equal(t, 100, capturedRequest.MaxTokens, "MaxTokens should be set for model %s", tt.model)
				assert.Equal(t, 0, capturedRequest.MaxCompletionTokens, "MaxCompletionTokens should not be set for model %s", tt.model)
			}
			if tt.expectMaxCompletionTokens {
				assert.Equal(t, 0, capturedRequest.MaxTokens, "MaxTokens should not be set for model %s", tt.model)
				assert.Equal(t, 100, capturedRequest.MaxCompletionTokens, "MaxCompletionTokens should be set for model %s", tt.model)
			}
		})
	}
}

func TestOpenAIChecker_TruncateUTF8(t *testing.T) {
	var capturedMsg string
	clientMock := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			capturedMsg = req.Messages[1].Content
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"ok", "confidence":100}`},
				}},
			}, nil
		},
	}

	msg := "Привет🌞"

	t.Run("truncate in middle of 2-byte char", func(t *testing.T) {

		checker := newOpenAIChecker(clientMock, OpenAIConfig{
			MaxSymbolsRequest: 1,
			MaxTokensRequest:  0,
		})

		checker.check(context.Background(), msg, llmContext{})
		assert.True(t, utf8.ValidString(capturedMsg), "Truncated string should be valid UTF-8. Got: %x", capturedMsg)
	})
}
