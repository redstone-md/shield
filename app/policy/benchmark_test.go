package policy

import (
	"testing"

	"github.com/redstone-md/shield/lib/spamcheck"
)

func BenchmarkEngine_Decide(b *testing.B) {
	e := NewEngine(BalancedProfile())
	sig := []spamcheck.Response{
		{Name: "stop-word", Spam: true, RuleID: "stop-word", Score: 1.0, Weight: 1.0},
	}
	input := PolicyInput{Signals: sig, StrikeCount: 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Decide(input)
	}
}

func BenchmarkEngine_Decide_WithEscalation(b *testing.B) {
	e := NewEngine(BalancedProfile())
	sig := []spamcheck.Response{
		{Name: "stop-word", Spam: true, RuleID: "stop-word"},
	}
	input := PolicyInput{Signals: sig, StrikeCount: 3}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Decide(input)
	}
}

func BenchmarkEngine_Decide_WithShadow(b *testing.B) {
	e := NewEngineWithShadow(BalancedProfile(), StrictProfile())
	sig := []spamcheck.Response{
		{Name: "stop-word", Spam: true, RuleID: "stop-word"},
	}
	input := PolicyInput{Signals: sig, StrikeCount: 2}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Decide(input)
	}
}
