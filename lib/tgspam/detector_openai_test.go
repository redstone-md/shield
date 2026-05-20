package tgspam

import (
	"context"
	"errors"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/redstone-md/shield/lib/tgspam/mocks"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
)

func TestDetector_CheckOpenAI(t *testing.T) {
	t.Run("with openai and first-only", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"bad text", "confidence":100}`},
					}},
				}, nil
			},
		}
		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4"})
		spam, cr := d.Check(spamcheck.Request{Msg: "some message 1234"})
		assert.True(t, spam)
		require.Len(t, cr, 1)
		assert.Equal(t, "openai", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "bad text, confidence: 100%", cr[0].Details)
		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("with openai and not first-only", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"bad text", "confidence":100}`},
					}},
				}, nil
			},
		}
		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4"})
		spam, cr := d.Check(spamcheck.Request{Msg: "some message 1234"})
		assert.True(t, spam)
		require.Len(t, cr, 1)
		assert.Equal(t, "openai", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "bad text, confidence: 100%", cr[0].Details)
		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("without openai", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1})
		spam, cr := d.Check(spamcheck.Request{Msg: "some message 1234"})
		assert.False(t, spam)
		require.Empty(t, cr)
	})

	t.Run("with openai, first-only but spam detected before", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"bad text", "confidence":100}`},
					}},
				}, nil
			},
		}
		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4"})
		_, err := d.LoadStopWords(strings.NewReader("some message"))
		require.NoError(t, err)

		spam, cr := d.Check(spamcheck.Request{Msg: "some message 1234"})
		assert.True(t, spam)
		require.Len(t, cr, 1)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "some message", cr[0].Details)
		assert.Empty(t, mockOpenAIClient.CreateChatCompletionCalls())
	})

	t.Run("with openai, first-only spam detected before, veto passes", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, OpenAIVeto: true})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"bad text", "confidence":100}`},
					}},
				}, nil
			},
		}
		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4"})
		_, err := d.LoadStopWords(strings.NewReader("some message"))
		require.NoError(t, err)

		spam, cr := d.Check(spamcheck.Request{Msg: "some message 1234"})
		assert.True(t, spam)
		require.Len(t, cr, 2)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "some message", cr[0].Details)

		assert.Equal(t, "openai", cr[1].Name)
		assert.True(t, cr[1].Spam)
		assert.Equal(t, "bad text, confidence: 100%", cr[1].Details)

		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("with openai, first-only spam detected before, veto failed", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, OpenAIVeto: true})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"good text", "confidence":100}`},
					}},
				}, nil
			},
		}
		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4"})
		_, err := d.LoadStopWords(strings.NewReader("some message"))
		require.NoError(t, err)
		spam, cr := d.Check(spamcheck.Request{Msg: "some message 1234"})
		assert.False(t, spam)
		require.Len(t, cr, 2)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "some message", cr[0].Details)

		assert.Equal(t, "openai", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "good text, confidence: 100%", cr[1].Details)

		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("with openai, first-only spam detected before, openai error", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, OpenAIVeto: true})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"good text", "confidence":100}`},
					}},
				}, errors.New("openai error")
			},
		}
		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4"})
		_, err := d.LoadStopWords(strings.NewReader("some message"))
		require.NoError(t, err)
		spam, cr := d.Check(spamcheck.Request{Msg: "some message 1234"})
		assert.True(t, spam)
		require.Len(t, cr, 2)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "some message", cr[0].Details)

		assert.Equal(t, "openai", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "OpenAI error: failed to create chat completion: openai error", cr[1].Details)
		assert.Equal(t, "failed to create chat completion: openai error", cr[1].Error.Error())

		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("with openai, first-only spam not detected before", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, OpenAIVeto: false})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"bad text", "confidence":100}`},
					}},
				}, nil
			},
		}
		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4"})
		_, err := d.LoadStopWords(strings.NewReader("some message"))
		require.NoError(t, err)

		spam, cr := d.Check(spamcheck.Request{Msg: "1234"})
		assert.True(t, spam)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "not found", cr[0].Details)

		assert.Equal(t, "openai", cr[1].Name)
		assert.True(t, cr[1].Spam)
		assert.Equal(t, "bad text, confidence: 100%", cr[1].Details)

		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("with openai and MinMsgLen - short message skips openai by default", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, MinMsgLen: 50})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"bad text", "confidence":100}`},
					}},
				}, nil
			},
		}

		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4"})

		spam, cr := d.Check(spamcheck.Request{Msg: "short msg"})
		assert.False(t, spam)
		require.Len(t, cr, 1)
		assert.Equal(t, "message length", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "too short", cr[0].Details)

		assert.Empty(t, mockOpenAIClient.CreateChatCompletionCalls())

		spam2, cr2 := d.Check(spamcheck.Request{Msg: "this is a much longer message that exceeds the minimum length requirement"})
		assert.True(t, spam2)
		require.Len(t, cr2, 1)
		assert.Equal(t, "openai", cr2[0].Name)
		assert.True(t, cr2[0].Spam)
		assert.Equal(t, "bad text, confidence: 100%", cr2[0].Details)

		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("with openai and MinMsgLen - short message checked when flag is true", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, MinMsgLen: 50})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"bad text", "confidence":100}`},
					}},
				}, nil
			},
		}

		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4", CheckShortMessagesWithOpenAI: true})

		spam, cr := d.Check(spamcheck.Request{Msg: "short msg"})
		assert.True(t, spam)
		require.Len(t, cr, 2)
		assert.Equal(t, "message length", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "too short", cr[0].Details)
		assert.Equal(t, "openai", cr[1].Name)
		assert.True(t, cr[1].Spam)
		assert.Equal(t, "bad text, confidence: 100%", cr[1].Details)

		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)

		spam2, cr2 := d.Check(spamcheck.Request{Msg: "this is a much longer message that exceeds the minimum length requirement"})
		assert.True(t, spam2)
		require.Len(t, cr2, 1)
		assert.Equal(t, "openai", cr2[0].Name)
		assert.True(t, cr2[0].Spam)
		assert.Equal(t, "bad text, confidence: 100%", cr2[0].Details)

		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 2)
	})

	t.Run("with openai and MinMsgLen - short message already spam skips openai", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, MinMsgLen: 50})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"bad text", "confidence":100}`},
					}},
				}, nil
			},
		}

		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4", CheckShortMessagesWithOpenAI: true})

		_, err := d.LoadStopWords(strings.NewReader("viagra"))
		require.NoError(t, err)

		spam, cr := d.Check(spamcheck.Request{Msg: "buy viagra"})
		assert.True(t, spam)
		require.Len(t, cr, 2)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "viagra", cr[0].Details)
		assert.Equal(t, "message length", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "too short", cr[1].Details)

		assert.Empty(t, mockOpenAIClient.CreateChatCompletionCalls())
	})

	t.Run("with openai enabled for short messages - still skips classifier/similarity", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, MinMsgLen: 50, SimilarityThreshold: 0.5})

		spamSamples := strings.NewReader("buy cheap viagra now\nclick here for free money")
		hamSamples := strings.NewReader("hello world\nhow are you")
		lr, err := d.LoadSamples(strings.NewReader(""), []io.Reader{spamSamples}, []io.Reader{hamSamples})
		require.NoError(t, err)
		assert.Positive(t, lr.SpamSamples)
		assert.Positive(t, lr.HamSamples)

		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"too short to tell", "confidence":30}`},
					}},
				}, nil
			},
		}
		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{Model: "gpt4", CheckShortMessagesWithOpenAI: true})

		spam, cr := d.Check(spamcheck.Request{Msg: "hi there"})
		assert.False(t, spam)

		hasMessageLength := false
		hasOpenAI := false
		for _, r := range cr {
			switch r.Name {
			case "message length":
				hasMessageLength = true
				assert.False(t, r.Spam)
				assert.Equal(t, "too short", r.Details)
			case "openai":
				hasOpenAI = true
				assert.False(t, r.Spam)
			case "classifier":
				t.Error("classifier should not run for short messages even with openai enabled")
			case "similarity":
				t.Error("similarity should not run for short messages even with openai enabled")
			}
		}

		assert.True(t, hasMessageLength, "should have message length check")
		assert.True(t, hasOpenAI, "should have openai check when enabled for short messages")
		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("short message with CheckShortMessagesWithOpenAI ignores veto mode", func(t *testing.T) {

		d := NewDetector(Config{
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
			MinMsgLen:        50,
			OpenAIVeto:       true,
		})

		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"suspicious short message", "confidence":95}`},
					}},
				}, nil
			},
		}

		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{
			Model:                        "gpt4",
			CheckShortMessagesWithOpenAI: true,
		})

		spam, cr := d.Check(spamcheck.Request{Msg: "short msg"})

		assert.True(t, spam)
		require.Len(t, cr, 2)

		assert.Equal(t, "message length", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "too short", cr[0].Details)

		assert.Equal(t, "openai", cr[1].Name)
		assert.True(t, cr[1].Spam)
		assert.Equal(t, "suspicious short message, confidence: 95%", cr[1].Details)

		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("short message with CheckShortMessagesWithOpenAI and OpenAIVeto=false", func(t *testing.T) {

		d := NewDetector(Config{
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
			MinMsgLen:        50,
			OpenAIVeto:       false,
		})

		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"looks fine", "confidence":90}`},
					}},
				}, nil
			},
		}

		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{
			Model:                        "gpt4",
			CheckShortMessagesWithOpenAI: true,
		})

		spam, cr := d.Check(spamcheck.Request{Msg: "hi there"})

		assert.False(t, spam)
		require.Len(t, cr, 2)

		assert.Equal(t, "message length", cr[0].Name)
		assert.Equal(t, "openai", cr[1].Name)
		assert.False(t, cr[1].Spam)

		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("short message with veto mode - OpenAI returns ham", func(t *testing.T) {

		d := NewDetector(Config{
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
			MinMsgLen:        50,
			OpenAIVeto:       true,
		})

		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"looks clean", "confidence":85}`},
					}},
				}, nil
			},
		}

		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{
			Model:                        "gpt4",
			CheckShortMessagesWithOpenAI: true,
		})

		spam, cr := d.Check(spamcheck.Request{Msg: "hello"})

		assert.False(t, spam)
		require.Len(t, cr, 2)

		assert.Equal(t, "message length", cr[0].Name)
		assert.Equal(t, "openai", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "looks clean, confidence: 85%", cr[1].Details)

		assert.Len(t, mockOpenAIClient.CreateChatCompletionCalls(), 1)
	})

	t.Run("with openai and image-only empty text skips text llm", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, MinMsgLen: 50})
		mockOpenAIClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"bad image", "confidence":100}`},
					}},
				}, nil
			},
		}

		d.WithOpenAIChecker(mockOpenAIClient, OpenAIConfig{
			Model:                        "gpt4",
			CheckShortMessagesWithOpenAI: true,
		})

		spam, cr := d.Check(spamcheck.Request{Msg: "", Meta: spamcheck.MetaData{Images: 1}})
		assert.False(t, spam)
		assert.NotEmpty(t, cr)
		assert.Empty(t, mockOpenAIClient.CreateChatCompletionCalls())
		for _, check := range cr {
			assert.NotEqual(t, "openai", check.Name)
		}
	})
}
