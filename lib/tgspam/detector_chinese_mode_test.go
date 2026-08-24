package tgspam

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redstone-md/shield/lib/spamcheck"
)

func TestDetector_CheckChineseMode(t *testing.T) {
	t.Run("short chinese message banned despite being below min length", func(t *testing.T) {
		d := NewDetector(Config{MinMsgLen: 50, MaxAllowedEmoji: -1, ChineseMode: true})
		spam, cr := d.Check(spamcheck.Request{Msg: "有活", UserID: "1001"})
		assert.True(t, spam)
		require.Len(t, cr, 2)
		assert.Equal(t, "chinese-chars", cr[0].Name)
		assert.True(t, cr[0].Spam)
		assert.Equal(t, "2/2 cjk", cr[0].Details)
		assert.Equal(t, "message length", cr[1].Name)
	})

	t.Run("mixed cyrillic and chinese banned when no ratio is set", func(t *testing.T) {
		d := NewDetector(Config{MinMsgLen: 50, MaxAllowedEmoji: -1, ChineseMode: true})
		spam, cr := d.Check(spamcheck.Request{Msg: "привет, есть работа 有活"})
		assert.True(t, spam)
		require.NotEmpty(t, cr)
		assert.Equal(t, "chinese-chars", cr[0].Name)
		assert.True(t, cr[0].Spam)
	})

	t.Run("ratio mode ignores mostly non-chinese messages", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, ChineseMode: true, ChineseCharRatio: 0.5})
		spam, cr := d.Check(spamcheck.Request{Msg: "привет, есть работа 有活"})
		assert.False(t, spam)
		require.Len(t, cr, 1)
		assert.False(t, cr[0].Spam)
		assert.Equal(t, "2/18 cjk", cr[0].Details)
	})

	t.Run("ratio mode bans dominated messages including boundary", func(t *testing.T) {
		tests := []struct {
			name string
			msg  string
		}{
			{"pure chinese", "有活"},
			{"exact half", "аа 有活"},
			{"mostly chinese", "工作 加微信 谢谢"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				d := NewDetector(Config{MaxAllowedEmoji: -1, ChineseMode: true, ChineseCharRatio: 0.5})
				spam, cr := d.Check(spamcheck.Request{Msg: tt.msg})
				assert.True(t, spam)
				require.NotEmpty(t, cr)
				assert.True(t, cr[0].Spam)
			})
		}
	})

	t.Run("plain english message passes", func(t *testing.T) {
		d := NewDetector(Config{MinMsgLen: 50, MaxAllowedEmoji: -1, ChineseMode: true})
		spam, cr := d.Check(spamcheck.Request{Msg: "hello there friend, this is a normal message"})
		assert.False(t, spam)
		require.Len(t, cr, 2)
		assert.Equal(t, "chinese-chars", cr[0].Name)
		assert.False(t, cr[0].Spam)
	})

	t.Run("japanese kana only passes", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1, ChineseMode: true})
		spam, cr := d.Check(spamcheck.Request{Msg: "ひらがな カタカナ"})
		assert.False(t, spam)
		require.Len(t, cr, 1)
		assert.False(t, cr[0].Spam)
	})

	t.Run("no letters passes without division by zero", func(t *testing.T) {
		for _, msg := range []string{"", "!@# $%", "👍😂"} {
			d := NewDetector(Config{MaxAllowedEmoji: -1, ChineseMode: true})
			spam, cr := d.Check(spamcheck.Request{Msg: msg})
			assert.False(t, spam)
			require.Len(t, cr, 1)
			assert.Equal(t, "chinese-chars", cr[0].Name)
			assert.False(t, cr[0].Spam)
			assert.Equal(t, "0/0 cjk", cr[0].Details)
		}
	})

	t.Run("disabled leaves short messages unchecked", func(t *testing.T) {
		d := NewDetector(Config{MinMsgLen: 50, MaxAllowedEmoji: -1})
		spam, cr := d.Check(spamcheck.Request{Msg: "有活", UserID: "1001"})
		assert.False(t, spam)
		require.Len(t, cr, 1)
		assert.Equal(t, "message length", cr[0].Name)
	})

	t.Run("approved users are not checked", func(t *testing.T) {
		d := NewDetector(Config{MinMsgLen: 50, MaxAllowedEmoji: -1, ChineseMode: true, FirstMessagesCount: 1})
		_, _ = d.Check(spamcheck.Request{Msg: "regular first message to get approved, this text is long enough to pass the min length check", UserID: "2002"})
		spam, cr := d.Check(spamcheck.Request{Msg: "有活", UserID: "2002"})
		assert.False(t, spam)
		require.Len(t, cr, 1)
		assert.Equal(t, "pre-approved", cr[0].Name)
	})

	t.Run("update config toggles the mode", func(t *testing.T) {
		d := NewDetector(Config{MaxAllowedEmoji: -1})
		spam, _ := d.Check(spamcheck.Request{Msg: "有活"})
		assert.False(t, spam)

		d.UpdateConfig(Config{ChineseMode: true, MaxAllowedEmoji: -1})
		spam, cr := d.Check(spamcheck.Request{Msg: "有活"})
		assert.True(t, spam)
		require.NotEmpty(t, cr)
		assert.True(t, cr[0].Spam)

		d.UpdateConfig(Config{MaxAllowedEmoji: -1})
		spam, _ = d.Check(spamcheck.Request{Msg: "有活"})
		assert.False(t, spam)
	})
}
