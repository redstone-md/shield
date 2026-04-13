package rules

import "time"

// RuleSet is the single-tenant moderation configuration snapshot.
type RuleSet struct {
	WorkspaceID string    `json:"workspace_id"`
	Version     int       `json:"version"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"created_at"`

	Meta            MetaRules            `json:"meta"`
	Duplicates      DuplicateRules       `json:"duplicates"`
	AbnormalSpacing AbnormalSpacingRules `json:"abnormal_spacing"`
	Moderation      ModerationRules      `json:"moderation"`
	Reports         ReportRules          `json:"reports"`
	OpenAI          LLMRules             `json:"openai"`
	Gemini          LLMRules             `json:"gemini"`
}

type MetaRules struct {
	LinksLimit      int    `json:"links_limit"`
	MentionsLimit   int    `json:"mentions_limit"`
	ImageOnly       bool   `json:"image_only"`
	LinksOnly       bool   `json:"links_only"`
	VideosOnly      bool   `json:"videos_only"`
	AudiosOnly      bool   `json:"audios_only"`
	ContactOnly     bool   `json:"contact_only"`
	Forwarded       bool   `json:"forwarded"`
	Keyboard        bool   `json:"keyboard"`
	UsernameSymbols string `json:"username_symbols"`
	Giveaway        bool   `json:"giveaway"`
}

type DuplicateRules struct {
	Threshold int           `json:"threshold"`
	Window    time.Duration `json:"window"`
}

type AbnormalSpacingRules struct {
	Enabled                 bool    `json:"enabled"`
	SpaceRatioThreshold     float64 `json:"space_ratio_threshold"`
	ShortWordRatioThreshold float64 `json:"short_word_ratio_threshold"`
	ShortWordLen            int     `json:"short_word_len"`
	MinWords                int     `json:"min_words"`
}

type ModerationRules struct {
	FirstStrike  time.Duration `json:"first_strike"`
	SecondStrike time.Duration `json:"second_strike"`
	SoftBan      bool          `json:"soft_ban"`
	DryRun       bool          `json:"dry_run"`
}

type ReportRules struct {
	Enabled          bool          `json:"enabled"`
	Threshold        int           `json:"threshold"`
	AutoBanThreshold int           `json:"auto_ban_threshold"`
	RateLimit        int           `json:"rate_limit"`
	RatePeriod       time.Duration `json:"rate_period"`
}

type LLMRules struct {
	Enabled            bool     `json:"enabled"`
	Veto               bool     `json:"veto"`
	Model              string   `json:"model"`
	HistorySize        int      `json:"history_size"`
	CheckShortMessages bool     `json:"check_short_messages"`
	CustomPrompts      []string `json:"custom_prompts,omitempty"`
}
