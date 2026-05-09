package tgspam

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/lib/spamcheck"
)

func TestAppendHistoryToLLMMessage(t *testing.T) {
	t.Run("no history", func(t *testing.T) {
		assert.Equal(t, "hello world", appendHistoryToLLMMessage("hello world", llmContext{}))
	})

	t.Run("with history", func(t *testing.T) {
		history := llmContext{
			RequestContext: "manual report",
			RecentChatMessages: []spamcheck.Request{
				{Msg: "first message", UserName: "user1"},
				{Msg: "second message", UserName: ""},
			},
		}

		got := appendHistoryToLLMMessage("current message", history)

		assert.Equal(t, `Moderation context:
manual report

Current checked user message:
current message

Recent chat messages:
"user1": "first message"
"": "second message"
`, got)
	})
}

func TestRunLLMProviderCheck(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		calls := 0
		spam, details := runLLMProviderCheck(context.Background(), llmCheckParams{
			Name: "openai", ErrorPrefix: "OpenAI", RetryCount: 3, Msg: "test message",
			Send: func(_ context.Context, msg string) (llmResponse, error) {
				calls++
				assert.Equal(t, "test message", msg)
				return llmResponse{IsSpam: true, Reason: "bad text.", Confidence: 91}, nil
			},
		})

		assert.True(t, spam)
		assert.Equal(t, 1, calls)
		assert.Equal(t, "openai", details.Name)
		assert.Equal(t, "bad text, confidence: 91%", details.Details)
		assert.NoError(t, details.Error)
	})

	t.Run("retries until success", func(t *testing.T) {
		calls := 0
		history := llmContext{RecentChatMessages: []spamcheck.Request{{Msg: "prev", UserName: "alice"}}}

		spam, details := runLLMProviderCheck(context.Background(), llmCheckParams{
			Name: "gemini", ErrorPrefix: "Gemini", RetryCount: 3, Msg: "current", History: history,
			Send: func(_ context.Context, msg string) (llmResponse, error) {
				calls++
				assert.Equal(t, "Current checked user message:\ncurrent\n\nRecent chat messages:\n\"alice\": \"prev\"\n", msg)
				if calls < 3 {
					return llmResponse{}, errors.New("temporary failure")
				}
				return llmResponse{IsSpam: false, Reason: "looks fine", Confidence: 42}, nil
			},
		})

		assert.False(t, spam)
		assert.Equal(t, 3, calls)
		assert.Equal(t, "gemini", details.Name)
		assert.Equal(t, "looks fine, confidence: 42%", details.Details)
		assert.NoError(t, details.Error)
	})

	t.Run("retry count defaults to one", func(t *testing.T) {
		calls := 0

		spam, details := runLLMProviderCheck(context.Background(), llmCheckParams{
			Name: "gemini", ErrorPrefix: "Gemini", RetryCount: 0, Msg: "test", History: llmContext{},
			Send: func(_ context.Context, msg string) (llmResponse, error) {
				calls++
				return llmResponse{}, errors.New("boom")
			},
		})

		require.False(t, spam)
		assert.Equal(t, 1, calls)
		assert.Equal(t, "gemini", details.Name)
		assert.Equal(t, "Gemini error: boom", details.Details)
		require.Error(t, details.Error)
		assert.EqualError(t, details.Error, "boom")
	})

	t.Run("retry count capped at twenty", func(t *testing.T) {
		calls := 0

		spam, details := runLLMProviderCheck(context.Background(), llmCheckParams{
			Name: "openai", ErrorPrefix: "OpenAI", RetryCount: 99, Msg: "test",
			Send: func(_ context.Context, msg string) (llmResponse, error) {
				calls++
				return llmResponse{}, errors.New("bad json")
			},
		})

		require.False(t, spam)
		assert.Equal(t, 20, calls)
		require.Error(t, details.Error)
		assert.EqualError(t, details.Error, "bad json")
	})
}

func TestParseLLMResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    llmResponse
		wantErr string
	}{
		{
			name:  "strict json",
			input: `{"spam": true, "reason":"bad text", "confidence":91}`,
			want:  llmResponse{IsSpam: true, Reason: "bad text", Confidence: 91},
		},
		{
			name:  "wrapped json object",
			input: `Answer: {"spam": false, "reason":"ok", "confidence":42} done`,
			want:  llmResponse{IsSpam: false, Reason: "ok", Confidence: 42},
		},
		{
			name:  "trailing comma json",
			input: `{"spam": true, "reason":"bad text", "confidence":91,}`,
			want:  llmResponse{IsSpam: true, Reason: "bad text", Confidence: 91},
		},
		{
			name:  "fallback field parse",
			input: `spam: true, reason: "job scam", confidence: 95`,
			want:  llmResponse{IsSpam: true, Reason: "job scam", Confidence: 95},
		},
		{
			name:    "invalid content",
			input:   `nonsense`,
			wantErr: "can't unmarshal response: nonsense",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLLMResponse(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
