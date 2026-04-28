package tgspam

import (
	"fmt"
	"math"
	"strings"

	"github.com/umputun/tg-spam/lib/spamcheck"
)

type ScoringEngine struct {
	threshold float64
	signals   []spamcheck.Response
}

func NewScoringEngine(threshold float64) *ScoringEngine {
	if threshold <= 0 {
		threshold = 1.0
	}
	return &ScoringEngine{threshold: threshold}
}

func (e *ScoringEngine) AddSignal(r spamcheck.Response) {
	e.signals = append(e.signals, r)
}

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
		var names []string
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
// Probabilistic checks pass their own score. NormalizedText is truncated to 64 runes.
func withScoring(r spamcheck.Response, score float64, normText string) spamcheck.Response {
	r.RuleID = r.Name
	r.Score = score
	if r.Weight == 0 && r.Spam {
		r.Weight = 1.0
	}
	if len(normText) > 64 {
		r.NormalizedText = string([]rune(normText)[:64])
	} else {
		r.NormalizedText = normText
	}
	return r
}

// truncateRunes truncates text to maxRunes runes.
func truncateRunes(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}
