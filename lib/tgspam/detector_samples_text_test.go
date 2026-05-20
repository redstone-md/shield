package tgspam

import (
	"bytes"
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"github.com/umputun/tg-spam/lib/tgspam/mocks"
	"io"
	"sort"
	"strings"
	"testing"
)

func TestDetector_ApprovedUsers(t *testing.T) {
	mockUserStore := &mocks.UserStorageMock{
		ReadFunc: func(context.Context) ([]approved.UserInfo, error) {
			return []approved.UserInfo{{UserID: "123"}, {UserID: "456"}}, nil
		},
		WriteFunc:  func(_ context.Context, au approved.UserInfo) error { return nil },
		DeleteFunc: func(_ context.Context, id string) error { return nil },
	}

	t.Run("load with storage", func(t *testing.T) {
		mockUserStore.ResetCalls()
		d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5, FirstMessageOnly: true})
		count, err := d.WithUserStorage(mockUserStore)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		err = d.AddApprovedUser(approved.UserInfo{UserID: "999", UserName: "test"})
		require.NoError(t, err)
		res := d.ApprovedUsers()
		ids := make([]string, len(res))
		for i, u := range res {
			ids[i] = u.UserID
		}
		sort.Strings(ids)
		assert.Equal(t, []string{"123", "456", "999"}, ids)
		assert.Len(t, mockUserStore.WriteCalls(), 1)
		assert.Equal(t, "999", mockUserStore.WriteCalls()[0].Au.UserID)
		assert.Equal(t, "test", mockUserStore.WriteCalls()[0].Au.UserName)
	})

	t.Run("user not approved, spam detected", func(t *testing.T) {
		mockUserStore.ResetCalls()
		d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5, FirstMessageOnly: true})
		count, err := d.WithUserStorage(mockUserStore)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		_, err = d.LoadStopWords(strings.NewReader("spam\nbuy cryptocurrency"))
		require.NoError(t, err)
		isSpam, info := d.Check(spamcheck.Request{Msg: "Hello, how are you my friend? buy cryptocurrency now!", UserID: "999"})
		t.Logf("%+v", info)
		assert.True(t, isSpam)
		require.Len(t, info, 1)
		assert.Equal(t, "stopword", info[0].Name)
		assert.False(t, d.IsApprovedUser("999"))
		assert.Len(t, mockUserStore.ReadCalls(), 1)
		assert.Empty(t, mockUserStore.WriteCalls())
		assert.Empty(t, mockUserStore.DeleteCalls())
	})

	t.Run("user pre-approved, spam check avoided", func(t *testing.T) {
		mockUserStore.ResetCalls()
		d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5, FirstMessageOnly: true})
		count, err := d.WithUserStorage(mockUserStore)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		_, err = d.LoadStopWords(strings.NewReader("spam\nbuy cryptocurrency"))
		require.NoError(t, err)
		isSpam, info := d.Check(spamcheck.Request{Msg: "Hello, how are you my friend? buy cryptocurrency now!", UserID: "123"})
		t.Logf("%+v", info)
		assert.False(t, isSpam)
		require.Len(t, info, 1)
		assert.Equal(t, "pre-approved", info[0].Name)
		assert.True(t, d.IsApprovedUser("123"))
	})

	t.Run("user pre-approved with count, spam check avoided", func(t *testing.T) {
		mockUserStore.ResetCalls()
		d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5, FirstMessagesCount: 10})
		_, err := d.LoadStopWords(strings.NewReader("spam\nbuy cryptocurrency"))
		require.NoError(t, err)
		count, err := d.WithUserStorage(mockUserStore)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		isSpam, info := d.Check(spamcheck.Request{Msg: "Hello, how are you my friend? buy cryptocurrency now!", UserID: "123"})
		t.Logf("%+v", info)
		assert.False(t, isSpam)
		require.Len(t, info, 1)
		assert.Equal(t, "pre-approved", info[0].Name)
		assert.True(t, d.IsApprovedUser("123"))
	})

	t.Run("remove user with store", func(t *testing.T) {
		mockUserStore.ResetCalls()
		d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5, FirstMessageOnly: true})
		_, err := d.LoadStopWords(strings.NewReader("spam\nbuy cryptocurrency"))
		require.NoError(t, err)
		count, err := d.WithUserStorage(mockUserStore)
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		err = d.AddApprovedUser(approved.UserInfo{UserID: "999"})
		require.NoError(t, err)
		isSpam, info := d.Check(spamcheck.Request{Msg: "Hello, how are you my friend? buy cryptocurrency now!", UserID: "999"})
		t.Logf("%+v", info)
		assert.False(t, isSpam)
		require.Len(t, info, 1)
		assert.Equal(t, "pre-approved", info[0].Name)
		assert.True(t, d.IsApprovedUser("999"))
		assert.Len(t, mockUserStore.WriteCalls(), 1)
		assert.Equal(t, "999", mockUserStore.WriteCalls()[0].Au.UserID)

		d.RemoveApprovedUser("123")
		isSpam, info = d.Check(spamcheck.Request{Msg: "Hello, how are you my friend? buy cryptocurrency now!", UserID: "123"})
		t.Logf("%+v", info)
		assert.True(t, isSpam)
		require.Len(t, info, 1)
		assert.Equal(t, "stopword", info[0].Name)
		assert.False(t, d.IsApprovedUser("123"))
		assert.Len(t, mockUserStore.DeleteCalls(), 1)
		assert.Equal(t, "123", mockUserStore.DeleteCalls()[0].ID)
	})

	t.Run("remove user no store", func(t *testing.T) {
		mockUserStore.ResetCalls()
		d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5, FirstMessageOnly: true})
		_, err := d.LoadStopWords(strings.NewReader("spam\nbuy cryptocurrency"))
		require.NoError(t, err)

		d.AddApprovedUser(approved.UserInfo{UserID: "123"})
		isSpam, info := d.Check(spamcheck.Request{Msg: "Hello, how are you my friend? buy cryptocurrency now!", UserID: "123"})
		t.Logf("%+v", info)
		assert.False(t, isSpam)
		require.Len(t, info, 1)
		assert.Equal(t, "pre-approved", info[0].Name)
		assert.True(t, d.IsApprovedUser("123"))

		d.RemoveApprovedUser("123")
		isSpam, info = d.Check(spamcheck.Request{Msg: "Hello, how are you my friend? buy cryptocurrency now!", UserID: "123"})
		t.Logf("%+v", info)
		assert.True(t, isSpam)
		require.Len(t, info, 1)
		assert.Equal(t, "stopword", info[0].Name)
		assert.False(t, d.IsApprovedUser("123"))
		assert.Empty(t, mockUserStore.WriteCalls())
		assert.Empty(t, mockUserStore.DeleteCalls())
	})

	t.Run("add user", func(t *testing.T) {
		mockUserStore.ResetCalls()
		d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5, FirstMessageOnly: true})
		_, err := d.LoadStopWords(strings.NewReader("spam\nbuy cryptocurrency"))
		require.NoError(t, err)
		count, err := d.WithUserStorage(mockUserStore)
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		d.AddApprovedUser(approved.UserInfo{UserID: "777"})
		isSpam, info := d.Check(spamcheck.Request{Msg: "Hello, how are you my friend? buy cryptocurrency now!", UserID: "777"})
		t.Logf("%+v", info)
		assert.False(t, isSpam)
		require.Len(t, info, 1)
		assert.Equal(t, "pre-approved", info[0].Name)
		assert.True(t, d.IsApprovedUser("777"))
		assert.Len(t, mockUserStore.WriteCalls(), 1)
		assert.Equal(t, "777", mockUserStore.WriteCalls()[0].Au.UserID)
	})

	t.Run("add user, no store", func(t *testing.T) {
		mockUserStore.ResetCalls()
		d := NewDetector(Config{MaxAllowedEmoji: -1, MinMsgLen: 5, FirstMessageOnly: true})
		_, err := d.LoadStopWords(strings.NewReader("spam\nbuy cryptocurrency"))
		require.NoError(t, err)

		d.AddApprovedUser(approved.UserInfo{UserID: "777"})
		isSpam, info := d.Check(spamcheck.Request{Msg: "Hello, how are you my friend? buy cryptocurrency now!", UserID: "777"})
		t.Logf("%+v", info)
		assert.False(t, isSpam)
		require.Len(t, info, 1)
		assert.Equal(t, "pre-approved", info[0].Name)
		assert.True(t, d.IsApprovedUser("777"))
		assert.Empty(t, mockUserStore.WriteCalls())
	})

}

func TestDetector_LoadSamples(t *testing.T) {
	t.Run("basic loading", func(t *testing.T) {
		d := NewDetector(Config{})
		spamSamples := strings.NewReader("win free iPhone\nlottery prize xyz XyZ")
		hamSamples := strings.NewReader("hello world\nhow are you\nhave a good day")
		exclSamples := strings.NewReader("xyz")

		lr, err := d.LoadSamples(exclSamples, []io.Reader{spamSamples}, []io.Reader{hamSamples})

		require.NoError(t, err)
		assert.Equal(t, 1, lr.ExcludedTokens)
		assert.Equal(t, 2, lr.SpamSamples)
		assert.Equal(t, 3, lr.HamSamples)

		assert.Contains(t, d.excludedTokens, "xyz")

		assert.Len(t, d.tokenizedSpam, 2)
		assert.Contains(t, d.tokenizedSpam[0], "win")
		assert.Contains(t, d.tokenizedSpam[1], "lottery")

		assert.Equal(t, 5, d.classifier.nAllDocument)
		assert.Contains(t, d.classifier.learningResults, "win")
		assert.Contains(t, d.classifier.learningResults["win"], spamClass("spam"))
		assert.Contains(t, d.classifier.learningResults, "world")
		assert.Contains(t, d.classifier.learningResults["world"], spamClass("ham"))

		assert.NotContains(t, d.classifier.learningResults, "xyz", "excluded token should not be in learning results")
		assert.NotContains(t, d.classifier.learningResults, "XyZ", "excluded token should not be in learning results")
	})

	t.Run("empty samples", func(t *testing.T) {
		d := NewDetector(Config{})
		exclSamples := strings.NewReader("")
		spamSamples := strings.NewReader("")
		hamSamples := strings.NewReader("")

		lr, err := d.LoadSamples(exclSamples, []io.Reader{spamSamples}, []io.Reader{hamSamples})

		require.NoError(t, err)
		assert.Equal(t, 0, lr.ExcludedTokens)
		assert.Equal(t, 0, lr.SpamSamples)
		assert.Equal(t, 0, lr.HamSamples)
		assert.Equal(t, 0, d.classifier.nAllDocument)
	})

	t.Run("multiple readers", func(t *testing.T) {
		d := NewDetector(Config{})
		exclSamples := strings.NewReader("xy\n z\n the\n")
		spamSamples1 := strings.NewReader("win free iPhone")
		spamSamples2 := strings.NewReader("lottery prize xyz")
		hamsSamples1 := strings.NewReader("hello world\nhow are you\nhave a good day")
		hamsSamples2 := strings.NewReader("some other text\nwith more words")

		lr, err := d.LoadSamples(
			exclSamples,
			[]io.Reader{spamSamples1, spamSamples2},
			[]io.Reader{hamsSamples1, hamsSamples2},
		)

		require.NoError(t, err)
		assert.Equal(t, 3, lr.ExcludedTokens)

		exTkns := make([]string, 0, len(d.excludedTokens))
		for k := range d.excludedTokens {
			exTkns = append(exTkns, k)
		}
		sort.Strings(exTkns)

		assert.Equal(t, []string{"the", "xy", "z"}, exTkns)
		assert.Equal(t, 2, lr.SpamSamples)
		assert.Equal(t, 5, lr.HamSamples)
		t.Logf("Learning results: %+v", d.classifier.learningResults)
		assert.Equal(t, 7, d.classifier.nAllDocument)
		assert.Contains(t, d.classifier.learningResults["win"], spamClass("spam"))
		assert.Contains(t, d.classifier.learningResults["prize"], spamClass("spam"))
		assert.Contains(t, d.classifier.learningResults["world"], spamClass("ham"))
		assert.Contains(t, d.classifier.learningResults["some"], spamClass("ham"))
	})
}

func TestDetector_tokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]int
	}{
		{name: "empty", input: "", expected: map[string]int{}},
		{name: "no filters or cleanups", input: "hello world", expected: map[string]int{"hello": 1, "world": 1}},
		{name: "with excluded tokens", input: "hello world the she", expected: map[string]int{"hello": 1, "world": 1}},
		{name: "with short tokens", input: "hello world the she a or", expected: map[string]int{"hello": 1, "world": 1}},
		{name: "with repeated tokens", input: "hello world hello world", expected: map[string]int{"hello": 2, "world": 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Detector{excludedTokens: map[string]struct{}{"the": {}, "she": {}}}
			assert.Equal(t, tt.expected, d.tokenize(tt.input))
		})
	}
}

func TestDetector_readerIterator(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "empty", input: "", expected: []string{}},
		{name: "token per line", input: "hello\nworld", expected: []string{"hello", "world"}},
		{name: "token per line", input: "hello 123\nworld", expected: []string{"hello 123", "world"}},
		{name: "token per line with spaces", input: "hello \n world", expected: []string{"hello", "world"}},
		{name: "multiple tokens per line with", input: " hello blah\n the new world ",
			expected: []string{"hello blah", "the new world"}},
		{name: "with extra EOL", input: " hello blah\n the new world \n  ", expected: []string{"hello blah", "the new world"}},
		{name: "with empty lines", input: " hello blah\n\n  \n the new world \n  \n", expected: []string{"hello blah", "the new world"}},
	}

	d := Detector{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := d.readerIterator(bytes.NewBufferString(tt.input))
			res := make([]string, 0)
			for token := range ch {
				res = append(res, token)
			}
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestDetector_readerIteratorMultipleReaders(t *testing.T) {
	d := Detector{}
	ch := d.readerIterator(bytes.NewBufferString("hello\nworld"), bytes.NewBufferString("something, new"))
	res := make([]string, 0)
	for token := range ch {
		res = append(res, token)
	}
	sort.Strings(res)
	assert.Equal(t, []string{"hello", "something, new", "world"}, res)
}

func TestCleanText(t *testing.T) {
	d := Detector{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "English text with word joiners",
			input:    "D\u2062ude i\u2062t i\u2062s t\u2062he la\u2062st da\u2062y",
			expected: "Dude it is the last day",
		},
		{
			name:     "Russian text with word joiners",
			input:    "Р\u2062ебята д\u2062авайте б\u2062ыстрее",
			expected: "Ребята давайте быстрее",
		},
		{
			name:     "Text with pop directional formatting",
			input:    "F\u2068ast t\u2068ake i\u2068t",
			expected: "Fast take it",
		},
		{
			name:     "Mixed invisible characters",
			input:    "Hello\u200BWorld\u2062Test\u206FCase",
			expected: "HelloWorldTestCase",
		},
		{
			name:     "No invisible characters",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "Only invisible characters",
			input:    "\u200B\u2062\u206F",
			expected: "",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "URLs with invisible characters",
			input:    "https://\u2062example\u2062.com/\u2062test",
			expected: "https://example.com/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			result := d.cleanText(tt.input)
			assert.Equal(t, tt.expected, result, "failed for case: %s", tt.name)
		})
	}
}

func Test_countEmoji(t *testing.T) {
	tests := []struct {
		name  string
		input string
		count int
	}{
		{"NoEmoji", "Hello, world!", 0},
		{"OneEmoji", "Hi there 👋", 1},
		{"DupEmoji", "️‍🌈Hi 👋there 👋", 3},
		{"TwoEmojis", "Good morning 🌞🌻", 2},
		{"Mixed", "👨‍👩👦 Family emoji", 3},
		{"TextAfterEmoji", "😊 Have a nice day!", 1},
		{"OnlyEmojis", "😁🐶🍕", 3},
		{"WithCyrillic", "Привет 🌞 🍕 мир! 👋", 3},
		{"real1", "❗️НУЖЕН 1 ЧЕЛОВЕК НА ДИСТАНЦИОННУЮ РАБОТУ❗️", 2},
		{"real2", "⏰💯⚡️💯🤝🤝🤝🤝🤝🤝🤝🤝  ❗️HУЖHЫ OТВЕТCТВЕHHЫЕ ЛЮДИ❗️              🔤🔤  ➡️@yyyyy🥢" +
			"  ⚡️(OТ 2️⃣1️⃣ ВOЗРАCТ)🟢 🔋OHЛАЙH ЗАРАБOТOК 🟢 ✅COПРOВOЖДЕHИЕ🟢 ❗1-2 ЧАCА В ДЕHЬ 🟢   👍1️⃣2️⃣0️⃣0️⃣💸" +
			"➕в неделю🟢 ПИCАТЬ ✉️@xxxxxx✉️", 38},
		{"real3", "‼️СРОЧНО‼️  ‼️ЭТО КАСАЕТСЯ КАЖДОГО В ЭТОЙ ГРУППЕ‼️  🔥Строго 20+  В данный момент проходит обучение " +
			"для новичков 🔥 Сразу говорю - без наркотиков, инвестиций и прочей ерунды. 🔥 Быстрый старт, прибыль вы получите" +
			" уже в первый день работы 🔥 Все легально 🔥 Для работы нужен смартфон и всего 1 час твоего времени" +
			" в день 🔥 Доведём вас за ручку до прибыли ‼️", 11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.count, countEmoji(tt.input))
		})
	}
}

func Test_cleanEmoji(t *testing.T) {
	tests := []struct {
		name  string
		input string
		clean string
	}{
		{"NoEmoji", "Hello, world!", "Hello, world!"},
		{"OneEmoji", "Hi there 👋", "Hi there "},
		{"TwoEmojis", "Good morning 🌞🌻", "Good morning "},
		{"Mixed", "👨‍👩‍👧‍👦 Family emoji", " Family emoji"},
		{"EmojiSequences", "🏳️‍🌈 Rainbow flag", " Rainbow flag"},
		{"TextAfterEmoji", "😊 Have a nice day!", " Have a nice day!"},
		{"OnlyEmojis", "😁🐶🍕", ""},
		{"WithCyrillic", "Привет 🌞 🍕 мир! 👋", "Привет   мир! "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.clean, cleanEmoji(tt.input))
		})
	}
}

func TestDetector_RemoveHam(t *testing.T) {

	updMock := &mocks.SampleUpdaterMock{
		RemoveFunc: func(msg string) error {
			return nil
		},
		AppendFunc: func(msg string) error {
			return nil
		},
	}

	d := NewDetector(Config{})
	d.WithHamUpdater(updMock)

	hamSamples := strings.NewReader("test message\nhello world")
	lr, err := d.LoadSamples(strings.NewReader(""), nil, []io.Reader{hamSamples})
	require.NoError(t, err)
	require.Equal(t, 2, lr.HamSamples)

	require.NoError(t, d.RemoveHam("test message"))
	assert.Len(t, updMock.RemoveCalls(), 1)
	assert.Equal(t, "test message", updMock.RemoveCalls()[0].Msg)

	updMock.RemoveFunc = func(msg string) error {
		return errors.New("remove error")
	}
	err = d.RemoveHam("hello world")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remove error")
}
