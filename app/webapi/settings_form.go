package webapi

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"

	"github.com/redstone-md/shield/app/rules"
)

// ruleSetFromForm applies submitted form values onto a copy of base and returns the
// updated ruleset plus a list of human-readable validation errors. Identity and
// versioning fields on base are preserved. When errs is non-empty the ruleset must
// not be persisted.
func ruleSetFromForm(base rules.RuleSet, form url.Values) (rs rules.RuleSet, errs []string) {
	rs = base

	intField := func(key string, minV, maxV int, target *int) {
		raw := form.Get(key)
		if raw == "" {
			return
		}
		v, err := strconv.Atoi(raw)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: not a whole number", key))
			return
		}
		if v < minV || v > maxV {
			errs = append(errs, fmt.Sprintf("%s: must be between %d and %d", key, minV, maxV))
			return
		}
		*target = v
	}
	floatField := func(key string, minV, maxV float64, target *float64) {
		raw := form.Get(key)
		if raw == "" {
			return
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: not a number", key))
			return
		}
		if v < minV || v > maxV {
			errs = append(errs, fmt.Sprintf("%s: must be between %.2f and %.2f", key, minV, maxV))
			return
		}
		*target = v
	}
	boolField := func(key string) bool { return form.Get(key) == "on" }
	enumField := func(key string, allowed []string, target *string) {
		raw := form.Get(key)
		if raw == "" {
			// treat absent/empty as "not submitted"; reset to zero value rather than error
			*target = ""
			return
		}
		if slices.Contains(allowed, raw) {
			*target = raw
			return
		}
		errs = append(errs, fmt.Sprintf("%s: must be one of %v", key, allowed))
	}

	// detection
	intField("detection.max_emoji", -1, 1000, &rs.Detection.MaxEmoji)
	intField("detection.min_msg_len", 0, 100000, &rs.Detection.MinMsgLen)
	floatField("detection.similarity_threshold", 0, 1, &rs.Detection.SimilarityThreshold)
	floatField("detection.min_spam_probability", 0, 100, &rs.Detection.MinSpamProbability)
	intField("detection.multi_lang_words", 0, 1000, &rs.Detection.MultiLangWords)
	intField("detection.history_size", 0, 1000000, &rs.Detection.HistorySize)
	intField("detection.first_messages_count", 0, 10000, &rs.Detection.FirstMessagesCount)
	rs.Detection.CasEnabled = boolField("detection.cas_enabled")
	rs.Detection.ParanoidMode = boolField("detection.paranoid_mode")

	// llm
	enumField("llm.mode", []string{"", "missed", "flagged", "always"}, &rs.LLM.Mode)
	enumField("llm.consensus", []string{"any", "all"}, &rs.LLM.Consensus)
	intField("llm.history_context_size", 0, 1000, &rs.LLM.HistoryContextSize)
	rs.LLM.VisionPrompt = form.Get("llm.vision_prompt")

	// openai
	rs.OpenAI.Veto = boolField("openai.veto")
	rs.OpenAI.CheckShortMessages = boolField("openai.check_short_messages")
	rs.OpenAI.Model = form.Get("openai.model")
	rs.OpenAI.Prompt = form.Get("openai.prompt")
	intField("openai.history_size", 0, 1000000, &rs.OpenAI.HistorySize)

	// gemini
	rs.Gemini.Veto = boolField("gemini.veto")
	rs.Gemini.CheckShortMessages = boolField("gemini.check_short_messages")
	rs.Gemini.Model = form.Get("gemini.model")
	rs.Gemini.Prompt = form.Get("gemini.prompt")
	intField("gemini.history_size", 0, 1000000, &rs.Gemini.HistorySize)

	rs.SlowPathEnabled = boolField("slow_path_enabled")

	return rs, errs
}
