package slowpath

type EscalationCheck struct {
	HasImages      bool
	AmbiguousScore bool
	ForceLLM       bool
	HighRiskPolicy bool
	UserReport     bool
}

func ShouldEscalate(check EscalationCheck) (bool, EscalationReason) {
	if check.ForceLLM {
		return true, EscalationForceLLM
	}
	if check.UserReport {
		return true, EscalationUserReport
	}
	if check.HighRiskPolicy {
		return true, EscalationHighRiskPolicy
	}
	if check.HasImages {
		return true, EscalationImageContent
	}
	if check.AmbiguousScore {
		return true, EscalationAmbiguousFast
	}
	return false, ""
}

func MergeResults(fast DetectionResult, slow *SlowPathResult) DetectionResult {
	if slow == nil || slow.Skipped {
		return fast
	}

	merged := DetectionResult{
		Spam:    fast.Spam,
		Signals: make([]DetectionSignal, len(fast.Signals)),
	}
	copy(merged.Signals, fast.Signals)

	for _, pr := range slow.Signals {
		merged.Signals = append(merged.Signals, DetectionSignal{
			Name:    pr.Provider,
			Score:   float64(pr.Confidence) / 100.0,
			Matched: pr.Spam,
			Reason:  pr.Reason,
		})
	}

	if slow.Final && len(slow.Signals) > 0 {
		merged.Spam = slow.Spam
		if slow.Spam {
			merged.Score = max(fast.Score, float64(slow.Confidence)/100.0)
		} else {
			merged.Score = fast.Score
		}
	}

	return merged
}
