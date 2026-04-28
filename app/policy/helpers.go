package policy

import "time"

func ComputePenalty(strikes int, keepRestricted bool, firstStrike, secondStrike time.Duration) (time.Duration, bool) {
	if firstStrike <= 0 {
		firstStrike = 30 * time.Minute
	}
	if secondStrike <= 0 {
		secondStrike = 6 * time.Hour
	}

	switch {
	case keepRestricted:
		return permanentBanDuration, true
	case strikes <= 1:
		return firstStrike, true
	case strikes == 2:
		return secondStrike, true
	default:
		return permanentBanDuration, false
	}
}

func ComputeScore(weights []struct{ W, S float64 }) float64 {
	total := 0.0
	hasWeighted := false
	for _, w := range weights {
		if w.W > 0 || w.S > 0 {
			hasWeighted = true
			weight := w.W
			if weight == 0 {
				weight = 1.0
			}
			score := w.S
			if score == 0 {
				score = 1.0
			}
			total += weight * score
		} else {
			total++
		}
	}
	if total == 0 {
		return 1
	}
	_ = hasWeighted
	return total
}
