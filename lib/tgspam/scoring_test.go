package tgspam

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/umputun/tg-spam/lib/spamcheck"
)

func TestScoringEngine_SingleSignal(t *testing.T) {
	se := NewScoringEngine(1.0)
	se.AddSignal(spamcheck.Response{Name: "stopword", Spam: true, Score: 1.0, Weight: 1.0, RuleID: "stopword"})
	rs := se.Score()
	assert.True(t, rs.Decision)
	assert.InDelta(t, 1.0, rs.Total, 1e-9)
	assert.Len(t, rs.Signals, 1)
}

func TestScoringEngine_MultipleSignals(t *testing.T) {
	se := NewScoringEngine(1.0)
	se.AddSignal(spamcheck.Response{Name: "stopword", Spam: true, Score: 0.8, Weight: 1.0, RuleID: "stopword"})
	se.AddSignal(spamcheck.Response{Name: "links", Spam: true, Score: 1.0, Weight: 0.5, RuleID: "meta-links"})
	se.AddSignal(spamcheck.Response{Name: "similarity", Spam: false, Score: 0.3, Weight: 1.0, RuleID: "similarity"})
	rs := se.Score()
	// total = 0.8*1.0 + 1.0*0.5 = 1.3 (only spam signals)
	assert.True(t, rs.Decision)
	assert.InDelta(t, 1.3, rs.Total, 0.001)
	assert.Len(t, rs.Signals, 2) // only spam signals
}

func TestScoringEngine_ThresholdBoundary(t *testing.T) {
	se := NewScoringEngine(2.0)
	se.AddSignal(spamcheck.Response{Name: "stopword", Spam: true, Score: 1.0, Weight: 1.0})
	assert.False(t, se.Score().Decision) // 1.0 < 2.0

	se.AddSignal(spamcheck.Response{Name: "links", Spam: true, Score: 1.0, Weight: 1.5})
	rs := se.Score()
	assert.True(t, rs.Decision)
	assert.InDelta(t, 2.5, rs.Total, 0.001)
}

func TestScoringEngine_ZeroWeightFallback(t *testing.T) {
	se := NewScoringEngine(1.0)
	se.AddSignal(spamcheck.Response{Name: "stopword", Spam: true, Score: 1.0, Weight: 0})
	se.AddSignal(spamcheck.Response{Name: "links", Spam: true, Score: 1.0, Weight: 0})
	rs := se.Score()
	// no weighted signals → fallback to boolean OR → true
	assert.True(t, rs.Decision)
	assert.Contains(t, rs.Reason, "boolean")
}

func TestScoringEngine_NoSpamSignals(t *testing.T) {
	se := NewScoringEngine(1.0)
	se.AddSignal(spamcheck.Response{Name: "similarity", Spam: false, Score: 0.5, Weight: 1.0})
	rs := se.Score()
	assert.False(t, rs.Decision)
	assert.InDelta(t, 0.0, rs.Total, 0.001)
}

func TestScoringEngine_Empty(t *testing.T) {
	se := NewScoringEngine(1.0)
	rs := se.Score()
	assert.False(t, rs.Decision)
	assert.InDelta(t, 0.0, rs.Total, 0.001)
	assert.Empty(t, rs.Signals)
}

func TestScoringEngine_ReasonIncludesSignals(t *testing.T) {
	se := NewScoringEngine(1.0)
	se.AddSignal(spamcheck.Response{Name: "stopword", Spam: true, Score: 1.0, Weight: 1.0, RuleID: "stopword"})
	se.AddSignal(spamcheck.Response{Name: "links", Spam: true, Score: 1.0, Weight: 0.8, RuleID: "meta-links"})
	rs := se.Score()
	assert.Contains(t, rs.Reason, "stopword")
	assert.Contains(t, rs.Reason, "links")
}

func TestScoringEngine_DefaultWeightIsOne(t *testing.T) {
	se := NewScoringEngine(1.0)
	// Weight=0 means "not set", default treated as 1.0 in aggregation
	se.AddSignal(spamcheck.Response{Name: "stopword", Spam: true, Score: 1.0, Weight: 0})
	rs := se.Score()
	// fallback to boolean OR since no weighted signals
	assert.True(t, rs.Decision)
}
