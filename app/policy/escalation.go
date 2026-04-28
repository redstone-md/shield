package policy

func ApplyEscalation(base ActionLevel, strikes int, cfg EscalationConfig) ActionLevel {
	if !cfg.Enabled || strikes <= 0 {
		return base
	}
	if len(cfg.Levels) == 0 {
		return base
	}
	idx := strikes - 1
	if idx >= len(cfg.Levels) {
		idx = len(cfg.Levels) - 1
	}
	escalated := cfg.Levels[idx]
	if escalated > base {
		return escalated
	}
	return base
}
