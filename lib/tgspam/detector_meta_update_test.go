package tgspam

import (
	"fmt"
	"github.com/redstone-md/shield/lib/spamcheck"
	"github.com/redstone-md/shield/lib/tgspam/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
)

func TestDetector_CheckWithMeta(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1})
	d.WithMetaChecks(LinksCheck(1), ImagesCheck(0), LinkOnlyCheck())

	t.Run("no links, no images", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "Hello, how are you?"})
		assert.False(t, spam)
		require.Len(t, cr, 3)
		assert.Equal(t, "links", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "links 0/1", cr[0].Details)
		assert.Equal(t, "images", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "text or no images", cr[1].Details)
		assert.Equal(t, "link-only", cr[2].Name)
		assert.False(t, cr[2].Spam)
	})

	t.Run("one link, no images", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "Hello, how are you? https://google.com"})
		assert.False(t, spam)
		require.Len(t, cr, 3)
		assert.Equal(t, "links", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "links 1/1", cr[0].Details)
		assert.Equal(t, "images", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "text or no images", cr[1].Details)
		assert.Equal(t, "link-only", cr[2].Name)
		assert.False(t, cr[2].Spam)
	})

	t.Run("one link, no text", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: " https://google.com"})
		assert.True(t, spam)
		t.Logf("%+v", cr)
		require.Len(t, cr, 3)
		assert.Equal(t, "links", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "links 1/1", cr[0].Details)
		assert.Equal(t, "images", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "text or no images", cr[1].Details)
		assert.Equal(t, "link-only", cr[2].Name)
		assert.True(t, cr[2].Spam)
	})

	t.Run("one link, one image with some text", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "Hello, how are you? https://google.com", Meta: spamcheck.MetaData{Images: 1}})
		assert.False(t, spam)
		t.Logf("%+v", cr)
		require.Len(t, cr, 3)
		assert.Equal(t, "links", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "links 1/1", cr[0].Details)
		assert.Equal(t, "images", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "text or no images", cr[1].Details)
		assert.Equal(t, "link-only", cr[2].Name)
		assert.False(t, cr[2].Spam)
		assert.Equal(t, "message contains text", cr[2].Details)
	})

	t.Run("two links, one image with text", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "Hello, how are you? https://google.com https://google.com", Meta: spamcheck.MetaData{Images: 1}})
		assert.True(t, spam)
		t.Logf("%+v", cr)
		require.Len(t, cr, 3)
		assert.Equal(t, "links", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "too many links 2/1", cr[0].Details)
		assert.Equal(t, "images", cr[1].Name)
		assert.False(t, cr[1].Spam)
		assert.Equal(t, "text or no images", cr[1].Details)
		assert.Equal(t, "link-only", cr[2].Name)
		assert.False(t, cr[2].Spam)
		assert.Equal(t, "message contains text", cr[2].Details)
	})

	t.Run("no links, two images, no text", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "", Meta: spamcheck.MetaData{Images: 2}})
		assert.True(t, spam)
		t.Logf("%+v", cr)
		require.Len(t, cr, 3)
		assert.Equal(t, "links", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "links 0/1", cr[0].Details)
		assert.Equal(t, "images", cr[1].Name)
		assert.True(t, cr[1].Spam)
		assert.Equal(t, "image without text", cr[1].Details)
		assert.Equal(t, "link-only", cr[2].Name)
		assert.False(t, cr[2].Spam)
		assert.Equal(t, "empty message", cr[2].Details)
	})
}

func TestDetector_CheckMultiLang(t *testing.T) {
	d := NewDetector(Config{MultiLangWords: 2, MaxAllowedEmoji: -1})
	tests := []struct {
		name  string
		input string
		count int
		spam  bool
	}{
		{"No MultiLang", "Hello, world!\n 12345-980! _", 0, false},
		{"One MultiLang", "Hi thereе", 1, false},
		{"Two MultiLang", "Gооd moфning", 2, true},
		{"WithCyrillic no MultiLang", "Привет  мир", 0, false},
		{"WithCyrillic two MultiLang", "Привеt  мip", 2, true},
		{"WithCyrillic and special symbols", "Привет мир -@#$%^&*(_", 0, false},
		{"WithCyrillic real example 1", "Ищем заинтeрeсoвaнных в зaрaбoткe нa кpиптoвaлютe. Всeгдa хотeли пoпpoбовать сeбя в этом, нo нe знали с чeго нaчaть? Тогдa вaм кo мнe 3aнимаемся aрбuтражeм, зaрабaтывaeм на paзницe курсов с минимaльныmи pискaми 💲Рынok oчень волатильный и нам это выгoднo, пo этoмe пишиte @vitalgoescra и зapaбaтывaйтe сo мнoй ", 31, true},
		{"WithCyrillic real example 2", "В поuске паpтнеров, заuнтересованных в пассuвном дoходе с затpатой мuнuмум лuчного временu. Все деталu в лс", 10, true},
		{"WithCyrillic real example 3", "Всем привет, есть простая шабашка, подойдет любому. Даю 15 тысяч. Накину на проезд, сигареты, обед. ", 0, false},
		{"WithCyrillic and i", "Привет  мiр", 0, false},
		{"strange with cyrillic", "𝐇айди и𝐇𝐓и𝐦𝐇ы𝐞 ф𝐨𝐓𝐤и лю𝐛𝐨й д𝐞𝐁𝐲ш𝐤и ч𝐞𝐩𝐞𝟓 𝐛𝐨𝐓а", 7, true},
		{"coptic capital leter", "✔️ⲠⲢⲞⳜⲈЙ-ⲖЮⳜⲨЮ-ⲆⲈⲂⲨⲰⲔⲨ...\n\nⲎⲀЙⲆⳘ ⲤⲔⲢЫⲦⲈ ⲂⳘⲆⲞⲤЫ-ⲪⲞⲦⲞⳠⲔⳘ ⳘⲎⲦⳘⲘⲎⲞⲄⲞ-ⲬⲀⲢⲀⲔⲦⲈⲢⲀ..\n@INTIM0CHKI110DE\n\n", 5, true},
		{"mix with gothic, cyrillic and greek", "𐌿РОВЕРЬ ЛЮБУЮ НА НАЛИЧИЕ ПОШЛЫХ ΦΟͲΟ-ΒͶДξΟ, 🍑НАБЕРИ В Т𐌲 𐌿ОИСКЕ СЛOВО: 30GRL", 5, true},
		{"Mixed Latin and numbers", "Meta-Llama-3.1-8B-Instruct-IQ4_XS Meta-Llama-3.1-8B-Instruct-Q3_K_L Meta-Llama-3.1-8B-Instruct-Q4_K_M", 0, false},
		{"Mixed Latin, numbers, and Cyrillic", "Meta-Llama-3.1-8B-Instruct-IQ4_XS Meta-Llama-3.1-8B-Instruct-Q3_K_L коллеги, подскажите пожалуйста", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spam, cr := d.Check(spamcheck.Request{Msg: tt.input})
			assert.Equal(t, tt.spam, spam)
			require.Len(t, cr, 1)
			assert.Equal(t, "multi-lingual", cr[0].Name)
			assert.Equal(t, tt.spam, cr[0].Spam)
			assert.Equal(t, fmt.Sprintf("%d/2", tt.count), cr[0].Details)
		})
	}

	d.MultiLangWords = 0
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spam, _ := d.Check(spamcheck.Request{Msg: tt.input})
			assert.False(t, spam)
		})
	}
}

func TestDetector_CheckWithAbnormalSpacing(t *testing.T) {
	d := NewDetector(Config{MaxAllowedEmoji: -1})
	d.AbnormalSpacing.Enabled = true
	d.AbnormalSpacing.ShortWordLen = 3
	d.AbnormalSpacing.ShortWordRatioThreshold = 0.7
	d.AbnormalSpacing.SpaceRatioThreshold = 0.3
	d.AbnormalSpacing.MinWordsCount = 6

	tests := []struct {
		name     string
		input    string
		expected bool
		details  string
	}{
		{
			name:     "normal text",
			input:    "This is a normal message with regular spacing between words",
			expected: false,
			details:  "normal (ratio: 0.18, short: 20%)",
		},
		{
			name:     "another normal text",
			input:    "For structured content, like code or tables, the ratio can vary significantly based on formatting and indentation",
			expected: false,
			details:  "normal (ratio: 0.17, short: 35%)",
		},
		{
			name: "normal text, cyrillic",
			input: "Если продолжать этот парад благородного безумия, я бы предложил базу имеющихся примеров спама" +
				" сконвертировать в эмбеддинги и проверять через них после байеса, но до gpt4. Эмбеддинги должны быть" +
				" устойчивы к разделенным словам и даже переформулировкам, при этом они гораздо дешевле большой модели.",
			expected: false,
			details:  "normal (ratio: 0.17, short: 26%)",
		},
		{
			name:     "suspicious spacing cyrillic",
			input:    "СРО ЧНО ЭТО КАСА ЕТСЯ КАЖ ДОГО В ЭТ ОЙ ГРУ ППЕ",
			expected: true,
			details:  "abnormal (ratio: 0.31, short: 75%)",
		},
		{
			name:     "very suspicious spacing cyrillic",
			input:    "н а р к о т и к о в, и н в е с т и ц и й",
			expected: true,
			details:  "abnormal (ratio: 0.95, short: 100%)",
		},
		{
			name:     "mixed normal and suspicious",
			input:    "Hello there к а ж д о г о в э т о й г р у п п е",
			expected: true,
			details:  "abnormal (ratio: 0.68, short: 90%)",
		},
		{
			name:     "short spaced text",
			input:    "a b c d",
			expected: false,
			details:  "too short",
		},
		{
			name:     "text with some extra spaces",
			input:    "This   is    a    message    with    some    extra    spaces",
			expected: true,
			details:  "abnormal (ratio: 0.82, short: 25%)",
		},
		{
			name: "real spam example",
			input: "СРО ЧНО ЭТО КАСА ЕТСЯ КАЖ ДОГО В ЭТ ОЙ ГРУ ППЕ  Строго 20+ В данный момент про ходит обуч ение " +
				"для нови чков  Сразу говорю - без н а р к о т и к о в, и н в е с т и ц и й и прочей еру нды.  Быстрый старт, " +
				"при быль вы полу чите уже в пе рвый день раб   Все легально  Для раб оты нужен смарт фон и всего 1 час твоего " +
				"врем ени в день  Дове дём вас за ру чку до при были Не бер ём н и к а к и х о п л а т от вас, мы ра60таем " +
				"на % (вы зараба тываете и  только потом делитесь с нами) Кому действ ительно инте ресно " +
				"пишите в и я обязат ельно тебе отвечу",
			expected: true,
			details:  "abnormal (ratio: 0.36, short: 63%)",
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
			details:  "too short",
		},
		{
			name:     "low number of words",
			input:    "с впн всё работает",
			expected: false,
			details:  "too few words (4)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spam, resp := d.Check(spamcheck.Request{Msg: tt.input})
			t.Logf("Response: %+v", resp)
			require.Len(t, resp, 1)
			assert.Equal(t, tt.expected, spam, "spam detection mismatch")
			assert.Equal(t, tt.details, resp[0].Details, "details mismatch")
		})
	}

	t.Run("disabled short word threshold", func(t *testing.T) {
		d.AbnormalSpacing.ShortWordLen = 0
		spam, resp := d.Check(spamcheck.Request{Msg: "СРО ЧНО ЭТО КАС АЕТ СЯ КАЖ ДОГО В ЭТ ОЙ ГРУ ППЕ something else"})
		t.Logf("Response: %+v", resp)
		assert.False(t, spam)
		assert.Equal(t, "normal (ratio: 0.29, short: 0%)", resp[0].Details)
	})

	t.Run("enabled short word threshold", func(t *testing.T) {
		d.AbnormalSpacing.ShortWordLen = 3
		spam, resp := d.Check(spamcheck.Request{Msg: "СРО ЧНО ЭТО КАС АЕТ СЯ КАЖ ДОГО В ЭТ ОЙ ГРУ ППЕ something else"})
		t.Logf("Response: %+v", resp)
		assert.True(t, spam)
		assert.Equal(t, "abnormal (ratio: 0.29, short: 80%)", resp[0].Details)
	})

}

func TestDetector_UpdateSpam(t *testing.T) {
	upd := &mocks.SampleUpdaterMock{
		AppendFunc: func(msg string) error {
			return nil
		},
	}

	d := NewDetector(Config{MaxAllowedEmoji: -1})
	d.WithSpamUpdater(upd)

	spamSamples := strings.NewReader("win free iPhone\nlottery prize xyz")
	hamsSamples := strings.NewReader("hello world\nhow are you\nhave a good day")
	lr, err := d.LoadSamples(strings.NewReader("xyz"), []io.Reader{spamSamples}, []io.Reader{hamsSamples})
	require.NoError(t, err)
	assert.Equal(t, LoadResult{ExcludedTokens: 1, SpamSamples: 2, HamSamples: 3}, lr)
	d.tokenizedSpam = nil
	assert.Equal(t, 5, d.classifier.nAllDocument)
	exp := map[string]map[spamClass]int{"win": {"spam": 1}, "free": {"spam": 1}, "iphone": {"spam": 1}, "lottery": {"spam": 1},
		"prize": {"spam": 1}, "hello": {"ham": 1}, "world": {"ham": 1}, "how": {"ham": 1}, "are": {"ham": 1}, "you": {"ham": 1},
		"have": {"ham": 1}, "good": {"ham": 1}, "day": {"ham": 1}}
	assert.Equal(t, exp, d.classifier.learningResults)

	msg := "another good world one iphone user writes good things day"
	t.Run("initially a little bit ham", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: msg})
		assert.False(t, spam)
		require.Len(t, cr, 1)
		assert.Equal(t, "classifier", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "probability of ham: 59.97%", cr[0].Details)
	})

	err = d.UpdateSpam("another user writes")
	require.NoError(t, err)
	assert.Equal(t, 6, d.classifier.nAllDocument)
	assert.Len(t, upd.AppendCalls(), 1)

	t.Run("after update mostly spam", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: msg})
		assert.True(t, spam)
		require.Len(t, cr, 1)
		assert.Equal(t, "classifier", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "probability of spam: 66.67%", cr[0].Details)
	})
}

func TestDetector_UpdateHam(t *testing.T) {
	upd := &mocks.SampleUpdaterMock{
		AppendFunc: func(msg string) error {
			return nil
		},
	}

	d := NewDetector(Config{MaxAllowedEmoji: -1})
	d.WithHamUpdater(upd)

	spamSamples := strings.NewReader("win free iPhone\nlottery prize xyz")
	hamsSamples := strings.NewReader("hello world\nhow are you\nhave a good day")
	lr, err := d.LoadSamples(strings.NewReader("xyz"), []io.Reader{spamSamples}, []io.Reader{hamsSamples})
	require.NoError(t, err)
	assert.Equal(t, LoadResult{ExcludedTokens: 1, SpamSamples: 2, HamSamples: 3}, lr)
	d.tokenizedSpam = nil
	assert.Equal(t, 5, d.classifier.nAllDocument)
	exp := map[string]map[spamClass]int{"win": {"spam": 1}, "free": {"spam": 1}, "iphone": {"spam": 1}, "lottery": {"spam": 1},
		"prize": {"spam": 1}, "hello": {"ham": 1}, "world": {"ham": 1}, "how": {"ham": 1}, "are": {"ham": 1}, "you": {"ham": 1},
		"have": {"ham": 1}, "good": {"ham": 1}, "day": {"ham": 1}}
	assert.Equal(t, exp, d.classifier.learningResults)

	msg := "another free good world one iphone user writes good things day"
	t.Run("initially a little bit spam", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: msg})
		assert.True(t, spam)
		require.Len(t, cr, 1)
		assert.Equal(t, "classifier", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "probability of spam: 60.89%", cr[0].Details)
	})

	err = d.UpdateHam("another writes things")
	require.NoError(t, err)
	assert.Equal(t, 6, d.classifier.nAllDocument)
	assert.Len(t, upd.AppendCalls(), 1)

	t.Run("after update mostly ham", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: msg})
		assert.False(t, spam)
		require.Len(t, cr, 1)
		assert.Equal(t, "classifier", cr[0].Name)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "probability of ham: 72.16%", cr[0].Details)
	})
}

func TestDetector_Reset(t *testing.T) {
	d := NewDetector(Config{})
	spamSamples := strings.NewReader("win free iPhone\nlottery prize xyz")
	hamSamples := strings.NewReader("hello world\nhow are you\nhave a good day")
	lr, err := d.LoadSamples(strings.NewReader("xyz"), []io.Reader{spamSamples}, []io.Reader{hamSamples})
	require.NoError(t, err)
	assert.Equal(t, LoadResult{ExcludedTokens: 1, SpamSamples: 2, HamSamples: 3}, lr)
	sr, err := d.LoadStopWords(strings.NewReader("в личку\nвсем привет"))
	require.NoError(t, err)
	assert.Equal(t, LoadResult{StopWords: 2}, sr)

	assert.Equal(t, 5, d.classifier.nAllDocument)
	assert.Len(t, d.tokenizedSpam, 2)
	assert.Len(t, d.excludedTokens, 1)
	assert.Len(t, d.stopWords, 2)

	d.Reset()
	assert.Equal(t, 0, d.classifier.nAllDocument)
	assert.Empty(t, d.tokenizedSpam)
	assert.Empty(t, d.excludedTokens)
	assert.Empty(t, d.stopWords)
}

func TestDetector_FirstMessagesCount(t *testing.T) {
	t.Run("first message is spam", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: 1, MinMsgLen: 5, FirstMessagesCount: 2, FirstMessageOnly: true})
		spam, _ := d.Check(spamcheck.Request{Msg: "spam, too many emojis 🤣🤣🤣", UserID: "123"})
		assert.True(t, spam)
	})
	t.Run("first messages are ham, third is spam", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: 1, MinMsgLen: 5, FirstMessagesCount: 3, FirstMessageOnly: true})

		spam, _ := d.Check(spamcheck.Request{Msg: "ham, no emojis", UserID: "123"})
		assert.False(t, spam)

		spam, _ = d.Check(spamcheck.Request{Msg: "ham, no emojis", UserID: "123"})
		assert.False(t, spam)

		spam, _ = d.Check(spamcheck.Request{Msg: "spam, too many emojis 🤣🤣🤣", UserID: "123"})
		assert.True(t, spam)
	})
	t.Run("first messages are ham, spam after approved", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: 1, MinMsgLen: 5, FirstMessagesCount: 2, FirstMessageOnly: true})

		spam, _ := d.Check(spamcheck.Request{Msg: "ham, no emojis", UserID: "123"})
		assert.False(t, spam)

		spam, _ = d.Check(spamcheck.Request{Msg: "ham, no emojis", UserID: "123"})
		assert.False(t, spam)

		spam, _ = d.Check(spamcheck.Request{Msg: "spam, too many emojis 🤣🤣🤣", UserID: "123"})
		assert.False(t, spam, "spam is not detected because user is approved")
	})
	t.Run("first messages are ham, spam after approved, FirstMessageOnly were false", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: 1, MinMsgLen: 5, FirstMessagesCount: 2, FirstMessageOnly: false})

		spam, _ := d.Check(spamcheck.Request{Msg: "ham, no emojis", UserID: "123"})
		assert.False(t, spam)

		spam, _ = d.Check(spamcheck.Request{Msg: "ham, no emojis", UserID: "123"})
		assert.False(t, spam)

		spam, _ = d.Check(spamcheck.Request{Msg: "spam, too many emojis 🤣🤣🤣", UserID: "123"})
		assert.False(t, spam)
	})
}
