package tgspam

import (
	"context"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"github.com/umputun/tg-spam/lib/tgspam/mocks"
	"testing"
)

func TestOpenAIChecker_Check(t *testing.T) {
	clientMock := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(contextMoqParam context.Context, chatCompletionRequest openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{Content: ""},
					},
				},
			}, nil
		},
	}

	checker := newOpenAIChecker(clientMock, OpenAIConfig{
		MaxTokensResponse: 300,
		MaxTokensRequest:  3000,
		MaxSymbolsRequest: 12000,
		Model:             "gpt-4o-mini",
		RetryCount:        2,
	})

	t.Run("spam response", func(t *testing.T) {
		clientMock.CreateChatCompletionFunc = func(
			contextMoqParam context.Context, chatCompletionRequest openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"bad text", "confidence":100}`},
				}},
			}, nil
		}
		spam, details := checker.check(context.Background(), "some text", llmContext{})
		t.Logf("spam: %v, details: %+v", spam, details)
		assert.True(t, spam)
		assert.Equal(t, "openai", details.Name)
		assert.Equal(t, "bad text, confidence: 100%", details.Details)
		assert.NoError(t, details.Error)
	})

	t.Run("not spam response", func(t *testing.T) {
		clientMock.CreateChatCompletionFunc = func(
			contextMoqParam context.Context, chatCompletionRequest openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"good text", "confidence":99}`},
				}},
			}, nil
		}
		spam, details := checker.check(context.Background(), "some text", llmContext{})
		t.Logf("spam: %v, details: %+v", spam, details)
		assert.False(t, spam)
		assert.Equal(t, "openai", details.Name)
		assert.Equal(t, "good text, confidence: 99%", details.Details)
		assert.NoError(t, details.Error)
	})

	t.Run("error response", func(t *testing.T) {
		clientMock.CreateChatCompletionFunc = func(
			contextMoqParam context.Context, chatCompletionRequest openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{}, assert.AnError
		}
		spam, details := checker.check(context.Background(), "some text", llmContext{})
		t.Logf("spam: %v, details: %+v", spam, details)
		assert.False(t, spam)
		assert.Equal(t, "openai", details.Name)
		assert.Equal(t, "OpenAI error: failed to create chat completion: assert.AnError general error for testing", details.Details)
		assert.Equal(t, "failed to create chat completion: assert.AnError general error for testing", details.Error.Error())
	})

	t.Run("bad encoding", func(t *testing.T) {
		callCount := 0
		clientMock.CreateChatCompletionFunc = func(
			contextMoqParam context.Context, chatCompletionRequest openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			callCount++
			if callCount == 1 {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `bad json`},
					}},
				}, nil
			}
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"recovered", "confidence":88}`},
				}},
			}, nil
		}
		spam, details := checker.check(context.Background(), "some text", llmContext{})
		t.Logf("spam: %v, details: %+v", spam, details)
		assert.True(t, spam)
		assert.Equal(t, "openai", details.Name)
		assert.Equal(t, "recovered, confidence: 88%", details.Details)
		assert.NoError(t, details.Error)
		assert.Equal(t, 2, callCount)
	})

	t.Run("no choices", func(t *testing.T) {
		clientMock.CreateChatCompletionFunc = func(
			contextMoqParam context.Context, chatCompletionRequest openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{}, nil
		}
		spam, details := checker.check(context.Background(), "some text", llmContext{})
		t.Logf("spam: %v, details: %+v", spam, details)
		assert.False(t, spam)
		assert.Equal(t, "openai", details.Name)
		assert.Equal(t, "OpenAI error: no choices in response", details.Details)
	})

	t.Run("fallback parser handles wrapped json", func(t *testing.T) {
		clientMock.CreateChatCompletionFunc = func(
			contextMoqParam context.Context, chatCompletionRequest openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `analysis {"spam": true, "reason":"wrapped", "confidence":87,} done`},
				}},
			}, nil
		}
		spam, details := checker.check(context.Background(), "some text", llmContext{})
		assert.True(t, spam)
		assert.Equal(t, "openai", details.Name)
		assert.Equal(t, "wrapped, confidence: 87%", details.Details)
		assert.NoError(t, details.Error)
	})
}

func TestOpenAIChecker_CheckWithHistory(t *testing.T) {
	clientMock := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(contextMoqParam context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {

			assert.Contains(t, req.Messages[1].Content, "Recent chat messages:")
			assert.Contains(t, req.Messages[1].Content, `"user1": "first message"`)
			assert.Contains(t, req.Messages[1].Content, `"user2": "second message"`)
			assert.Contains(t, req.Messages[1].Content, `"user1": "third message"`)

			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"suspicious pattern in history", "confidence":90}`},
				}},
			}, nil
		},
	}

	checker := newOpenAIChecker(clientMock, OpenAIConfig{Model: "gpt-4o-mini"})
	history := llmContext{RecentChatMessages: []spamcheck.Request{
		{Msg: "first message", UserName: "user1"},
		{Msg: "second message", UserName: "user2"},
		{Msg: "third message", UserName: "user1"},
	}}

	spam, details := checker.check(context.Background(), "current message", history)
	t.Logf("spam: %v, details: %+v", spam, details)
	assert.True(t, spam)
	assert.Equal(t, "openai", details.Name)
	assert.Equal(t, "suspicious pattern in history, confidence: 90%", details.Details)
	require.NoError(t, details.Error)
	assert.Len(t, clientMock.CreateChatCompletionCalls(), 1)
}

func TestOpenAIChecker_FormatMessage(t *testing.T) {
	var capturedMsg string
	clientMock := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(contextMoqParam context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			capturedMsg = req.Messages[1].Content
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"test message", "confidence":90}`},
				}},
			}, nil
		},
	}

	tests := []struct {
		name            string
		currentMsg      string
		history         llmContext
		expectedMessage string
	}{
		{
			name:            "message with no history",
			currentMsg:      "hello world",
			history:         llmContext{},
			expectedMessage: "hello world",
		},
		{
			name:       "message with history",
			currentMsg: "current message",
			history: llmContext{RecentChatMessages: []spamcheck.Request{
				{Msg: "first message", UserName: "user1"},
				{Msg: "second message", UserName: "user2"},
			}},
			expectedMessage: `User message:
current message

Recent chat messages:
"user1": "first message"
"user2": "second message"
`,
		},
		{
			name:       "message with empty username in history",
			currentMsg: "current message",
			history: llmContext{RecentChatMessages: []spamcheck.Request{
				{Msg: "first message", UserName: ""},
				{Msg: "second message", UserName: "user2"},
			}},
			expectedMessage: `User message:
current message

Recent chat messages:
"": "first message"
"user2": "second message"
`,
		},
		{
			name:       "message with chat and same-user history",
			currentMsg: "current message",
			history: llmContext{
				RecentChatMessages: []spamcheck.Request{
					{Msg: "chat one", UserName: "user1"},
				},
				RecentUserMessages: []spamcheck.Request{
					{Msg: "user one", UserName: "user1"},
				},
			},
			expectedMessage: `User message:
current message

Recent chat messages:
"user1": "chat one"

Recent messages from the same user:
"user1": "user one"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientMock.ResetCalls()
			checker := newOpenAIChecker(clientMock, OpenAIConfig{Model: "gpt-4o-mini"})
			checker.check(context.Background(), tt.currentMsg, tt.history)
			assert.Equal(t, tt.expectedMessage, capturedMsg, "message formatting mismatch")
			assert.Len(t, clientMock.CreateChatCompletionCalls(), 1)
		})
	}
}

func TestStripThoughtTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no thought tags",
			input:    "This is a normal response",
			expected: "This is a normal response",
		},
		{
			name:     "single thought tag",
			input:    "This is <thought>some thinking process</thought> a response with thoughts",
			expected: "This is  a response with thoughts",
		},
		{
			name:     "multiple thought tags",
			input:    "<thought>Initial thinking</thought>Response<thought>More thinking</thought>",
			expected: "Response",
		},
		{
			name:     "multiline thought tags",
			input:    "Start\n<thought>\nMultiline\nthinking\n</thought>\nEnd",
			expected: "Start\n\nEnd",
		},
		{
			name:     "JSON content with thought tags",
			input:    `{"result": "<thought>thinking about spam</thought>This is spam"}`,
			expected: `{"result": "This is spam"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripThoughtTags(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReasoningEffortInRequest(t *testing.T) {
	tests := []struct {
		name            string
		reasoningEffort string
		expectInRequest bool
		expectedEffort  string
	}{
		{
			name:            "empty reasoning effort",
			reasoningEffort: "",
			expectInRequest: false,
		},
		{
			name:            "none reasoning effort",
			reasoningEffort: "none",
			expectInRequest: false,
		},
		{
			name:            "low reasoning effort",
			reasoningEffort: "low",
			expectInRequest: true,
			expectedEffort:  "low",
		},
		{
			name:            "medium reasoning effort",
			reasoningEffort: "medium",
			expectInRequest: true,
			expectedEffort:  "medium",
		},
		{
			name:            "high reasoning effort",
			reasoningEffort: "high",
			expectInRequest: true,
			expectedEffort:  "high",
		},
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
				Model:           "gpt-4o-mini",
				ReasoningEffort: tt.reasoningEffort,
			})

			checker.check(context.Background(), "test message", llmContext{})

			if tt.expectInRequest {
				require.Equal(t, tt.expectedEffort, capturedRequest.ReasoningEffort)
			} else {
				require.Empty(t, capturedRequest.ReasoningEffort)
			}
		})
	}
}

func TestBuildSystemPromptWithCustomPrompts(t *testing.T) {
	tests := []struct {
		name          string
		systemPrompt  string
		customPrompts []string
		expected      string
	}{
		{
			name:          "with empty custom prompts",
			systemPrompt:  "Base prompt",
			customPrompts: []string{},
			expected:      "Base prompt",
		},
		{
			name:          "with one custom prompt",
			systemPrompt:  "Base prompt",
			customPrompts: []string{"Check for pattern X"},
			expected:      "Base prompt\n\nAlso, specifically check for these patterns:\n1. Check for pattern X\n",
		},
		{
			name:          "with multiple custom prompts",
			systemPrompt:  "Base prompt",
			customPrompts: []string{"Check for pattern X", "Check for pattern Y", "Watch for price patterns like $X,XXX"},
			expected:      "Base prompt\n\nAlso, specifically check for these patterns:\n1. Check for pattern X\n2. Check for pattern Y\n3. Watch for price patterns like $X,XXX\n",
		},
		{
			name:          "with default prompt when system prompt is empty",
			systemPrompt:  "",
			customPrompts: []string{"Custom pattern check"},
			expected:      defaultPrompt + "\n\nAlso, specifically check for these patterns:\n1. Custom pattern check\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientMock := &mocks.OpenAIClientMock{
				CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
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

			result := checker.buildSystemPrompt()
			assert.Equal(t, tt.expected, result)
		})
	}
}
