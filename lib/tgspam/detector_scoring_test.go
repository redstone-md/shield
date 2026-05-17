package tgspam

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/lib/spamcheck"
)

func TestDetector_ScoringEndToEnd(t *testing.T) {
	d := NewDetector(Config{
		MaxAllowedEmoji:    3,
		MinMsgLen:          20,
		ScoringThreshold:   1.0,
		FirstMessageOnly:   true,
		FirstMessagesCount: 1,
	})

	lr, err := d.LoadStopWords(bytes.NewBufferString("buy now\nfree money\nclick here"))
	require.NoError(t, err)
	assert.Equal(t, LoadResult{StopWords: 3}, lr)

	t.Run("ham message scoring off still works", func(t *testing.T) {
		dNoScore := NewDetector(Config{MaxAllowedEmoji: 3, MinMsgLen: 20})
		dNoScore.LoadStopWords(bytes.NewBufferString("buy now\nfree money"))
		spam, cr := dNoScore.Check(spamcheck.Request{Msg: "hello, how are you doing today?", UserID: "u1"})
		assert.False(t, spam)
		for _, r := range cr {
			if r.Name == "stopword" {
				assert.False(t, r.Spam)
			}
		}
	})

	t.Run("spam stopword with scoring fields", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "Buy now! Free money waiting for you!", UserID: "u2"})
		assert.True(t, spam)

		var stopwordResp *spamcheck.Response
		for i := range cr {
			if cr[i].Name == "stopword" && cr[i].Spam {
				stopwordResp = &cr[i]
				break
			}
		}
		require.NotNil(t, stopwordResp)
		assert.Equal(t, "stopword", stopwordResp.RuleID)
		assert.InDelta(t, 1.0, stopwordResp.Score, 1e-9)
		assert.InDelta(t, 1.0, stopwordResp.Weight, 1e-9)
		assert.NotEmpty(t, stopwordResp.NormalizedText)
	})

	t.Run("spam emoji with scoring fields", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "Hello 😁🐶🍕🎉🔥💯🎊🎈🎀", UserID: "u3"})
		assert.True(t, spam)

		var emojiResp *spamcheck.Response
		for i := range cr {
			if cr[i].Name == "emoji" && cr[i].Spam {
				emojiResp = &cr[i]
				break
			}
		}
		require.NotNil(t, emojiResp)
		assert.Equal(t, "emoji", emojiResp.RuleID)
		assert.InDelta(t, 1.0, emojiResp.Score, 1e-9)
		assert.InDelta(t, 1.0, emojiResp.Weight, 1e-9)
	})

	t.Run("ham with scoring engine returns not spam", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{
			Msg:    "I wanted to ask about the project timeline and deliverables",
			UserID: "u4",
		})
		assert.False(t, spam)
		for _, r := range cr {
			if r.Spam {
				t.Fatalf("unexpected spam from %q: %s", r.Name, r.Details)
			}
		}
	})

	t.Run("scoring fields populated on all spam checks", func(t *testing.T) {
		spam, cr := d.Check(spamcheck.Request{Msg: "Buy now! Free money waiting for you click here!", UserID: "u5"})
		assert.True(t, spam)
		for _, r := range cr {
			if r.Spam {
				assert.NotEmpty(t, r.Name, "spam result must have Name")
				assert.Equal(t, r.Name, r.RuleID, "RuleID must match Name")
				assert.InDelta(t, 1.0, r.Score, 1e-9, "deterministic check score=1.0")
				assert.InDelta(t, 1.0, r.Weight, 1e-9, "default weight=1.0")
			}
		}
	})
}

func TestDetector_ScoringBackwardCompat(t *testing.T) {
	spamWords := bytes.NewBufferString("buy now\nfree money\nclick here")
	msg := "Buy now! Free money waiting for you click here!"
	req := spamcheck.Request{Msg: msg, UserID: "compat-user"}

	dBool := NewDetector(Config{MaxAllowedEmoji: 5, MinMsgLen: 10, FirstMessageOnly: true, FirstMessagesCount: 1})
	dBool.LoadStopWords(spamWords)
	spamBool, crBool := dBool.Check(req)

	spamWords2 := bytes.NewBufferString("buy now\nfree money\nclick here")
	dScore := NewDetector(Config{MaxAllowedEmoji: 5, MinMsgLen: 10, ScoringThreshold: 1.0, FirstMessageOnly: true, FirstMessagesCount: 1})
	dScore.LoadStopWords(spamWords2)
	spamScore, crScore := dScore.Check(req)

	assert.Equal(t, spamBool, spamScore, "scoring engine must agree with boolean OR on clear spam")

	boolSpamCount := 0
	for _, r := range crBool {
		if r.Spam {
			boolSpamCount++
		}
	}
	scoreSpamCount := 0
	for _, r := range crScore {
		if r.Spam {
			scoreSpamCount++
		}
	}
	assert.Equal(t, boolSpamCount, scoreSpamCount, "same number of spam signals")
}

func TestDetector_ScoringThresholdBoundary(t *testing.T) {
	// use MinMsgLen=5 and messages longer than 5 runes so they reach scoreSignals (line 292).
	// short messages (< MinMsgLen) early-return at line 266 using boolean OR, bypassing scoring engine.
	d := NewDetector(Config{
		MaxAllowedEmoji:    -1,
		MinMsgLen:          5,
		ScoringThreshold:   2.0,
		FirstMessageOnly:   true,
		FirstMessagesCount: 1,
	})
	d.LoadStopWords(bytes.NewBufferString("buy now\nfree money"))
	d.WithMetaChecks(LinksCheck(0))

	t.Run("single signal below threshold still spam via early return", func(t *testing.T) {
		// "Buy now everything!" is 18 runes > MinMsgLen=5, hits scoreSignals.
		// single stopword: total = 1.0 * 1.0 = 1.0 < threshold 2.0 → scoring says ham.
		spam, cr := d.Check(spamcheck.Request{Msg: "Buy now everything is on sale today!", UserID: "t1"})
		_ = cr
		assert.False(t, spam, "single stopword total=1.0 < threshold=2.0 → not spam")
	})

	t.Run("two signals exceed threshold", func(t *testing.T) {
		// stopword (1.0) + links (1.0) = 2.0 >= threshold 2.0 → spam
		spam, cr := d.Check(spamcheck.Request{Msg: "Buy now at http://spam.com for great deals", UserID: "t2"})
		assert.True(t, spam, "stopword + links = 2.0 >= threshold 2.0 → spam")
		_ = cr
	})
}

func TestDetector_ScoringEngineIntegration(t *testing.T) {
	d := NewDetector(Config{
		MaxAllowedEmoji:    3,
		MinMsgLen:          5,
		ScoringThreshold:   1.5,
		FirstMessageOnly:   true,
		FirstMessagesCount: 1,
	})
	d.LoadStopWords(bytes.NewBufferString("crypto\nviagra\ncasino"))

	t.Run("single stopword below 1.5 threshold", func(t *testing.T) {
		// "Crypto is interesting technology for the future" > 5 runes, reaches scoreSignals
		// single stopword: total = 1.0 < 1.5 → ham
		spam, _ := d.Check(spamcheck.Request{Msg: "Crypto is interesting technology for the future", UserID: "s1"})
		assert.False(t, spam, "single stopword total=1.0 < threshold=1.5 → not spam")
	})

	t.Run("stopword plus meta exceeds 1.5 threshold", func(t *testing.T) {
		dMeta := NewDetector(Config{
			MaxAllowedEmoji:    -1,
			MinMsgLen:          5,
			ScoringThreshold:   1.5,
			FirstMessageOnly:   true,
			FirstMessagesCount: 1,
		})
		dMeta.LoadStopWords(bytes.NewBufferString("crypto"))
		dMeta.WithMetaChecks(LinksCheck(0))
		// stopword (1.0) + links (1.0) = 2.0 >= 1.5 → spam
		spam, cr := dMeta.Check(spamcheck.Request{Msg: "Crypto bonus at http://spam.com today", UserID: "s1b"})
		assert.True(t, spam, "stopword + links total=2.0 >= threshold=1.5 → spam")
		_ = cr
	})

	t.Run("ham message stays ham", func(t *testing.T) {
		spam, _ := d.Check(spamcheck.Request{Msg: "The weather is nice today, want to go for a walk?", UserID: "s2"})
		assert.False(t, spam)
	})

	t.Run("all check results have names", func(t *testing.T) {
		_, cr := d.Check(spamcheck.Request{Msg: "regular message for testing the scoring engine", UserID: "s3"})
		for _, r := range cr {
			assert.NotEmpty(t, r.Name, "every check result must have a name")
		}
	})
}
