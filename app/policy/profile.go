package policy

import (
	"strings"
	"time"
)

type PolicyProfile struct {
	Name      string               `json:"name"`
	Version   int                  `json:"version"`
	Matrix    map[RiskType]ActionLevel `json:"matrix"`
	Escalate  EscalationConfig     `json:"escalate"`
	CreatedAt time.Time            `json:"created_at"`
}

func PermissiveProfile() PolicyProfile {
	return PolicyProfile{
		Name:    "permissive",
		Version: 1,
		Matrix: map[RiskType]ActionLevel{
			RiskSpam:   LevelMute,
			RiskAbuse:  LevelWarn,
			RiskScam:   LevelMute,
			RiskRaid:   LevelMute,
			RiskNSFW:   LevelWarn,
			RiskUnknown: LevelMute,
		},
		Escalate: EscalationConfig{
			Enabled: true,
			Levels:  []ActionLevel{LevelWarn, LevelMute, LevelDeleteAndMute, LevelBan},
		},
		CreatedAt: time.Now().UTC(),
	}
}

func BalancedProfile() PolicyProfile {
	return PolicyProfile{
		Name:    "balanced",
		Version: 1,
		Matrix: map[RiskType]ActionLevel{
			RiskSpam:   LevelDeleteAndMute,
			RiskAbuse:  LevelMute,
			RiskScam:   LevelDeleteAndMute,
			RiskRaid:   LevelDeleteAndMute,
			RiskNSFW:   LevelMute,
			RiskUnknown: LevelDeleteAndMute,
		},
		Escalate: EscalationConfig{
			Enabled: true,
			Levels:  []ActionLevel{LevelMute, LevelDeleteAndMute, LevelBan, LevelBan},
		},
		CreatedAt: time.Now().UTC(),
	}
}

func StrictProfile() PolicyProfile {
	return PolicyProfile{
		Name:    "strict",
		Version: 1,
		Matrix: map[RiskType]ActionLevel{
			RiskSpam:   LevelBan,
			RiskAbuse:  LevelBan,
			RiskScam:   LevelBan,
			RiskRaid:   LevelBan,
			RiskNSFW:   LevelDeleteAndMute,
			RiskUnknown: LevelBan,
		},
		Escalate: EscalationConfig{
			Enabled: true,
			Levels:  []ActionLevel{LevelDeleteAndMute, LevelBan, LevelBan, LevelBan},
		},
		CreatedAt: time.Now().UTC(),
	}
}

var checkNameToRiskType = map[string]RiskType{
	"stop-word":        RiskSpam,
	"stopwords":        RiskSpam,
	"similarity":       RiskSpam,
	"links":            RiskSpam,
	"meta-links":       RiskSpam,
	"classifier":       RiskSpam,
	"cas":              RiskSpam,
	"duplicate":        RiskRaid,
	"mentions":         RiskAbuse,
	"username-symbols": RiskAbuse,
	"image-only":       RiskSpam,
	"video-only":       RiskNSFW,
	"audio-only":       RiskSpam,
	"giveaway":         RiskScam,
	"abnormal-spacing": RiskSpam,
	"multi-lang":       RiskSpam,
	"openai":           RiskSpam,
	"gemini":           RiskSpam,
}

func ClassifyRisk(checkName string) RiskType {
	name := strings.ToLower(strings.TrimSpace(checkName))
	if rt, ok := checkNameToRiskType[name]; ok {
		return rt
	}
	for key, rt := range checkNameToRiskType {
		if strings.Contains(name, key) {
			return rt
		}
	}
	return RiskUnknown
}

func ResolveProfile(name string) PolicyProfile {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "permissive":
		return PermissiveProfile()
	case "strict":
		return StrictProfile()
	default:
		return BalancedProfile()
	}
}
