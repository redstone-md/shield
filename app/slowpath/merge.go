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
