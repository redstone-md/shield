package tgspam

import (
	"bytes"
	"context"
	"fmt"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/redstone-md/shield/lib/tgspam/mocks"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDetector_RemoveSpamHam(t *testing.T) {
	updSpam := &mocks.SampleUpdaterMock{
		RemoveFunc: func(msg string) error {
			return nil
		},
		AppendFunc: func(msg string) error {
			return nil
		},
	}

	d := NewDetector(Config{MaxAllowedEmoji: -1})
	d.WithSpamUpdater(updSpam)

	spamSamples := strings.NewReader("lottery prize xyz")
	hamsSamples := strings.NewReader("hello world\nhow are you\nhave a good day")
	lr, err := d.LoadSamples(strings.NewReader("xyz"), []io.Reader{spamSamples}, []io.Reader{hamsSamples})
	require.NoError(t, err)
	assert.Equal(t, LoadResult{ExcludedTokens: 1, SpamSamples: 1, HamSamples: 3}, lr)

	spamMsg := "win free iPhone"
	err = d.UpdateSpam(spamMsg)
	require.NoError(t, err)

	t.Run("initially classified as spam", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "win free iPhone hello world"})
		t.Logf("%+v", cr)
		require.True(t, spam, "should initially be classified as spam")
		require.NotEmpty(t, cr, "should have classification results")
		assert.Equal(t, "classifier", cr[0].Name)
		assert.True(t, cr[0].Spam)
	})

	err = d.RemoveSpam(spamMsg)
	require.NoError(t, err)
	assert.Len(t, updSpam.RemoveCalls(), 1)
	assert.Equal(t, spamMsg, updSpam.RemoveCalls()[0].Msg)

	t.Run("after removing spam", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "win free iPhone hello world"})
		t.Logf("%+v", cr)
		require.NotEmpty(t, cr, "should have classification results")
		assert.Equal(t, "classifier", cr[0].Name)
		assert.False(t, spam, "should no longer be classified as spam")
		assert.False(t, cr[0].Spam)
	})

	t.Run("error on updater", func(t *testing.T) {
		failingUpd := &mocks.SampleUpdaterMock{
			RemoveFunc: func(msg string) error {
				return fmt.Errorf("remove error")
			},
		}
		d.WithSpamUpdater(failingUpd)
		err := d.RemoveSpam(spamMsg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "can't unlearn spam samples")
	})

	t.Run("error on non-existent message", func(t *testing.T) {
		err := d.RemoveSpam("not-learned-message")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "can't unlearn spam samples")
	})
}

func TestDetector_buildDocs(t *testing.T) {
	d := &Detector{excludedTokens: map[string]struct{}{"the": {}, "and": {}}}

	docs := d.buildDocs("buy crypto coins now", "spam")
	assert.Len(t, docs, 1, "should create single document")
	assert.Equal(t, spamClass("spam"), docs[0].spamClass)
	assert.ElementsMatch(t, []string{"buy", "crypto", "coins", "now"}, docs[0].tokens)
}

func TestDetector_CheckHistory(t *testing.T) {
	d := NewDetector(Config{HistorySize: 5})
	_, err := d.LoadStopWords(strings.NewReader("spam text\nbad text"))
	require.NoError(t, err)

	isSpam, _ := d.Check(spamcheck.Request{Msg: "good message", UserID: "1"})
	assert.False(t, isSpam)
	hamMsgs := d.hamHistory.Last(5)
	require.Len(t, hamMsgs, 1)
	assert.Equal(t, "good message", hamMsgs[0].Msg)
	assert.Empty(t, d.spamHistory.Last(5))

	isSpam, _ = d.Check(spamcheck.Request{Msg: "spam text", UserID: "1"})
	assert.True(t, isSpam)
	spamMsgs := d.spamHistory.Last(5)
	require.Len(t, spamMsgs, 1)
	assert.Equal(t, "spam text", spamMsgs[0].Msg)
	assert.Len(t, d.hamHistory.Last(5), 1, "ham history should remain unchanged")

	isSpam, _ = d.Check(spamcheck.Request{Msg: "another good one", UserID: "2"})
	assert.False(t, isSpam)
	hamMsgs = d.hamHistory.Last(5)
	require.Len(t, hamMsgs, 2)
	assert.Equal(t, "good message", hamMsgs[0].Msg)
	assert.Equal(t, "another good one", hamMsgs[1].Msg)
	assert.Len(t, d.spamHistory.Last(5), 1, "spam history should remain unchanged")
}

func TestDetector_CheckHistory_ShortMessagesNotAddedToHam(t *testing.T) {
	t.Run("short ham message without openai should not be added to history", func(t *testing.T) {
		d := NewDetector(Config{HistorySize: 5, MinMsgLen: 10})

		isSpam, cr := d.Check(spamcheck.Request{Msg: "hi", UserID: "1"})
		assert.False(t, isSpam)
		assert.Contains(t, spamcheck.ChecksToString(cr), "message length")

		hamMsgs := d.hamHistory.Last(5)
		assert.Empty(t, hamMsgs, "short unchecked messages should not be added to hamHistory")
	})

	t.Run("short ham message with openai enabled should be added to history", func(t *testing.T) {
		d := NewDetector(Config{HistorySize: 5, MinMsgLen: 10, FirstMessageOnly: true})
		mockClient := &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: `{"spam":false,"reason":"good","confidence":30}`}}},
				}, nil
			},
		}
		d.WithOpenAIChecker(mockClient, OpenAIConfig{CheckShortMessagesWithOpenAI: true})

		isSpam, _ := d.Check(spamcheck.Request{Msg: "hi", UserID: "1", UserName: "user1"})
		assert.False(t, isSpam)

		hamMsgs := d.hamHistory.Last(5)
		assert.Len(t, hamMsgs, 1, "short messages checked by openai should be added to hamHistory")
		assert.Equal(t, "hi", hamMsgs[0].Msg)
	})

	t.Run("normal length ham message should be added to history", func(t *testing.T) {
		d := NewDetector(Config{HistorySize: 5, MinMsgLen: 10})

		isSpam, _ := d.Check(spamcheck.Request{Msg: "this is a normal length message", UserID: "1"})
		assert.False(t, isSpam)

		hamMsgs := d.hamHistory.Last(5)
		assert.Len(t, hamMsgs, 1, "normal length messages should be added to hamHistory")
		assert.Equal(t, "this is a normal length message", hamMsgs[0].Msg)
	})
}

func TestNewDetector_DefaultLLMConsensus(t *testing.T) {
	d := NewDetector(Config{})
	assert.Equal(t, LLMConsensusAny, d.LLMConsensus)
}

func TestDetector_LLMModeAlwaysChecksHamAndSpamBase(t *testing.T) {
	tests := []struct {
		name     string
		baseSpam bool
		wantSpam bool
	}{
		{name: "checks ham base", wantSpam: true},
		{name: "checks spam base", baseSpam: true, wantSpam: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, LLMMode: LLMModeAlways})
			req := spamcheck.Request{Msg: "normal looking message"}
			llmSpam := true
			if tc.baseSpam {
				_, err := d.LoadStopWords(strings.NewReader("spamword"))
				require.NoError(t, err)
				req.Msg = "spamword message"
				llmSpam = false
			}

			openAIMock := &mocks.OpenAIClientMock{
				CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
					return openai.ChatCompletionResponse{
						Choices: []openai.ChatCompletionChoice{{
							Message: openai.ChatCompletionMessage{
								Content: fmt.Sprintf(`{"spam": %t, "reason":"always", "confidence":95}`, llmSpam),
							},
						}},
					}, nil
				},
			}
			d.WithOpenAIChecker(openAIMock, OpenAIConfig{Model: "gpt4"})

			spam, cr := d.Check(req)

			assert.Equal(t, tc.wantSpam, spam)
			assert.Len(t, openAIMock.CreateChatCompletionCalls(), 1)
			assert.NotNil(t, findResponseByName(cr, "openai"))
		})
	}
}

func TestDetector_CheckWithLLMInParanoidMode(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5})
	openAIMock := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam": true, "reason":"llm hit", "confidence":95}`},
				}},
			}, nil
		},
	}
	d.WithOpenAIChecker(openAIMock, OpenAIConfig{Model: "gpt4"})

	spam, cr := d.Check(spamcheck.Request{Msg: "totally normal looking message", UserID: "42", UserName: "u42"})
	assert.True(t, spam)
	assert.Len(t, openAIMock.CreateChatCompletionCalls(), 1)
	assert.NotNil(t, findResponseByName(cr, "openai"))
}

func TestDetector_LLMContextOmitsRecentChatMessagesByDefault(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5})
	var captured string
	openAIMock := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			captured = req.Messages[1].Content
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"ok", "confidence":20}`},
				}},
			}, nil
		},
	}
	d.WithOpenAIChecker(openAIMock, OpenAIConfig{Model: "gpt4"})

	_, _ = d.Check(spamcheck.Request{Msg: "previous message", UserID: "1", UserName: "alice"})
	_, _ = d.Check(spamcheck.Request{Msg: "trigger message", UserID: "42", UserName: "u42"})

	assert.NotContains(t, captured, "Recent chat messages:")
	assert.NotContains(t, captured, "previous message")
}

func TestDetector_LLMContextIncludesConfiguredRecentChatMessages(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5, LLMHistoryContextSize: 5})
	var captured string
	openAIMock := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			captured = req.Messages[1].Content
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"ok", "confidence":20}`},
				}},
			}, nil
		},
	}
	d.WithOpenAIChecker(openAIMock, OpenAIConfig{Model: "gpt4"})

	for i := range 6 {
		userID := "other"
		userName := "other"
		if i%2 == 0 {
			userID = "42"
			userName = "u42"
		}
		_, _ = d.Check(spamcheck.Request{
			Msg:      fmt.Sprintf("msg-%d", i),
			UserID:   userID,
			UserName: userName,
		})
	}

	_, _ = d.Check(spamcheck.Request{Msg: "trigger message", UserID: "42", UserName: "u42"})
	assert.Contains(t, captured, "Recent chat messages:")
	assert.NotContains(t, captured, `"u42": "msg-0"`)
	assert.Contains(t, captured, `"other": "msg-1"`)
	assert.Contains(t, captured, `"u42": "msg-2"`)
	assert.Contains(t, captured, `"other": "msg-3"`)
	assert.Contains(t, captured, `"u42": "msg-4"`)
	assert.Contains(t, captured, `"other": "msg-5"`)
	assert.NotContains(t, captured, "Recent messages from the same user:")
}

func TestDetector_LLMContextExcludesSpamMessages(t *testing.T) {
	d := NewDetector(Config{HistorySize: 5, LLMHistoryContextSize: 5, MaxAllowedEmoji: -1, MinMsgLen: 5})
	replies := []string{
		`{"spam": true, "reason":"first is spam", "confidence":95}`,
		`{"spam": false, "reason":"second is ham", "confidence":20}`,
	}
	var captured []string
	openAIMock := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			captured = append(captured, req.Messages[1].Content)
			idx := len(captured) - 1
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: replies[idx]},
				}},
			}, nil
		},
	}
	d.WithOpenAIChecker(openAIMock, OpenAIConfig{Model: "gpt4", CheckShortMessagesWithOpenAI: true})

	spam, _ := d.Check(spamcheck.Request{Msg: "deleted spam offer", UserID: "1", UserName: "spammer"})
	assert.True(t, spam)
	require.Len(t, d.spamHistory.Last(5), 1)

	spam, _ = d.Check(spamcheck.Request{Msg: "ok", UserID: "2", UserName: "regular"})
	assert.False(t, spam)
	require.Len(t, captured, 2)
	assert.NotContains(t, captured[1], "deleted spam offer")
	assert.NotContains(t, captured[1], "spammer")
}

func TestDetector_LLMContextExcludesEarlyShortSpamMessages(t *testing.T) {
	d := NewDetector(Config{HistorySize: 5, LLMHistoryContextSize: 5, MaxAllowedEmoji: 1, MinMsgLen: 10})
	var captured string
	openAIMock := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			captured = req.Messages[1].Content
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"ok", "confidence":20}`},
				}},
			}, nil
		},
	}
	d.WithOpenAIChecker(openAIMock, OpenAIConfig{Model: "gpt4", CheckShortMessagesWithOpenAI: true})

	spam, _ := d.Check(spamcheck.Request{Msg: "🔥🔥", UserID: "1", UserName: "spammer"})
	assert.True(t, spam)
	require.Len(t, d.spamHistory.Last(5), 1)

	spam, _ = d.Check(spamcheck.Request{Msg: "ok", UserID: "2", UserName: "regular"})
	assert.False(t, spam)
	assert.NotContains(t, captured, "🔥🔥")
	assert.NotContains(t, captured, "spammer")
}

func TestDetector_CheckWithLLMConsensus(t *testing.T) {
	makeOpenAIResponse := func(spam bool, reason string, confidence int) openai.ChatCompletionResponse {
		return openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{{
				Message: openai.ChatCompletionMessage{
					Content: fmt.Sprintf(`{"spam": %t, "reason":%q, "confidence":%d}`, spam, reason, confidence),
				},
			}},
		}
	}

	makeGeminiResponse := func(spam bool, reason string, confidence int) *genai.GenerateContentResponse {
		return &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{
					Parts: []*genai.Part{{
						Text: fmt.Sprintf(`{"spam": %t, "reason":%q, "confidence":%d}`, spam, reason, confidence),
					}},
				},
			}},
		}
	}

	tests := []struct {
		name       string
		consensus  LLMConsensusMode
		baseSpam   bool
		openAISpam bool
		geminiSpam bool
		wantSpam   bool
	}{
		{
			name:       "any flips ham base when either llm flags spam",
			consensus:  LLMConsensusAny,
			openAISpam: true,
			geminiSpam: false,
			wantSpam:   true,
		},
		{
			name:       "all requires every llm to flag spam on ham base",
			consensus:  LLMConsensusAll,
			openAISpam: true,
			geminiSpam: false,
			wantSpam:   false,
		},
		{
			name:       "any flips spam base when either veto llm clears spam",
			consensus:  LLMConsensusAny,
			baseSpam:   true,
			openAISpam: false,
			geminiSpam: true,
			wantSpam:   false,
		},
		{
			name:       "all requires every veto llm to clear spam",
			consensus:  LLMConsensusAll,
			baseSpam:   true,
			openAISpam: false,
			geminiSpam: true,
			wantSpam:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDetector(Config{
				MaxAllowedEmoji:  -1,
				FirstMessageOnly: true,
				OpenAIVeto:       tc.baseSpam,
				GeminiVeto:       tc.baseSpam,
				LLMConsensus:     tc.consensus,
			})

			req := spamcheck.Request{Msg: "hello there"}
			if tc.baseSpam {
				_, err := d.LoadStopWords(strings.NewReader("spamword"))
				require.NoError(t, err)
				req.Msg = "spamword message"
			}

			openAIMock := &mocks.OpenAIClientMock{
				CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
					return makeOpenAIResponse(tc.openAISpam, "openai verdict", 95), nil
				},
			}
			geminiMock := &mocks.GeminiClientMock{
				GenerateContentFunc: func(ctx context.Context, model string, contents []*genai.Content,
					config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
					return makeGeminiResponse(tc.geminiSpam, "gemini verdict", 95), nil
				},
			}

			d.WithOpenAIChecker(openAIMock, OpenAIConfig{Model: "gpt4"})
			d.WithGeminiChecker(geminiMock, GeminiConfig{Model: "gemma-4-31b-it"})

			spam, cr := d.Check(req)
			assert.Equal(t, tc.wantSpam, spam)
			assert.Len(t, openAIMock.CreateChatCompletionCalls(), 1)
			assert.Len(t, geminiMock.GenerateContentCalls(), 1)

			checkNames := make([]string, 0, len(cr))
			for _, check := range cr {
				checkNames = append(checkNames, check.Name)
			}
			assert.Contains(t, checkNames, "openai")
			assert.Contains(t, checkNames, "gemini")
		})
	}
}

func TestDetector_CASNonVetoable(t *testing.T) {
	casResponder := func(body string) *mocks.HTTPClientMock {
		return &mocks.HTTPClientMock{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(body)),
				}, nil
			},
		}
	}
	hamOpenAI := func() *mocks.OpenAIClientMock {
		return &mocks.OpenAIClientMock{
			CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
				return openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{{
						Message: openai.ChatCompletionMessage{Content: `{"spam": false, "reason":"looks fine", "confidence":15}`},
					}},
				}, nil
			},
		}
	}

	t.Run("cas hit survives openai ham veto", func(t *testing.T) {
		d := NewDetector(Config{
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
			CasAPI:           "http://localhost",
			HTTPClient:       casResponder(`{"ok": true, "description": "banned"}`),
			OpenAIVeto:       true,
		})
		openAIMock := hamOpenAI()
		d.WithOpenAIChecker(openAIMock, OpenAIConfig{Model: "gpt4"})

		spam, cr := d.Check(spamcheck.Request{Msg: "hello there friends", UserID: "12345"})
		assert.True(t, spam, "cas hit must not be vetoed by openai ham verdict")
		assert.Len(t, openAIMock.CreateChatCompletionCalls(), 1, "openai still runs as veto")

		var casSpam, openaiHam bool
		for _, c := range cr {
			switch c.Name {
			case "cas":
				casSpam = c.Spam
			case "openai":
				openaiHam = !c.Spam
			}
		}
		assert.True(t, casSpam, "cas flagged spam")
		assert.True(t, openaiHam, "openai returned ham")
	})

	t.Run("no cas hit still allows openai veto", func(t *testing.T) {
		d := NewDetector(Config{
			MaxAllowedEmoji:  -1,
			FirstMessageOnly: true,
			CasAPI:           "http://localhost",
			HTTPClient:       casResponder(`{"ok": false, "description": "not found"}`),
			OpenAIVeto:       true,
		})
		_, err := d.LoadStopWords(strings.NewReader("spamword"))
		require.NoError(t, err)
		openAIMock := hamOpenAI()
		d.WithOpenAIChecker(openAIMock, OpenAIConfig{Model: "gpt4"})

		spam, _ := d.Check(spamcheck.Request{Msg: "spamword message here", UserID: "999"})
		assert.False(t, spam, "without cas hit openai veto still clears spam")
	})
}

func TestDetector_CheckWithShortMessageRunsOnlyEligibleLLMs(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1, FirstMessageOnly: true, MinMsgLen: 50})

	openAIMock := &mocks.OpenAIClientMock{
		CreateChatCompletionFunc: func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{{
					Message: openai.ChatCompletionMessage{Content: `{"spam":false,"reason":"openai","confidence":20}`},
				}},
			}, nil
		},
	}
	geminiMock := &mocks.GeminiClientMock{
		GenerateContentFunc: func(ctx context.Context, model string, contents []*genai.Content,
			config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			return &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{
					Content: &genai.Content{Parts: []*genai.Part{{Text: `{"spam":false,"reason":"gemini","confidence":20}`}}},
				}},
			}, nil
		},
	}

	d.WithOpenAIChecker(openAIMock, OpenAIConfig{Model: "gpt4"})
	d.WithGeminiChecker(geminiMock, GeminiConfig{Model: "gemma-4-31b-it", CheckShortMessages: true})

	spam, cr := d.Check(spamcheck.Request{Msg: "hi"})
	assert.False(t, spam)
	assert.Empty(t, openAIMock.CreateChatCompletionCalls())
	assert.Len(t, geminiMock.GenerateContentCalls(), 1)

	checkNames := make([]string, 0, len(cr))
	for _, check := range cr {
		checkNames = append(checkNames, check.Name)
	}
	assert.Contains(t, checkNames, "message length")
	assert.Contains(t, checkNames, "gemini")
	assert.NotContains(t, checkNames, "openai")
}

func BenchmarkTokenize(b *testing.B) {
	d := &Detector{
		excludedTokens: map[string]struct{}{"the": {}, "and": {}, "or": {}, "but": {}, "in": {}, "on": {}, "at": {}, "to": {}},
	}

	tests := []struct {
		name string
		text string
	}{
		{
			name: "Short_NoExcluded",
			text: "hello world test message",
		},
		{
			name: "Short_WithExcluded",
			text: "the quick brown fox and the lazy dog",
		},
		{
			name: "Medium_Mixed",
			text: strings.Repeat("hello world and test message with some excluded tokens ", 10),
		},
		{
			name: "Long_MixedWithPunct",
			text: strings.Repeat("hello, world! test? message. with!! some... excluded tokens!!! ", 50),
		},
		{
			name: "WithEmoji",
			text: "hello 👋 world 🌍 test 🧪 message 📝 with emoji 😊",
		},
		{
			name: "RealWorldSample",
			text: "🔥 EXCLUSIVE OFFER! Don't miss out on this amazing deal. Buy now and get 50% OFF! Limited time offer. Click here: http://example.com #deal #shopping #discount",
		},
	}

	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = d.tokenize(tc.text)
			}
		})
	}
}
