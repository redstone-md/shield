package rules

import "time"

// CurrentSchemaVersion is the RuleSet payload schema version. Bump it whenever new
// fields are added so older persisted rulesets can be detected and backfilled.
const CurrentSchemaVersion = 5

// RuleSet is the single-tenant moderation configuration snapshot.
type RuleSet struct {
	WorkspaceID   string    `json:"workspace_id"`
	Version       int       `json:"version"`
	Source        string    `json:"source"`
	SchemaVersion int       `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`

	Meta            MetaRules            `json:"meta"`
	Duplicates      DuplicateRules       `json:"duplicates"`
	AbnormalSpacing AbnormalSpacingRules `json:"abnormal_spacing"`
	Moderation      ModerationRules      `json:"moderation"`
	Reports         ReportRules          `json:"reports"`
	Detection       DetectionRules       `json:"detection"`
	LLM             LLMCommonRules       `json:"llm"`
	OpenAI          LLMRules             `json:"openai"`
	Gemini          LLMRules             `json:"gemini"`
	PolicyProfile   string               `json:"policy_profile"`
	SlowPathEnabled bool                 `json:"slow_path_enabled"`
	JoinGate        JoinGateRules        `json:"join_gate"`
}

// JoinGateRules configures the join-time datacenter gate: when a user joins a
// monitored chat and their profile-photo datacenter is in BannedDCs, the user
// is banned preemptively across all primary chats. An empty list disables the
// gate. DC values are Telegram datacenter ids (1-5).
type JoinGateRules struct {
	BannedDCs []int `json:"banned_dcs"`
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
	FirstStrike        time.Duration `json:"first_strike"`
	SecondStrike       time.Duration `json:"second_strike"`
	WarnStrikes        int           `json:"warn_strikes"`
	SoftBan            bool          `json:"soft_ban"`
	DryRun             bool          `json:"dry_run"`
	WarnDeleteDuration time.Duration `json:"warn_delete_duration"`
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
	VisionModel        string   `json:"vision_model"`
	Prompt             string   `json:"prompt"`
	HistorySize        int      `json:"history_size"`
	CheckShortMessages bool     `json:"check_short_messages"`
	CustomPrompts      []string `json:"custom_prompts,omitempty"`
}

// DetectionRules holds detection tuning previously sourced only from env flags.
type DetectionRules struct {
	MaxEmoji            int     `json:"max_emoji"`
	MinMsgLen           int     `json:"min_msg_len"`
	SimilarityThreshold float64 `json:"similarity_threshold"`
	MinSpamProbability  float64 `json:"min_spam_probability"`
	MultiLangWords      int     `json:"multi_lang_words"`
	ChineseMode         bool    `json:"chinese_mode"`
	ChineseCharRatio    float64 `json:"chinese_char_ratio"`
	CasEnabled          bool    `json:"cas_enabled"`
	HistorySize         int     `json:"history_size"`
	FirstMessagesCount  int     `json:"first_messages_count"`
	ParanoidMode        bool    `json:"paranoid_mode"`
}

// LLMCommonRules holds LLM settings shared across providers.
type LLMCommonRules struct {
	Mode               string `json:"mode"`      // "" | missed | flagged | always
	Consensus          string `json:"consensus"` // any | all
	HistoryContextSize int    `json:"history_context_size"`
	VisionPrompt       string `json:"vision_prompt"`
}
