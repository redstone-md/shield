package tgspam

import (
	"bytes"
	"strings"
	"testing"

	"github.com/redstone-md/shield/lib/spamcheck"
)

func BenchmarkDetector_Check(b *testing.B) {
	spamWords := bytes.NewBufferString("buy now\nfree money\nclick here\ncasino\ncrypto")
	d := NewDetector(Config{MaxAllowedEmoji: 5, MinMsgLen: 10})
	d.LoadStopWords(spamWords)

	benchCases := []struct {
		name string
		msg  string
	}{
		{"Ham_Short", "hello world"},
		{"Ham_Medium", "I wanted to ask about the meeting schedule for tomorrow, can you help?"},
		{"Ham_Long", strings.Repeat("This is a regular message about project updates and team coordination. ", 5)},
		{"Spam_Stopword", "Buy now! Free money waiting for you! Click here to claim!"},
		{"Spam_Emoji", "Hello 😁🐶🍕🎉🔥💯🎊🎈🎀"},
		{"Mixed_Confusables", "Buy nоw frее mоney"}, // cyrillic о/е mixed in
	}

	for _, bc := range benchCases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			req := spamcheck.Request{Msg: bc.msg, UserID: "bench-user"}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				d.Check(req)
			}
		})
	}
}

func BenchmarkDetector_Check_Scoring(b *testing.B) {
	spamWords := bytes.NewBufferString("buy now\nfree money\nclick here")
	msgs := []struct {
		name string
		msg  string
	}{
		{"Ham", "regular message about work"},
		{"Spam", "Buy now! Free money! Click here!"},
	}

	b.Run("BooleanOR", func(b *testing.B) {
		d := NewDetector(Config{MaxAllowedEmoji: 5, MinMsgLen: 10})
		d.LoadStopWords(spamWords)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			d.Check(spamcheck.Request{Msg: msgs[i%2].msg, UserID: "bench-user"})
		}
	})

	b.Run("ScoringEngine", func(b *testing.B) {
		d := NewDetector(Config{MaxAllowedEmoji: 5, MinMsgLen: 10, ScoringThreshold: 1.0})
		d.LoadStopWords(spamWords)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			d.Check(spamcheck.Request{Msg: msgs[i%2].msg, UserID: "bench-user"})
		}
	})
}

func BenchmarkDetector_Check_Burst(b *testing.B) {
	spamWords := bytes.NewBufferString("buy now\nfree money\nclick here\ncasino")
	d := NewDetector(Config{MaxAllowedEmoji: 5, MinMsgLen: 10})
	d.LoadStopWords(spamWords)

	msgs := []string{
		"hello, how are you doing today?",
		"Buy now! Free money waiting for you!",
		"the project deadline is next friday",
		"Click here to claim your prize now",
		"can we schedule a meeting for tomorrow",
		"Casino bonus! Free money! Click here!",
		"thanks for the update, looks good",
		"Buy now crypto free money casino click",
		"sending the revised document shortly",
		"Free money buy now click here casino",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := spamcheck.Request{Msg: msgs[i%len(msgs)], UserID: "bench-user"}
		d.Check(req)
	}
}

func BenchmarkScoringEngine(b *testing.B) {
	signals := []spamcheck.Response{
		{Name: "stopword", Spam: true, Score: 1.0, Weight: 1.0, RuleID: "stopword"},
		{Name: "emoji", Spam: false, Score: 0, Weight: 0, RuleID: "emoji"},
		{Name: "links", Spam: true, Score: 1.0, Weight: 1.0, RuleID: "meta-links"},
		{Name: "similarity", Spam: true, Score: 0.85, Weight: 0.5, RuleID: "similarity"},
		{Name: "classifier", Spam: false, Score: 0.3, Weight: 0.8, RuleID: "classifier"},
	}

	b.Run("WeightedScore", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			se := NewScoringEngine(1.5)
			for _, s := range signals {
				se.AddSignal(s)
			}
			se.Score()
		}
	})

	b.Run("BooleanFallback", func(b *testing.B) {
		noWeightSignals := make([]spamcheck.Response, len(signals))
		for i, s := range signals {
			noWeightSignals[i] = s
			noWeightSignals[i].Weight = 0
			noWeightSignals[i].Score = 0
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			se := NewScoringEngine(1.0)
			for _, s := range noWeightSignals {
				se.AddSignal(s)
			}
			se.Score()
		}
	})
}
