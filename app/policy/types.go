package policy

type RiskType string

const (
	RiskSpam    RiskType = "spam"
	RiskAbuse   RiskType = "abuse"
	RiskScam    RiskType = "scam"
	RiskRaid    RiskType = "raid"
	RiskNSFW    RiskType = "nsfw"
	RiskUnknown RiskType = "unknown"
)

type ActionLevel int

const (
	LevelNone ActionLevel = iota
	LevelWarn
	LevelMute
	LevelDeleteAndMute
	LevelBan
)

type EscalationConfig struct {
	Enabled bool
	Levels  []ActionLevel
}

type ModerationConfig struct {
	FirstStrike  string
	SecondStrike string
	SoftBan      bool
}
