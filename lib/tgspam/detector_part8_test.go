package tgspam

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"github.com/umputun/tg-spam/lib/tgspam/mocks"
	"io"
	"strings"
	"testing"
	"time"
)

func BenchmarkLoadSamples(b *testing.B) {
	makeReader := func(lines []string) io.Reader {
		return strings.NewReader(strings.Join(lines, "\n"))
	}

	tests := []struct {
		name     string
		spam     []string
		ham      []string
		excluded []string
	}{
		{
			name:     "Small",
			spam:     []string{"spam message 1", "buy now spam 2", "spam offer 3"},
			ham:      []string{"hello world", "normal message", "how are you"},
			excluded: []string{"the", "and", "or"},
		},
		{
			name:     "Medium",
			spam:     []string{"spam message 1", "buy now spam 2", "spam offer 3", "urgent offer", "free money"},
			ham:      []string{"hello world", "normal message", "how are you", "meeting tomorrow", "project update"},
			excluded: []string{"the", "and", "or", "but", "in", "on", "at"},
		},
		{
			name: "Large_RealWorld",

			spam: []string{
				"Здравствуйте   Мы занимаемая новым видом заработка в интернете   Наша сфера даст вам опыт, знания",
				"У кого нет карты карты Тинькофф? Можете оформить по моей ссылке и получите 500р от меня",
				"😀😀😀 Для тeх ктo ищeт дoпoлнительный доход предлагаю перспективный и прибыльный зaрaботok",
			},
			ham: []string{
				"When is our next meeting?",
				"Here's the project update you requested",
				"Thanks for the feedback, I'll review it",
			},
			excluded: []string{"the", "and", "or", "but", "in", "on", "at", "to", "for", "with"},
		},
	}

	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			d := NewDetector(Config{})

			b.ResetTimer()
			b.ReportAllocs()
			for range b.N {

				spamReader := makeReader(tc.spam)
				hamReader := makeReader(tc.ham)
				exclReader := makeReader(tc.excluded)

				_, err := d.LoadSamples(exclReader, []io.Reader{spamReader}, []io.Reader{hamReader})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestDetector_ClassifierNaNHandling(t *testing.T) {
	d := NewDetector(Config{})

	t.Run("empty classifier no NaN", func(t *testing.T) {

		spam, cr := d.Check(spamcheck.Request{Msg: "test message with no training data"})
		assert.False(t, spam)

		for _, r := range cr {
			if r.Name == "classifier" {
				t.Error("classifier check should not be performed with empty training data")
			}
		}
	})

	t.Run("minimal training data no NaN", func(t *testing.T) {

		spamReader := strings.NewReader("spam")
		hamReader := strings.NewReader("ham")
		exclReader := strings.NewReader("")
		_, err := d.LoadSamples(exclReader, []io.Reader{spamReader}, []io.Reader{hamReader})
		require.NoError(t, err)

		spam, cr := d.Check(spamcheck.Request{Msg: "completely unknown tokens that were never seen"})
		assert.False(t, spam)

		// find classifier result
		var classifierResult *spamcheck.Response
		for i := range cr {
			if cr[i].Name == "classifier" {
				classifierResult = &cr[i]
				break
			}
		}

		require.NotNil(t, classifierResult, "classifier result should be present")
		assert.NotContains(t, classifierResult.Details, "NaN", "probability should not be NaN")

		assert.Regexp(t, `probability of (spam|ham): \d+\.\d+%`, classifierResult.Details)
	})

	t.Run("edge case tokens no NaN", func(t *testing.T) {

		testCases := []string{
			"",
			"   ",
			"a",
			"aa",
			"😀😀😀😀😀😀😀😀😀😀",
			"\n\n\n\n",
			strings.Repeat("a", 10000),
		}

		for _, tc := range testCases {
			spam, cr := d.Check(spamcheck.Request{Msg: tc})

			for _, r := range cr {
				if r.Name == "classifier" {
					assert.NotContains(t, r.Details, "NaN", "probability should not be NaN for input: %q", tc)
				}
			}
			_ = spam
		}
	})
}

func TestDetector_ShortMessageApproval(t *testing.T) {
	t.Run("short messages don't count towards approval", func(t *testing.T) {
		d := NewDetector(Config{MinMsgLen: 10, FirstMessagesCount: 3, FirstMessageOnly: true, MaxAllowedEmoji: -1})

		for range 3 {
			spam, cr := d.Check(spamcheck.Request{Msg: "hi", UserID: "123"})
			assert.False(t, spam)
			assert.NotEmpty(t, cr)

			found := false
			for _, r := range cr {
				if r.Name == "message length" {
					assert.Equal(t, "too short", r.Details)
					found = true
					break
				}
			}
			assert.True(t, found, "message length check not found")
		}

		assert.False(t, d.IsApprovedUser("123"))

		_, err := d.LoadStopWords(strings.NewReader("spam"))
		require.NoError(t, err)
		spam, cr := d.Check(spamcheck.Request{Msg: "spam", UserID: "123"})

		assert.True(t, spam)
		found := false
		for _, r := range cr {
			if r.Name == "stopword" && r.Spam {
				found = true
				break
			}
		}
		assert.True(t, found, "stopword check should detect spam")

		spam, cr = d.Check(spamcheck.Request{Msg: "this is a spam message", UserID: "123"})
		assert.True(t, spam)

		found = false
		for _, r := range cr {
			if r.Name == "stopword" && r.Spam {
				found = true
				break
			}
		}
		assert.True(t, found, "stopword check not found")
	})

	t.Run("normal messages count towards approval", func(t *testing.T) {
		d := NewDetector(Config{MinMsgLen: 10, FirstMessagesCount: 3, FirstMessageOnly: true, MaxAllowedEmoji: -1})

		for range 2 {
			spam, _ := d.Check(spamcheck.Request{Msg: "this is a normal message", UserID: "456"})
			assert.False(t, spam)
		}

		assert.False(t, d.IsApprovedUser("456"))

		spam, _ := d.Check(spamcheck.Request{Msg: "another normal message here", UserID: "456"})
		assert.False(t, spam)

		assert.False(t, d.IsApprovedUser("456"))

		spam, cr := d.Check(spamcheck.Request{Msg: "fourth normal message", UserID: "456"})
		assert.False(t, spam)
		assert.Equal(t, "pre-approved", cr[0].Name)

		d.lock.RLock()
		actualCount := d.approvedUsers["456"].Count
		d.lock.RUnlock()
		assert.Equal(t, 3, actualCount)

		assert.False(t, d.IsApprovedUser("456"))
	})

	t.Run("mix of short and normal messages", func(t *testing.T) {
		d := NewDetector(Config{MinMsgLen: 10, FirstMessagesCount: 3, FirstMessageOnly: true, MaxAllowedEmoji: -1})

		spam, _ := d.Check(spamcheck.Request{Msg: "hi", UserID: "789"})
		assert.False(t, spam)
		assert.False(t, d.IsApprovedUser("789"))

		spam, _ = d.Check(spamcheck.Request{Msg: "this is a normal message", UserID: "789"})
		assert.False(t, spam)
		assert.False(t, d.IsApprovedUser("789"))

		spam, _ = d.Check(spamcheck.Request{Msg: "ok", UserID: "789"})
		assert.False(t, spam)
		assert.False(t, d.IsApprovedUser("789"))

		spam, _ = d.Check(spamcheck.Request{Msg: "another normal message here", UserID: "789"})
		assert.False(t, spam)
		assert.False(t, d.IsApprovedUser("789"))

		spam, _ = d.Check(spamcheck.Request{Msg: "yes", UserID: "789"})
		assert.False(t, spam)
		assert.False(t, d.IsApprovedUser("789"))

		spam, _ = d.Check(spamcheck.Request{Msg: "third normal message finally", UserID: "789"})
		assert.False(t, spam)

		assert.False(t, d.IsApprovedUser("789"))
	})

	t.Run("short messages with storage", func(t *testing.T) {
		mockUserStore := &mocks.UserStorageMock{
			ReadFunc: func(context.Context) ([]approved.UserInfo, error) {
				return []approved.UserInfo{}, nil
			},
			WriteFunc:  func(_ context.Context, au approved.UserInfo) error { return nil },
			DeleteFunc: func(_ context.Context, id string) error { return nil },
		}

		d := NewDetector(Config{MinMsgLen: 10, FirstMessagesCount: 2, FirstMessageOnly: true, MaxAllowedEmoji: -1})
		_, err := d.WithUserStorage(mockUserStore)
		require.NoError(t, err)

		d.Check(spamcheck.Request{Msg: "hi", UserID: "111"})
		d.Check(spamcheck.Request{Msg: "ok", UserID: "111"})

		assert.Empty(t, mockUserStore.WriteCalls())

		d.Check(spamcheck.Request{Msg: "normal message one", UserID: "111"})
		assert.Len(t, mockUserStore.WriteCalls(), 1)

		d.Check(spamcheck.Request{Msg: "normal message two", UserID: "111"})
		assert.Len(t, mockUserStore.WriteCalls(), 2)

		assert.False(t, d.IsApprovedUser("111"))

		d.Check(spamcheck.Request{Msg: "normal message three", UserID: "111"})

		assert.Len(t, mockUserStore.WriteCalls(), 2)

		assert.False(t, d.IsApprovedUser("111"))
	})
}

func TestDetector_CheckStopWords_Ukrainian(t *testing.T) {
	d := NewDetector(Config{})
	lr, err := d.LoadStopWords(bytes.NewBufferString("пишіть у приват\nдеталі в особисті"))
	require.NoError(t, err)
	assert.Equal(t, LoadResult{StopWords: 2}, lr)

	spam, checks := d.Check(spamcheck.Request{Msg: "Якщо цікаво, пишіть у приват, все поясню"})
	assert.True(t, spam)
	resp := findResponseByName(checks, "stopword")
	require.NotNil(t, resp)
	assert.True(t, resp.Spam)
	assert.Contains(t, resp.Details, "пишіть у приват")

	spam, checks = d.Check(spamcheck.Request{Msg: "Якщо цікаво, деталі в особисті, відповім пізніше"})
	assert.True(t, spam)
	resp = findResponseByName(checks, "stopword")
	require.NotNil(t, resp)
	assert.True(t, resp.Spam)
	assert.Contains(t, resp.Details, "деталі в особисті")
}

// helper function to find response by name
func findResponseByName(responses []spamcheck.Response, name string) *spamcheck.Response {
	for _, r := range responses {
		if r.Name == name {
			return &r
		}
	}
	return nil
}

func TestDetector_UpdateConfig(t *testing.T) {
	d := NewDetector(Config{
		MinMsgLen:           50,
		SimilarityThreshold: 0.5,
		FirstMessageOnly:    true,
		FirstMessagesCount:  1,
		DuplicateDetection: struct {
			Threshold int
			Window    time.Duration
		}{Threshold: 2, Window: time.Hour},
	})

	assert.Equal(t, 50, d.MinMsgLen)
	assert.InDelta(t, 0.5, d.SimilarityThreshold, 1e-9)
	assert.NotNil(t, d.duplicateDetector)

	d.UpdateConfig(Config{
		MinMsgLen:           100,
		SimilarityThreshold: 0.8,
		MinSpamProbability:  60,
		FirstMessageOnly:    true,
		FirstMessagesCount:  1,
		DuplicateDetection: struct {
			Threshold int
			Window    time.Duration
		}{Threshold: 5, Window: 2 * time.Hour},
	})

	assert.Equal(t, 100, d.MinMsgLen)
	assert.InDelta(t, 0.8, d.SimilarityThreshold, 1e-9)
	assert.InDelta(t, float64(60), d.MinSpamProbability, 1e-9)
	assert.NotNil(t, d.duplicateDetector)

	_, cr := d.Check(spamcheck.Request{Msg: "test", UserID: "123"})
	assert.NotNil(t, findResponseByName(cr, "duplicate"))

	d.UpdateConfig(Config{
		MinMsgLen:           100,
		SimilarityThreshold: 0.8,
		FirstMessageOnly:    true,
		FirstMessagesCount:  1,
		DuplicateDetection: struct {
			Threshold int
			Window    time.Duration
		}{Threshold: 0, Window: 0},
	})

	assert.Nil(t, d.duplicateDetector)
}

func TestDetector_ReplaceMetaChecks(t *testing.T) {
	d := NewDetector(Config{MinMsgLen: 50})

	d.WithMetaChecks(LinksCheck(3), ImagesCheck(50))
	_, cr := d.Check(spamcheck.Request{Msg: "test http://example.com", UserID: "1"})
	assert.NotNil(t, findResponseByName(cr, "links"))
	assert.NotNil(t, findResponseByName(cr, "images"))

	d.ReplaceMetaChecks(MentionsCheck(5))
	_, cr = d.Check(spamcheck.Request{Msg: "test http://example.com", UserID: "2"})
	assert.Nil(t, findResponseByName(cr, "links"))
	assert.Nil(t, findResponseByName(cr, "images"))
	assert.NotNil(t, findResponseByName(cr, "mentions"))
}
