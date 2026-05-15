package slowpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeFastOnly(t *testing.T) {
	fast := DetectionResult{
		Spam:  true,
		Score: 0.8,
		Signals: []DetectionSignal{
			{Name: "stopword", Score: 0.9, Matched: true, Reason: "crypto"},
		},
	}
	merged := MergeResults(fast, nil)
	assert.True(t, merged.Spam)
	assert.InDelta(t, 0.8, merged.Score, 1e-9)
	assert.Len(t, merged.Signals, 1)
}

func TestMergeSlowOverridesToSpam(t *testing.T) {
	fast := DetectionResult{
		Spam:  false,
		Score: 0.3,
		Signals: []DetectionSignal{
			{Name: "stopword", Score: 0.3, Matched: false},
		},
	}
	slow := &SlowPathResult{
		Spam:       true,
		Confidence: 90,
		Reason:     "LLM detected scam",
		Providers:  []string{"openai"},
		Final:      true,
		Signals: []ProviderResult{
			{Spam: true, Confidence: 90, Provider: "openai"},
		},
	}
	merged := MergeResults(fast, slow)
	assert.True(t, merged.Spam)
	assert.GreaterOrEqual(t, merged.Score, fast.Score)
	assert.Len(t, merged.Signals, 2)
	assert.Equal(t, "openai", merged.Signals[1].Name)
}

func TestMergeSlowConfirmsHam(t *testing.T) {
	fast := DetectionResult{
		Spam:  false,
		Score: 0.1,
		Signals: []DetectionSignal{
			{Name: "stopword", Score: 0.1, Matched: false},
		},
	}
	slow := &SlowPathResult{
		Spam:       false,
		Confidence: 95,
		Reason:     "clean",
		Providers:  []string{"gemini"},
		Signals: []ProviderResult{
			{Spam: false, Confidence: 95, Provider: "gemini"},
		},
	}
	merged := MergeResults(fast, slow)
	assert.False(t, merged.Spam)
	assert.Len(t, merged.Signals, 2)
}

func TestMergeSlowSkipped(t *testing.T) {
	fast := DetectionResult{
		Spam:  false,
		Score: 0.2,
		Signals: []DetectionSignal{
			{Name: "meta", Score: 0.2, Matched: false},
		},
	}
	slow := &SlowPathResult{Skipped: true}
	merged := MergeResults(fast, slow)
	assert.False(t, merged.Spam)
	assert.Len(t, merged.Signals, 1)
}

func TestMergeSlowOverridesFastSpamToHam(t *testing.T) {
	fast := DetectionResult{
		Spam:  true,
		Score: 0.6,
		Signals: []DetectionSignal{
			{Name: "stopword", Score: 0.6, Matched: true, Reason: "borderline"},
		},
	}
	slow := &SlowPathResult{
		Spam:       false,
		Final:      true,
		Confidence: 85,
		Reason:     "legitimate",
		Providers:  []string{"openai"},
		Signals: []ProviderResult{
			{Spam: false, Confidence: 85, Provider: "openai"},
		},
	}
	merged := MergeResults(fast, slow)
	assert.False(t, merged.Spam)
	assert.Len(t, merged.Signals, 2)
}

func TestShouldEscalateForceLLM(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{ForceLLM: true})
	assert.True(t, ok)
	assert.Equal(t, EscalationForceLLM, reason)
}

func TestShouldEscalateUserReport(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{UserReport: true})
	assert.True(t, ok)
	assert.Equal(t, EscalationUserReport, reason)
}

func TestShouldEscalateHighRiskPolicy(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{HighRiskPolicy: true})
	assert.True(t, ok)
	assert.Equal(t, EscalationHighRiskPolicy, reason)
}

func TestShouldEscalateImageContent(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{HasImages: true})
	assert.True(t, ok)
	assert.Equal(t, EscalationImageContent, reason)
}

func TestShouldEscalateAmbiguous(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{AmbiguousScore: true})
	assert.True(t, ok)
	assert.Equal(t, EscalationAmbiguousFast, reason)
}

func TestShouldEscalateNone(t *testing.T) {
	ok, reason := ShouldEscalate(EscalationCheck{})
	assert.False(t, ok)
	assert.Empty(t, reason)
}
