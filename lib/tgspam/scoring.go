package tgspam

import (
	"fmt"
	"math"
	"strings"

	"github.com/umputun/tg-spam/lib/spamcheck"
)

// ScoringEngine aggregates weighted spam signals against a threshold and renders the verdict.
type ScoringEngine struct {
	threshold float64
	signals   []spamcheck.Response
}

// NewScoringEngine returns a ScoringEngine that flags spam when total weighted score reaches threshold (default 1.0).
func NewScoringEngine(threshold float64) *ScoringEngine {
	if threshold <= 0 {
		threshold = 1.0
	}
	return &ScoringEngine{threshold: threshold}
}

// AddSignal records a check response that the engine will weigh in Score.
func (e *ScoringEngine) AddSignal(r spamcheck.Response) {
	e.signals = append(e.signals, r)
}

// Score computes the aggregate RiskScore, falling back to boolean OR when no signal carries a weight.
func (e *ScoringEngine) Score() spamcheck.RiskScore {
	if e.hasWeightedSignals() {
		return e.weightedScore()
	}
	return e.booleanFallback()
}

func (e *ScoringEngine) hasWeightedSignals() bool {
	for _, s := range e.signals {
		if s.Weight > 0 && s.Spam {
			return true
		}
	}
	return false
}

func (e *ScoringEngine) weightedScore() spamcheck.RiskScore {
	var total float64
	var contributing []spamcheck.Response
	var reasons []string

	for _, s := range e.signals {
		if !s.Spam || s.Weight <= 0 {
			continue
		}
		weight := s.Weight
		if weight == 0 {
			weight = 1.0
		}
		score := s.Score
		if score == 0 {
			score = 1.0
		}
		contribution := weight * score
		total += contribution
		contributing = append(contributing, s)
		reasons = append(reasons, fmt.Sprintf("%s(%.2f*%.2f=%.2f)", s.Name, weight, score, contribution))
	}

	decision := total >= e.threshold
	reason := fmt.Sprintf("total=%.2f threshold=%.2f decision=%v signals=[%s]",
		total, e.threshold, decision, strings.Join(reasons, ", "))

	return spamcheck.RiskScore{
		Total:    math.Round(total*10000) / 10000,
		Signals:  contributing,
		Decision: decision,
		Reason:   reason,
	}
}

func (e *ScoringEngine) booleanFallback() spamcheck.RiskScore {
	var contributing []spamcheck.Response
	for _, s := range e.signals {
		if s.Spam {
			contributing = append(contributing, s)
		}
	}
	decision := len(contributing) > 0
	reason := "boolean-or"
	if decision {
		names := make([]string, 0, len(contributing))
		for _, s := range contributing {
			names = append(names, s.Name)
		}
		reason = fmt.Sprintf("boolean-or: [%s]", strings.Join(names, ", "))
	}
	return spamcheck.RiskScore{
		Total:    0,
		Signals:  contributing,
		Decision: decision,
		Reason:   reason,
	}
}

// scoreSignals determines spam via ScoringEngine if threshold > 0, else boolean OR.
func (d *Detector) scoreSignals(cr []spamcheck.Response, boolOR func([]spamcheck.Response) bool) bool {
	if d.ScoringThreshold > 0 {
		se := NewScoringEngine(d.ScoringThreshold)
		for _, r := range cr {
			se.AddSignal(r)
		}
		return se.Score().Decision
	}
	return boolOR(cr)
}

// withScoring populates scoring fields on a Response. Deterministic checks get Score=1.0, Weight=1.0.
// NormalizedText is truncated to 64 runes.
func withScoring(r spamcheck.Response, normText string) spamcheck.Response {
	r.RuleID = r.Name
	r.Score = 1.0
	if r.Weight == 0 && r.Spam {
		r.Weight = 1.0
	}
	r.NormalizedText = truncateRunes(normText)
	return r
}

// truncateRunes truncates text to 64 runes.
func truncateRunes(text string) string {
	const maxRunes = 64
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}
