package tgspam

import (
	"bytes"
	"fmt"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
	"time"
)

func TestDetector_CheckWithShort(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: 1, MinMsgLen: 150})
	lr, err := d.LoadStopWords(bytes.NewBufferString("в личку\nвсем привет"))
	require.NoError(t, err)
	assert.Equal(t, LoadResult{StopWords: 2}, lr)

	t.Run("short message without spam", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "good message"})
		assert.False(t, spam)
		require.Len(t, cr, 3, cr)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "not found", cr[0].Details)
		assert.Equal(t, "emoji", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "0/1", cr[1].Details)
		assert.Equal(t, "message length", cr[2].Name)
		assert.False(t, cr[2].Spam)
		assert.Equal(t, "too short", cr[2].Details)
	})

	t.Run("short message with stopwords", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "Hello, please send me a message в личку"})
		assert.True(t, spam)
		require.Len(t, cr, 3, cr)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "в личку", cr[0].Details)
		assert.Equal(t, "emoji", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "0/1", cr[1].Details)
		assert.Equal(t, "message length", cr[2].Name)
		assert.False(t, cr[2].Spam)
		assert.Equal(t, "too short", cr[2].Details)
	})

	t.Run("short message with emojis", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "Hello 😁🐶🍕"})
		assert.True(t, spam)
		require.Len(t, cr, 3, cr)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "not found", cr[0].Details)
		assert.Equal(t, "emoji", cr[1].Name)
		assert.True(t, cr[1].Spam)
		assert.Equal(t, "3/1", cr[1].Details)
		assert.Equal(t, "message length", cr[2].Name)
		assert.False(t, cr[2].Spam)
		assert.Equal(t, "too short", cr[2].Details)
	})

	t.Run("short message with emojis and stop words", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "Hello 😁🐶🍕в личку"})
		assert.True(t, spam)
		require.Len(t, cr, 3, cr)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "в личку", cr[0].Details)
		assert.Equal(t, "emoji", cr[1].Name)
		assert.True(t, cr[1].Spam)
		assert.Equal(t, "3/1", cr[1].Details)
		assert.Equal(t, "message length", cr[2].Name)
		assert.False(t, cr[2].Spam)
		assert.Equal(t, "too short", cr[2].Details)
	})

	t.Run("stopword with extra spaces and different case", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: 1, MinMsgLen: 150})
		lr, err := d.LoadStopWords(bytes.NewBufferString("дам денег"))
		require.NoError(t, err)
		assert.Equal(t, LoadResult{StopWords: 1}, lr)

		spam, cr := d.Check(spamcheck.Request{Msg: "Дам  денег"})
		assert.True(t, spam, "should detect stopword with extra spaces")
		require.Len(t, cr, 3, cr)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.True(t, cr[0].Spam, "should match 'дам денег' even with extra space in 'Дам  денег'")
		assert.Equal(t, "дам денег", cr[0].Details)

		spam, cr = d.Check(spamcheck.Request{Msg: "ДАМ ДЕНЕГ"})
		assert.True(t, spam, "should detect stopword with uppercase")
		require.Len(t, cr, 3, cr)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.True(t, cr[0].Spam, "should match 'дам денег' even with uppercase 'ДАМ ДЕНЕГ'")
		assert.Equal(t, "дам денег", cr[0].Details)

		spam, cr = d.Check(spamcheck.Request{Msg: "дам    денег"})
		assert.True(t, spam, "should detect stopword with multiple spaces")
		require.Len(t, cr, 3, cr)
		assert.Equal(t, "stopword", cr[0].Name)
		assert.True(t, cr[0].Spam, "should match 'дам денег' even with multiple spaces")
		assert.Equal(t, "дам денег", cr[0].Details)
	})

	t.Run("short message skips classifier and similarity", func(t *testing.T) {

		d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 50, SimilarityThreshold: 0.5})

		spamSamples := strings.NewReader("buy cheap viagra now\nclick here for free money\nwin lottery prize")
		hamSamples := strings.NewReader("hello world\nhow are you\nhave a good day")

		lr, err := d.LoadSamples(strings.NewReader(""), []io.Reader{spamSamples}, []io.Reader{hamSamples})
		require.NoError(t, err)
		assert.Positive(t, lr.SpamSamples)
		assert.Positive(t, lr.HamSamples)

		spam, cr := d.Check(spamcheck.Request{Msg: "hi"})
		assert.False(t, spam)

		require.Len(t, cr, 1)
		assert.Equal(t, "message length", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "too short", cr[0].Details)

		for _, r := range cr {
			assert.NotEqual(t, "classifier", r.Name, "classifier should not run for short messages")
			assert.NotEqual(t, "similarity", r.Name, "similarity should not run for short messages")
		}

		spam2, cr2 := d.Check(spamcheck.Request{Msg: "this is a much longer message that should trigger all checks including classifier and similarity"})
		assert.False(t, spam2)

		hasClassifier := false
		hasSimilarity := false
		for _, r := range cr2 {
			if r.Name == "classifier" {
				hasClassifier = true
			}
			if r.Name == "similarity" {
				hasSimilarity = true
			}
		}
		assert.True(t, hasClassifier, "classifier should run for long messages")
		assert.True(t, hasSimilarity, "similarity should run for long messages")
	})
}

func TestDetector_CheckStopWords(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1})
	lr, err := d.LoadStopWords(bytes.NewBufferString("в личку\nвсеМ прИвет\nspambot\n12345"))
	require.NoError(t, err)
	assert.Equal(t, LoadResult{StopWords: 4}, lr)

	tests := []struct {
		name     string
		message  string
		username string
		userID   string
		expected bool
		details  string
	}{
		{
			name:     "Stop word present in message",
			message:  "Hello, please send me a message в личкУ",
			username: "user1",
			userID:   "987654321",
			expected: true,
			details:  "в личку",
		},
		{
			name:     "Stop word present with emoji",
			message:  "👋Всем привет\nИщу амбициозного человека к се6е в команду\nКто в поисках дополнительного заработка или хочет попробовать себя в новой  сфере деятельности! 👨🏻\u200d💻\nПишите в лс✍️",
			username: "user1",
			userID:   "987654321",
			expected: true,
			details:  "всем привет",
		},
		{
			name:     "No stop word present",
			message:  "Hello, how are you?",
			username: "user1",
			userID:   "987654321",
			expected: false,
			details:  "not found",
		},
		{
			name:     "Case insensitive stop word present",
			message:  "Hello, please send me a message В ЛИЧКУ",
			username: "user1",
			userID:   "987654321",
			expected: true,
			details:  "в личку",
		},
		{
			name:     "Stop word in username",
			message:  "Hello, how are you?",
			username: "spambot_seller",
			userID:   "987654321",
			expected: true,
			details:  "spambot",
		},
		{
			name:     "Stop word in username case insensitive",
			message:  "Hello, how are you?",
			username: "SpAmBoT_seller",
			userID:   "987654321",
			expected: true,
			details:  "spambot",
		},
		{
			name:     "Stop word in user ID",
			message:  "Hello, how are you?",
			username: "normal_user",
			userID:   "12345_account",
			expected: true,
			details:  "12345",
		},
		{
			name:     "No stop word in anything",
			message:  "Regular message",
			username: "normal_user",
			userID:   "987654321",
			expected: false,
			details:  "not found",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spam, cr := d.Check(spamcheck.Request{Msg: test.message, UserName: test.username, UserID: test.userID})
			assert.Equal(t, test.expected, spam)
			require.Len(t, cr, 1)
			assert.Equal(t, "stopword", cr[0].Name)
			assert.Equal(t, test.details, cr[0].Details)
			t.Logf("%+v", cr[0].Details)
		})
	}
}

func TestDetector_CheckStopWordsExactMatch(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1})

	lr, err := d.LoadStopWords(bytes.NewBufferString("=buy now\nhello\n=spammer123"))
	require.NoError(t, err)
	assert.Equal(t, LoadResult{StopWords: 3}, lr)

	tests := []struct {
		name     string
		message  string
		username string
		userID   string
		expected bool
		details  string
	}{
		{
			name:     "exact match - message equals stop word",
			message:  "buy now",
			expected: true,
			details:  "buy now",
		},
		{
			name:     "exact match - message equals stop word case insensitive",
			message:  "BUY NOW",
			expected: true,
			details:  "buy now",
		},
		{
			name:     "exact match - message contains stop word but not exact",
			message:  "please buy now today",
			expected: false,
			details:  "not found",
		},
		{
			name:     "exact match - message with extra spaces normalized",
			message:  "buy  now",
			expected: true,
			details:  "buy now",
		},
		{
			name:     "substring match - message contains stop word",
			message:  "hello world",
			expected: true,
			details:  "hello",
		},
		{
			name:     "substring match - stop word anywhere in message",
			message:  "say hello to everyone",
			expected: true,
			details:  "hello",
		},
		{
			name:     "exact match username - exact match",
			message:  "normal message",
			username: "spammer123",
			expected: true,
			details:  "spammer123",
		},
		{
			name:     "exact match username - not exact",
			message:  "normal message",
			username: "spammer123_bot",
			expected: false,
			details:  "not found",
		},
		{
			name:     "exact match user id - exact match",
			message:  "normal message",
			userID:   "spammer123",
			expected: true,
			details:  "spammer123",
		},
		{
			name:     "no match",
			message:  "regular text",
			expected: false,
			details:  "not found",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spam, cr := d.Check(spamcheck.Request{Msg: test.message, UserName: test.username, UserID: test.userID})
			assert.Equal(t, test.expected, spam, "spam detection mismatch")
			require.Len(t, cr, 1)
			assert.Equal(t, "stopword", cr[0].Name)
			assert.Equal(t, test.details, cr[0].Details)
		})
	}
}

func TestDetector_CheckStopWordsEdgeCases(t *testing.T) {
	t.Run("equals-only stop word should not match anything", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1})
		_, err := d.LoadStopWords(bytes.NewBufferString("=\nhello"))
		require.NoError(t, err)

		spam, cr := d.Check(spamcheck.Request{Msg: ""})
		assert.False(t, spam)
		assert.Equal(t, "not found", cr[0].Details)

		spam, cr = d.Check(spamcheck.Request{Msg: "   "})
		assert.False(t, spam)
		assert.Equal(t, "not found", cr[0].Details)

		spam, cr = d.Check(spamcheck.Request{Msg: "say hello"})
		assert.True(t, spam)
		assert.Equal(t, "hello", cr[0].Details)
	})

	t.Run("double equals prefix matches literal equals", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1})
		_, err := d.LoadStopWords(bytes.NewBufferString("==test"))
		require.NoError(t, err)

		spam, cr := d.Check(spamcheck.Request{Msg: "=test"})
		assert.True(t, spam)
		assert.Equal(t, "=test", cr[0].Details)

		spam, cr = d.Check(spamcheck.Request{Msg: "test"})
		assert.False(t, spam)
		assert.Equal(t, "not found", cr[0].Details)
	})
}

func TestDetector_CheckEmojis(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: 2})
	tests := []struct {
		name  string
		input string
		count int
		spam  bool
	}{
		{"NoEmoji", "Hello, world!", 0, false},
		{"OneEmoji", "Hi there 👋", 1, false},
		{"TwoEmojis", "Good morning 🌞🌻", 2, false},
		{"Mixed", "👨‍👩‍👧‍👦 Family emoji", 1, false},
		{"EmojiSequences", "🏳️‍🌈 Rainbow flag", 1, false},
		{"TextAfterEmoji", "😊 Have a nice day!", 1, false},
		{"OnlyEmojis", "😁🐶🍕", 3, true},
		{"WithCyrillic", "Привет 🌞 🍕 мир! 👋", 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spam, cr := d.Check(spamcheck.Request{Msg: tt.input})
			assert.Equal(t, tt.spam, spam)
			require.Len(t, cr, 1)
			assert.Equal(t, "emoji", cr[0].Name)
			assert.Equal(t, tt.spam, cr[0].Spam)
			assert.Equal(t, fmt.Sprintf("%d/2", tt.count), cr[0].Details)
		})
	}
}

func TestDetector_CheckDuplicates(t *testing.T) {
	d := NewDetector(Config{
		DuplicateDetection: struct {
			Threshold int
			Window    time.Duration
		}{
			Threshold: 3,
			Window:    time.Hour,
		},
	})

	spam, cr := d.Check(spamcheck.Request{Msg: "test message", UserID: "123"})
	assert.False(t, spam)
	dupResp := findResponseByName(cr, "duplicate")
	require.NotNil(t, dupResp)
	assert.False(t, dupResp.Spam)

	spam, cr = d.Check(spamcheck.Request{Msg: "test message", UserID: "123"})
	assert.False(t, spam)
	dupResp = findResponseByName(cr, "duplicate")
	require.NotNil(t, dupResp)
	assert.False(t, dupResp.Spam)

	spam, cr = d.Check(spamcheck.Request{Msg: "test message", UserID: "123"})
	assert.True(t, spam)
	dupResp = findResponseByName(cr, "duplicate")
	require.NotNil(t, dupResp)
	assert.True(t, dupResp.Spam)
	assert.Contains(t, dupResp.Details, "3 times")

	spam, _ = d.Check(spamcheck.Request{Msg: "different message", UserID: "123"})
	assert.False(t, spam)

	spam, _ = d.Check(spamcheck.Request{Msg: "test message", UserID: "456"})
	assert.False(t, spam)
}

func TestDetector_CheckDuplicatesDisabled(t *testing.T) {

	d := NewDetector(Config{
		DuplicateDetection: struct {
			Threshold int
			Window    time.Duration
		}{
			Threshold: 0,
			Window:    time.Hour,
		},
	})

	for range 5 {
		spam, cr := d.Check(spamcheck.Request{Msg: "test", UserID: "123"})
		assert.False(t, spam)
		dupResp := findResponseByName(cr, "duplicate")
		if dupResp != nil {
			assert.False(t, dupResp.Spam)
			assert.Equal(t, "check disabled", dupResp.Details)
		}
	}
}
