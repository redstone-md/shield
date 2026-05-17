package main

import (
	"os"
	"strings"

	"github.com/umputun/tg-spam/app/rules"
)

func applyExplicitRuleSetOverrides(rs *rules.RuleSet, opts options) {
	applyExplicitMetaOverrides(rs, opts)
	applyExplicitDuplicateOverrides(rs, opts)
	applyExplicitSpacingOverrides(rs, opts)
	applyExplicitModerationOverrides(rs, opts)
	applyExplicitReportOverrides(rs, opts)
	applyExplicitLLMOverrides(rs, opts)
	applyExplicitDetectionOverrides(rs, opts)
}

func applyExplicitMetaOverrides(rs *rules.RuleSet, opts options) {
	if configured("meta.links-limit", "META_LINKS_LIMIT") {
		rs.Meta.LinksLimit = opts.Meta.LinksLimit
	}
	if configured("meta.mentions-limit", "META_MENTIONS_LIMIT") {
		rs.Meta.MentionsLimit = opts.Meta.MentionsLimit
	}
	if configured("meta.image-only", "META_IMAGE_ONLY") {
		rs.Meta.ImageOnly = opts.Meta.ImageOnly
	}
	if configured("meta.links-only", "META_LINKS_ONLY") {
		rs.Meta.LinksOnly = opts.Meta.LinksOnly
	}
	if configured("meta.video-only", "META_VIDEO_ONLY") {
		rs.Meta.VideosOnly = opts.Meta.VideosOnly
	}
	if configured("meta.audio-only", "META_AUDIO_ONLY") {
		rs.Meta.AudiosOnly = opts.Meta.AudiosOnly
	}
	if configured("meta.contact-only", "META_CONTACT_ONLY") {
		rs.Meta.ContactOnly = opts.Meta.ContactOnly
	}
	if configured("meta.forward", "META_FORWARD") {
		rs.Meta.Forwarded = opts.Meta.Forward
	}
	if configured("meta.keyboard", "META_KEYBOARD") {
		rs.Meta.Keyboard = opts.Meta.Keyboard
	}
	if configured("meta.username-symbols", "META_USERNAME_SYMBOLS") {
		rs.Meta.UsernameSymbols = opts.Meta.UsernameSymbols
	}
	if configured("meta.giveaway", "META_GIVEAWAY") {
		rs.Meta.Giveaway = opts.Meta.Giveaway
	}
}

func applyExplicitDuplicateOverrides(rs *rules.RuleSet, opts options) {
	if configured("duplicates.threshold", "DUPLICATES_THRESHOLD") {
		rs.Duplicates.Threshold = opts.Duplicates.Threshold
	}
	if configured("duplicates.window", "DUPLICATES_WINDOW") {
		rs.Duplicates.Window = opts.Duplicates.Window
	}
}

func applyExplicitSpacingOverrides(rs *rules.RuleSet, opts options) {
	if configured("space.enabled", "SPACE_ENABLED") {
		rs.AbnormalSpacing.Enabled = opts.AbnormalSpacing.Enabled
	}
	if configured("space.ratio", "SPACE_RATIO") {
		rs.AbnormalSpacing.SpaceRatioThreshold = opts.AbnormalSpacing.SpaceRatioThreshold
	}
	if configured("space.short-ratio", "SPACE_SHORT_RATIO") {
		rs.AbnormalSpacing.ShortWordRatioThreshold = opts.AbnormalSpacing.ShortWordRatioThreshold
	}
	if configured("space.short-word", "SPACE_SHORT_WORD") {
		rs.AbnormalSpacing.ShortWordLen = opts.AbnormalSpacing.ShortWordLen
	}
	if configured("space.min-words", "SPACE_MIN_WORDS") {
		rs.AbnormalSpacing.MinWords = opts.AbnormalSpacing.MinWords
	}
}

func applyExplicitModerationOverrides(rs *rules.RuleSet, opts options) {
	if configured("moderation.first-strike", "MODERATION_FIRST_STRIKE") {
		rs.Moderation.FirstStrike = opts.Moderation.FirstStrike
	}
	if configured("moderation.second-strike", "MODERATION_SECOND_STRIKE") {
		rs.Moderation.SecondStrike = opts.Moderation.SecondStrike
	}
	if configured("moderation.warn-strikes", "MODERATION_WARN_STRIKES") {
		rs.Moderation.WarnStrikes = opts.Moderation.WarnStrikes
	}
	if configured("moderation.warn-delete-duration", "MODERATION_WARN_DELETE_DURATION") {
		rs.Moderation.WarnDeleteDuration = opts.Moderation.WarnDeleteDuration
	}
	if configured("soft-ban", "SOFT_BAN") {
		rs.Moderation.SoftBan = opts.SoftBan
	}
	if configured("dry", "DRY") {
		rs.Moderation.DryRun = opts.Dry
	}
}

func applyExplicitReportOverrides(rs *rules.RuleSet, opts options) {
	if configured("report.enabled", "REPORT_ENABLED") {
		rs.Reports.Enabled = opts.Report.Enabled
	}
	if configured("report.threshold", "REPORT_THRESHOLD") {
		rs.Reports.Threshold = opts.Report.Threshold
	}
	if configured("report.auto-ban-threshold", "REPORT_AUTO_BAN_THRESHOLD") {
		rs.Reports.AutoBanThreshold = opts.Report.AutoBanThreshold
	}
	if configured("report.rate-limit", "REPORT_RATE_LIMIT") {
		rs.Reports.RateLimit = opts.Report.RateLimit
	}
	if configured("report.rate-period", "REPORT_RATE_PERIOD") {
		rs.Reports.RatePeriod = opts.Report.RatePeriod
	}
}

func applyExplicitLLMOverrides(rs *rules.RuleSet, opts options) {
	if configured("openai.token", "OPENAI_TOKEN") || configured("openai.apibase", "OPENAI_API_BASE") {
		rs.OpenAI.Enabled = opts.OpenAI.Token != "" || opts.OpenAI.APIBase != ""
	}
	if configured("openai.veto", "OPENAI_VETO") {
		rs.OpenAI.Veto = opts.OpenAI.Veto
	}
	if configured("openai.model", "OPENAI_MODEL") {
		rs.OpenAI.Model = opts.OpenAI.Model
	}
	if configured("openai.history-size", "OPENAI_HISTORY_SIZE") {
		rs.OpenAI.HistorySize = opts.OpenAI.HistorySize
	}
	if configured("openai.check-short-messages", "OPENAI_CHECK_SHORT_MESSAGES") {
		rs.OpenAI.CheckShortMessages = opts.OpenAI.CheckShortMessages
	}
	if configured("openai.custom-prompt", "OPENAI_CUSTOM_PROMPT") {
		rs.OpenAI.CustomPrompts = opts.OpenAI.CustomPrompts
	}
	if configured("openai.prompt", "OPENAI_PROMPT") {
		rs.OpenAI.Prompt = opts.OpenAI.Prompt
	}

	if configured("gemini.token", "GEMINI_TOKEN") {
		rs.Gemini.Enabled = opts.Gemini.Token != ""
	}
	if configured("gemini.veto", "GEMINI_VETO") {
		rs.Gemini.Veto = opts.Gemini.Veto
	}
	if configured("gemini.model", "GEMINI_MODEL") {
		rs.Gemini.Model = opts.Gemini.Model
	}
	if configured("gemini.history-size", "GEMINI_HISTORY_SIZE") {
		rs.Gemini.HistorySize = opts.Gemini.HistorySize
	}
	if configured("gemini.check-short-messages", "GEMINI_CHECK_SHORT_MESSAGES") {
		rs.Gemini.CheckShortMessages = opts.Gemini.CheckShortMessages
	}
	if configured("gemini.custom-prompt", "GEMINI_CUSTOM_PROMPT") {
		rs.Gemini.CustomPrompts = opts.Gemini.CustomPrompts
	}
	if configured("gemini.prompt", "GEMINI_PROMPT") {
		rs.Gemini.Prompt = opts.Gemini.Prompt
	}
}

func applyExplicitDetectionOverrides(rs *rules.RuleSet, opts options) {
	if configured("max-emoji", "MAX_EMOJI") {
		rs.Detection.MaxEmoji = opts.MaxEmoji
	}
	if configured("min-msg-len", "MIN_MSG_LEN") {
		rs.Detection.MinMsgLen = opts.MinMsgLen
	}
	if configured("similarity-threshold", "SIMILARITY_THRESHOLD") {
		rs.Detection.SimilarityThreshold = opts.SimilarityThreshold
	}
	if configured("min-probability", "MIN_PROBABILITY") {
		rs.Detection.MinSpamProbability = opts.MinSpamProbability
	}
	if configured("multi-lang", "MULTI_LANG") {
		rs.Detection.MultiLangWords = opts.MultiLangWords
	}
	if configured("first-messages-count", "FIRST_MESSAGES_COUNT") {
		rs.Detection.FirstMessagesCount = opts.FirstMessagesCount
	}
	if configured("paranoid", "PARANOID") {
		rs.Detection.ParanoidMode = opts.ParanoidMode
	}
	if configured("history-min-size", "HISTORY_MIN_SIZE") {
		rs.Detection.HistorySize = opts.HistoryMinSize
	}
	if configured("cas.api", "CAS_API") {
		rs.Detection.CasEnabled = opts.CAS.API != ""
	}
	if configured("llm.mode", "LLM_MODE") {
		rs.LLM.Mode = opts.LLM.Mode
	}
	if configured("llm.consensus", "LLM_CONSENSUS") {
		rs.LLM.Consensus = opts.LLM.Consensus
	}
}

// envPinnedKey maps a RuleSet JSON path to its CLI flag and env var.
type envPinnedKey struct {
	flag string
	env  string
}

var envPinnedRegistry = map[string]envPinnedKey{
	"detection.max_emoji":            {"max-emoji", "MAX_EMOJI"},
	"detection.min_msg_len":          {"min-msg-len", "MIN_MSG_LEN"},
	"detection.similarity_threshold": {"similarity-threshold", "SIMILARITY_THRESHOLD"},
	"detection.min_spam_probability": {"min-probability", "MIN_PROBABILITY"},
	"detection.multi_lang_words":     {"multi-lang", "MULTI_LANG"},
	"detection.first_messages_count": {"first-messages-count", "FIRST_MESSAGES_COUNT"},
	"detection.paranoid_mode":        {"paranoid", "PARANOID"},
	"detection.history_size":         {"history-min-size", "HISTORY_MIN_SIZE"},
	"detection.cas_enabled":          {"cas.api", "CAS_API"},
	"llm.mode":                       {"llm.mode", "LLM_MODE"},
	"llm.consensus":                  {"llm.consensus", "LLM_CONSENSUS"},
	"openai.veto":                    {"openai.veto", "OPENAI_VETO"},
	"openai.model":                   {"openai.model", "OPENAI_MODEL"},
	"openai.history_size":            {"openai.history-size", "OPENAI_HISTORY_SIZE"},
	"openai.check_short_messages":    {"openai.check-short-messages", "OPENAI_CHECK_SHORT_MESSAGES"},
	"openai.prompt":                  {"openai.prompt", "OPENAI_PROMPT"},
	"gemini.veto":                    {"gemini.veto", "GEMINI_VETO"},
	"gemini.model":                   {"gemini.model", "GEMINI_MODEL"},
	"gemini.history_size":            {"gemini.history-size", "GEMINI_HISTORY_SIZE"},
	"gemini.check_short_messages":    {"gemini.check-short-messages", "GEMINI_CHECK_SHORT_MESSAGES"},
	"gemini.prompt":                  {"gemini.prompt", "GEMINI_PROMPT"},
}

// envPinnedKeys returns RuleSet JSON paths whose value is explicitly set via env/CLI
// and therefore overrides the stored ruleset on the next restart.
func envPinnedKeys() map[string]bool {
	pinned := make(map[string]bool, len(envPinnedRegistry))
	for path, k := range envPinnedRegistry {
		if configured(k.flag, k.env) {
			pinned[path] = true
		}
	}
	return pinned
}

func configured(flagName, envName string) bool {
	if v, ok := os.LookupEnv(envName); ok && v != "" {
		return true
	}
	return cliFlagSet(flagName)
}

func cliFlagSet(name string) bool {
	long := "--" + name
	for _, arg := range os.Args[1:] {
		if arg == long || strings.HasPrefix(arg, long+"=") {
			return true
		}
	}
	return false
}
