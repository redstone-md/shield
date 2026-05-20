# Web Config — Backend (RuleSet extension + precedence) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move env-only detection tuning knobs into the versioned `rules.RuleSet` so they become persistent, hot-reloadable, and editable through the existing `PUT /api/rules/` API.

**Architecture:** `rules.RuleSet` gains a `Detection` and `LLM` sub-struct plus two `LLMRules` fields. `bootstrapRuleSet` seeds them from env on first boot; `buildDetectorConfig` reads them from the ruleset instead of `opts`. The env→ruleset override layer (`settings_precedence.go`) keeps working for backwards compatibility; empty env values are treated as not-set, and a reporting function lists env-pinned keys for the future UI. A `SchemaVersion` sentinel triggers a one-time backfill for rulesets that predate the new fields.

**Tech Stack:** Go 1.24, `jessevdk/go-flags`, `stretchr/testify`, SQLite via `jmoiron/sqlx`.

**Scope:** This plan is the backend foundation. The web UI (Plan 3) and LLM-prompt hot-reload + vision-prompt persistence (Plan 2) are separate plans. After this plan the new fields are fully functional and editable via the existing JSON API `PUT /api/rules/`.

---

## File Structure

- `app/rules/ruleset.go` — add `DetectionRules`, `LLMCommonRules` structs; add `Detection`, `LLM`, `SchemaVersion` fields to `RuleSet`; add `Prompt`, `VisionModel` to `LLMRules`.
- `app/rules/ruleset_test.go` — new; JSON round-trip + zero-value tests.
- `app/runtime_assembly.go` — extend `bootstrapRuleSet`; add backfill after `ruleSets.Active`.
- `app/assembly.go` — `buildDetectorConfig` reads new fields from `ruleSet`.
- `app/settings_precedence.go` — empty-env fix in `configured`; overrides for new fields; `envPinnedKeys` reporting function.
- `app/settings_precedence_test.go` — new; `configured` + `envPinnedKeys` tests.
- Existing `app/assembly_*_test.go` / `app/main_*_test.go` — extended where noted.

---

## Task 1: Extend `rules.RuleSet` with new structs and fields

**Files:**
- Modify: `app/rules/ruleset.go`
- Test: `app/rules/ruleset_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `app/rules/ruleset_test.go`:

```go
package rules

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleSet_NewFieldsJSONRoundTrip(t *testing.T) {
	rs := RuleSet{
		WorkspaceID:   "tg-spam",
		Version:       3,
		SchemaVersion: 1,
		Detection: DetectionRules{
			MaxEmoji:            5,
			MinMsgLen:           50,
			SimilarityThreshold: 0.5,
			MinSpamProbability:  50,
			MultiLangWords:      2,
			CasEnabled:          true,
			HistorySize:         1000,
			FirstMessagesCount:  1,
			ParanoidMode:        false,
		},
		LLM: LLMCommonRules{Mode: "flagged", Consensus: "any"},
		OpenAI: LLMRules{
			Model:       "gpt-4o-mini",
			Prompt:      "be strict",
			VisionModel: "gpt-4o",
		},
	}

	data, err := json.Marshal(rs)
	require.NoError(t, err)

	var got RuleSet
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, rs, got)
}

func TestRuleSet_LegacyPayloadDecodesNewFieldsAsZero(t *testing.T) {
	// payload written before the new fields existed
	legacy := `{"workspace_id":"tg-spam","version":1,"openai":{"model":"gpt-4o-mini"}}`

	var got RuleSet
	require.NoError(t, json.Unmarshal([]byte(legacy), &got))
	assert.Equal(t, 0, got.SchemaVersion)
	assert.Equal(t, DetectionRules{}, got.Detection)
	assert.Equal(t, LLMCommonRules{}, got.LLM)
	assert.Empty(t, got.OpenAI.Prompt)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/rules/ -run TestRuleSet -v`
Expected: FAIL — compile error, `SchemaVersion`/`Detection`/`LLM`/`DetectionRules`/`LLMCommonRules` undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/rules/ruleset.go`, add `SchemaVersion`, `Detection`, `LLM` to the `RuleSet` struct (after `CreatedAt`):

```go
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
```

Add `Prompt` and `VisionModel` to `LLMRules`:

```go
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
```

Add the two new structs at the end of the file:

```go
// DetectionRules holds detection tuning previously sourced only from env flags.
type DetectionRules struct {
	MaxEmoji            int     `json:"max_emoji"`
	MinMsgLen           int     `json:"min_msg_len"`
	SimilarityThreshold float64 `json:"similarity_threshold"`
	MinSpamProbability  float64 `json:"min_spam_probability"`
	MultiLangWords      int     `json:"multi_lang_words"`
	CasEnabled          bool    `json:"cas_enabled"`
	HistorySize         int     `json:"history_size"`
	FirstMessagesCount  int     `json:"first_messages_count"`
	ParanoidMode        bool    `json:"paranoid_mode"`
}

// LLMCommonRules holds LLM settings shared across providers.
type LLMCommonRules struct {
	Mode      string `json:"mode"`      // "" | missed | flagged | always
	Consensus string `json:"consensus"` // any | all
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/rules/ -run TestRuleSet -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add app/rules/ruleset.go app/rules/ruleset_test.go
git commit -m "feat(rules): add Detection, LLM and schema-version fields to RuleSet"
```

---

## Task 2: Define the current schema version constant

**Files:**
- Modify: `app/rules/ruleset.go`
- Test: `app/rules/ruleset_test.go`

- [ ] **Step 1: Write the failing test**

Append to `app/rules/ruleset_test.go`:

```go
func TestCurrentSchemaVersion(t *testing.T) {
	assert.Equal(t, 1, CurrentSchemaVersion)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/rules/ -run TestCurrentSchemaVersion -v`
Expected: FAIL — `CurrentSchemaVersion` undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/rules/ruleset.go`, add after the `import` block:

```go
// CurrentSchemaVersion is the RuleSet payload schema version. Bump it whenever new
// fields are added so older persisted rulesets can be detected and backfilled.
const CurrentSchemaVersion = 1
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/rules/ -run TestCurrentSchemaVersion -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/rules/ruleset.go app/rules/ruleset_test.go
git commit -m "feat(rules): add CurrentSchemaVersion constant"
```

---

## Task 3: Seed the new fields in `bootstrapRuleSet`

**Files:**
- Modify: `app/runtime_assembly.go` (`bootstrapRuleSet`, ~line 318-378)
- Test: `app/runtime_assembly_test.go` (add test; create file if absent)

- [ ] **Step 1: Write the failing test**

Add to `app/runtime_assembly_test.go` (create the file with package `main` and the imports below if it does not exist):

```go
func TestBootstrapRuleSet_SeedsNewFields(t *testing.T) {
	var opts options
	opts.InstanceID = "tg-spam"
	opts.MaxEmoji = 7
	opts.MinMsgLen = 40
	opts.SimilarityThreshold = 0.6
	opts.MinSpamProbability = 55
	opts.MultiLangWords = 3
	opts.CAS.API = "https://api.cas.chat"
	opts.HistoryMinSize = 1234
	opts.FirstMessagesCount = 2
	opts.ParanoidMode = true
	opts.LLM.Mode = "flagged"
	opts.LLM.Consensus = "all"
	opts.OpenAI.Prompt = "strict openai"
	opts.OpenAI.VisionModel = "gpt-4o"
	opts.Gemini.Prompt = "strict gemini"
	opts.Gemini.VisionModel = "gemini-vision"

	rs := bootstrapRuleSet(opts)

	assert.Equal(t, rules.CurrentSchemaVersion, rs.SchemaVersion)
	assert.Equal(t, 7, rs.Detection.MaxEmoji)
	assert.Equal(t, 40, rs.Detection.MinMsgLen)
	assert.InEpsilon(t, 0.6, rs.Detection.SimilarityThreshold, 0.0001)
	assert.InEpsilon(t, 55.0, rs.Detection.MinSpamProbability, 0.0001)
	assert.Equal(t, 3, rs.Detection.MultiLangWords)
	assert.True(t, rs.Detection.CasEnabled)
	assert.Equal(t, 1234, rs.Detection.HistorySize)
	assert.Equal(t, 2, rs.Detection.FirstMessagesCount)
	assert.True(t, rs.Detection.ParanoidMode)
	assert.Equal(t, "flagged", rs.LLM.Mode)
	assert.Equal(t, "all", rs.LLM.Consensus)
	assert.Equal(t, "strict openai", rs.OpenAI.Prompt)
	assert.Equal(t, "gpt-4o", rs.OpenAI.VisionModel)
	assert.Equal(t, "strict gemini", rs.Gemini.Prompt)
	assert.Equal(t, "gemini-vision", rs.Gemini.VisionModel)
}

func TestBootstrapRuleSet_CasDisabledWhenNoAPI(t *testing.T) {
	var opts options
	opts.InstanceID = "tg-spam"
	rs := bootstrapRuleSet(opts)
	assert.False(t, rs.Detection.CasEnabled)
}
```

Required imports for the file: `"testing"`, `"github.com/stretchr/testify/assert"`, `"github.com/redstone-md/shield/app/rules"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestBootstrapRuleSet -v`
Expected: FAIL — `rs.Detection`, `rs.LLM`, `rs.SchemaVersion`, `rs.OpenAI.Prompt` not populated (zero values).

- [ ] **Step 3: Write minimal implementation**

In `app/runtime_assembly.go`, in `bootstrapRuleSet`, set `SchemaVersion` and add the `Detection` / `LLM` blocks, and the `Prompt` / `VisionModel` fields to the OpenAI / Gemini blocks:

```go
func bootstrapRuleSet(opts options) rules.RuleSet {
	return rules.RuleSet{
		WorkspaceID:   opts.InstanceID,
		Source:        "bootstrap",
		SchemaVersion: rules.CurrentSchemaVersion,
		Meta: rules.MetaRules{
```

Add the `Detection` and `LLM` fields (place them after the `Reports` block, before `OpenAI`):

```go
		Detection: rules.DetectionRules{
			MaxEmoji:            opts.MaxEmoji,
			MinMsgLen:           opts.MinMsgLen,
			SimilarityThreshold: opts.SimilarityThreshold,
			MinSpamProbability:  opts.MinSpamProbability,
			MultiLangWords:      opts.MultiLangWords,
			CasEnabled:          opts.CAS.API != "",
			HistorySize:         opts.HistoryMinSize,
			FirstMessagesCount:  opts.FirstMessagesCount,
			ParanoidMode:        opts.ParanoidMode,
		},
		LLM: rules.LLMCommonRules{
			Mode:      opts.LLM.Mode,
			Consensus: opts.LLM.Consensus,
		},
```

Extend the existing `OpenAI` block with two fields:

```go
		OpenAI: rules.LLMRules{
			Enabled:            opts.OpenAI.Token != "" || opts.OpenAI.APIBase != "",
			Veto:               opts.OpenAI.Veto,
			Model:              opts.OpenAI.Model,
			VisionModel:        opts.OpenAI.VisionModel,
			Prompt:             opts.OpenAI.Prompt,
			HistorySize:        opts.OpenAI.HistorySize,
			CheckShortMessages: opts.OpenAI.CheckShortMessages,
			CustomPrompts:      opts.OpenAI.CustomPrompts,
		},
		Gemini: rules.LLMRules{
			Enabled:            opts.Gemini.Token != "",
			Veto:               opts.Gemini.Veto,
			Model:              opts.Gemini.Model,
			VisionModel:        opts.Gemini.VisionModel,
			Prompt:             opts.Gemini.Prompt,
			HistorySize:        opts.Gemini.HistorySize,
			CheckShortMessages: opts.Gemini.CheckShortMessages,
			CustomPrompts:      opts.Gemini.CustomPrompts,
		},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestBootstrapRuleSet -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add app/runtime_assembly.go app/runtime_assembly_test.go
git commit -m "feat(config): seed Detection and LLM ruleset fields from env in bootstrap"
```

---

## Task 4: `buildDetectorConfig` reads new fields from the ruleset

**Files:**
- Modify: `app/assembly.go` (`buildDetectorConfig`, lines 326-369)
- Test: `app/assembly_test.go` (add test; use the existing assembly test file — confirm exact name with `ls app/assembly*_test.go`)

- [ ] **Step 1: Write the failing test**

Add to the assembly test file (package `main`):

```go
func TestBuildDetectorConfig_ReadsDetectionFromRuleSet(t *testing.T) {
	var opts options
	// opts values must be ignored for these fields now
	opts.MaxEmoji = 999
	opts.MinMsgLen = 999
	opts.SimilarityThreshold = 9.9
	opts.MinSpamProbability = 99
	opts.MultiLangWords = 99
	opts.FirstMessagesCount = 99
	opts.LLM.Mode = "always"
	opts.LLM.Consensus = "all"
	opts.CAS.API = "https://api.cas.chat"

	rs := rules.RuleSet{
		Detection: rules.DetectionRules{
			MaxEmoji:            3,
			MinMsgLen:           50,
			SimilarityThreshold: 0.5,
			MinSpamProbability:  50,
			MultiLangWords:      2,
			CasEnabled:          true,
			FirstMessagesCount:  1,
			ParanoidMode:        false,
		},
		LLM: rules.LLMCommonRules{Mode: "flagged", Consensus: "any"},
	}

	cfg := buildDetectorConfig(opts, rs)

	assert.Equal(t, 3, cfg.MaxAllowedEmoji)
	assert.Equal(t, 50, cfg.MinMsgLen)
	assert.InEpsilon(t, 0.5, cfg.SimilarityThreshold, 0.0001)
	assert.InEpsilon(t, 50.0, cfg.MinSpamProbability, 0.0001)
	assert.Equal(t, 2, cfg.MultiLangWords)
	assert.Equal(t, 1, cfg.FirstMessagesCount)
	assert.Equal(t, tgspam.LLMMode("flagged"), cfg.LLMMode)
	assert.Equal(t, tgspam.LLMConsensusMode("any"), cfg.LLMConsensus)
	assert.Equal(t, "https://api.cas.chat", cfg.CasAPI)
}

func TestBuildDetectorConfig_CasDisabledClearsAPI(t *testing.T) {
	var opts options
	opts.CAS.API = "https://api.cas.chat"
	rs := rules.RuleSet{Detection: rules.DetectionRules{CasEnabled: false}}

	cfg := buildDetectorConfig(opts, rs)
	assert.Empty(t, cfg.CasAPI, "CAS API must be cleared when CasEnabled is false")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestBuildDetectorConfig -v`
Expected: FAIL — `cfg.MaxAllowedEmoji` is 999 (still from opts), `cfg.CasAPI` set when disabled.

- [ ] **Step 3: Write minimal implementation**

In `app/assembly.go`, replace the body of `buildDetectorConfig`. The fields below change source from `opts` to `ruleSet`:

```go
func buildDetectorConfig(opts options, ruleSet rules.RuleSet) tgspam.Config {
	casAPI := ""
	if ruleSet.Detection.CasEnabled {
		casAPI = opts.CAS.API
	}
	cfg := tgspam.Config{
		MaxAllowedEmoji:     ruleSet.Detection.MaxEmoji,
		MinMsgLen:           ruleSet.Detection.MinMsgLen,
		SimilarityThreshold: ruleSet.Detection.SimilarityThreshold,
		MinSpamProbability:  ruleSet.Detection.MinSpamProbability,
		CasAPI:              casAPI,
		CasUserAgent:        opts.CAS.UserAgent,
		HTTPClient:          &http.Client{Timeout: opts.CAS.Timeout},
		FirstMessageOnly:    !ruleSet.Detection.ParanoidMode,
		FirstMessagesCount:  ruleSet.Detection.FirstMessagesCount,
		OpenAIVeto:          ruleSet.OpenAI.Veto,
		OpenAIHistorySize:   ruleSet.OpenAI.HistorySize,
		GeminiVeto:          ruleSet.Gemini.Veto,
		GeminiHistorySize:   ruleSet.Gemini.HistorySize,
		LLMMode:             tgspam.LLMMode(ruleSet.LLM.Mode),
		LLMConsensus:        tgspam.LLMConsensusMode(ruleSet.LLM.Consensus),
		LLMRequestTimeout:   opts.LLM.RequestTimeout,
		MultiLangWords:      ruleSet.Detection.MultiLangWords,
		HistorySize:         ruleSet.Detection.HistorySize,
	}

	if ruleSet.Detection.FirstMessagesCount > 0 {
		cfg.FirstMessageOnly = true
	}
	if ruleSet.Detection.ParanoidMode {
		cfg.FirstMessageOnly = false
		cfg.FirstMessagesCount = 0
	}
	if opts.StorageTimeout > 0 {
		cfg.StorageTimeout = opts.StorageTimeout
	}

	cfg.DuplicateDetection.Threshold = ruleSet.Duplicates.Threshold
	cfg.DuplicateDetection.Window = ruleSet.Duplicates.Window

	cfg.AbnormalSpacing.Enabled = ruleSet.AbnormalSpacing.Enabled
	cfg.AbnormalSpacing.ShortWordLen = ruleSet.AbnormalSpacing.ShortWordLen
	cfg.AbnormalSpacing.ShortWordRatioThreshold = ruleSet.AbnormalSpacing.ShortWordRatioThreshold
	cfg.AbnormalSpacing.SpaceRatioThreshold = ruleSet.AbnormalSpacing.SpaceRatioThreshold
	cfg.AbnormalSpacing.MinWordsCount = ruleSet.AbnormalSpacing.MinWords

	return cfg
}
```

Note: `buildMetaChecks` is called with `opts.MinMsgLen` at `app/assembly.go:296` and `app/runtime_assembly.go:495`. Change both call sites to pass `ruleSet.Detection.MinMsgLen` (or `activeRuleSet.Detection.MinMsgLen` / `rs.Detection.MinMsgLen` respectively).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestBuildDetectorConfig -v`
Expected: PASS (both tests).

- [ ] **Step 5: Run the full app build to catch the `buildMetaChecks` call-site changes**

Run: `go build ./...`
Expected: builds clean. If `buildMetaChecks` call sites were missed, fix them to pass `ruleSet.Detection.MinMsgLen`.

- [ ] **Step 6: Commit**

```bash
git add app/assembly.go app/runtime_assembly.go app/assembly_test.go
git commit -m "feat(config): source detection tuning from RuleSet in buildDetectorConfig"
```

---

## Task 5: Treat an empty env value as not-set in `configured`

**Files:**
- Modify: `app/settings_precedence.go` (`configured`, lines 161-166)
- Test: `app/settings_precedence_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `app/settings_precedence_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigured_EmptyEnvIsNotSet(t *testing.T) {
	t.Setenv("MAX_EMOJI", "")
	assert.False(t, configured("max-emoji", "MAX_EMOJI"),
		"an env var present but empty must count as not configured")
}

func TestConfigured_NonEmptyEnvIsSet(t *testing.T) {
	t.Setenv("MAX_EMOJI", "5")
	assert.True(t, configured("max-emoji", "MAX_EMOJI"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestConfigured -v`
Expected: FAIL on `TestConfigured_EmptyEnvIsNotSet` — current code returns true for a present-but-empty var.

- [ ] **Step 3: Write minimal implementation**

In `app/settings_precedence.go`, change `configured`:

```go
func configured(flagName, envName string) bool {
	if v, ok := os.LookupEnv(envName); ok && v != "" {
		return true
	}
	return cliFlagSet(flagName)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestConfigured -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add app/settings_precedence.go app/settings_precedence_test.go
git commit -m "fix(config): treat an empty env value as not configured"
```

---

## Task 6: Apply explicit env overrides for the new RuleSet fields

**Files:**
- Modify: `app/settings_precedence.go`
- Test: `app/settings_precedence_test.go`

- [ ] **Step 1: Write the failing test**

Append to `app/settings_precedence_test.go`:

```go
func TestApplyExplicitOverrides_DetectionFields(t *testing.T) {
	t.Setenv("MAX_EMOJI", "9")
	t.Setenv("LLM_MODE", "always")

	var opts options
	opts.MaxEmoji = 9
	opts.LLM.Mode = "always"

	rs := rules.RuleSet{
		Detection: rules.DetectionRules{MaxEmoji: 2},
		LLM:       rules.LLMCommonRules{Mode: "flagged"},
	}
	applyExplicitRuleSetOverrides(&rs, opts)

	assert.Equal(t, 9, rs.Detection.MaxEmoji, "MAX_EMOJI env must override the ruleset")
	assert.Equal(t, "always", rs.LLM.Mode, "LLM_MODE env must override the ruleset")
}

func TestApplyExplicitOverrides_DetectionFieldsUntouchedWithoutEnv(t *testing.T) {
	var opts options
	opts.MaxEmoji = 9

	rs := rules.RuleSet{Detection: rules.DetectionRules{MaxEmoji: 2}}
	applyExplicitRuleSetOverrides(&rs, opts)

	assert.Equal(t, 2, rs.Detection.MaxEmoji, "no env set: ruleset value must be kept")
}
```

Add `"github.com/redstone-md/shield/app/rules"` to the test file imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestApplyExplicitOverrides -v`
Expected: FAIL — `applyExplicitRuleSetOverrides` does not touch `Detection`/`LLM` yet.

- [ ] **Step 3: Write minimal implementation**

In `app/settings_precedence.go`, add a new override function and call it from `applyExplicitRuleSetOverrides`:

```go
func applyExplicitRuleSetOverrides(rs *rules.RuleSet, opts options) {
	applyExplicitMetaOverrides(rs, opts)
	applyExplicitDuplicateOverrides(rs, opts)
	applyExplicitSpacingOverrides(rs, opts)
	applyExplicitModerationOverrides(rs, opts)
	applyExplicitReportOverrides(rs, opts)
	applyExplicitLLMOverrides(rs, opts)
	applyExplicitDetectionOverrides(rs, opts)
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestApplyExplicitOverrides -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add app/settings_precedence.go app/settings_precedence_test.go
git commit -m "feat(config): apply explicit env overrides for new detection fields"
```

---

## Task 7: Report env-pinned setting keys

**Files:**
- Modify: `app/settings_precedence.go`
- Test: `app/settings_precedence_test.go`

- [ ] **Step 1: Write the failing test**

Append to `app/settings_precedence_test.go`:

```go
func TestEnvPinnedKeys(t *testing.T) {
	t.Setenv("MAX_EMOJI", "5")
	t.Setenv("OPENAI_VETO", "true")

	pinned := envPinnedKeys()

	assert.True(t, pinned["detection.max_emoji"], "MAX_EMOJI must be reported as pinned")
	assert.True(t, pinned["openai.veto"], "OPENAI_VETO must be reported as pinned")
	assert.False(t, pinned["detection.min_msg_len"], "unset MIN_MSG_LEN must not be pinned")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestEnvPinnedKeys -v`
Expected: FAIL — `envPinnedKeys` undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/settings_precedence.go`, add a key→(flag,env) table and the reporting function. The map key is the JSON path the future UI uses to match a rendered field:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestEnvPinnedKeys -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/settings_precedence.go app/settings_precedence_test.go
git commit -m "feat(config): report env-pinned setting keys for the UI"
```

---

## Task 8: Backfill rulesets that predate the new schema

**Files:**
- Modify: `app/runtime_assembly.go` (after `ruleSets.Active`, ~line 108-112)
- Test: `app/runtime_assembly_test.go`

- [ ] **Step 1: Write the failing test**

Add to `app/runtime_assembly_test.go`:

```go
func TestBackfillRuleSetSchema_OldRuleSet(t *testing.T) {
	var opts options
	opts.InstanceID = "tg-spam"
	opts.MaxEmoji = 4
	opts.MinMsgLen = 50
	opts.LLM.Mode = "flagged"

	// a ruleset persisted before the new fields existed
	old := rules.RuleSet{WorkspaceID: "tg-spam", Version: 1, SchemaVersion: 0}

	got, changed := backfillRuleSetSchema(old, opts)

	assert.True(t, changed, "an old ruleset must be reported as changed")
	assert.Equal(t, rules.CurrentSchemaVersion, got.SchemaVersion)
	assert.Equal(t, 4, got.Detection.MaxEmoji)
	assert.Equal(t, 50, got.Detection.MinMsgLen)
	assert.Equal(t, "flagged", got.LLM.Mode)
	assert.Equal(t, 1, got.Version, "version and identity fields must be preserved")
}

func TestBackfillRuleSetSchema_CurrentRuleSetUnchanged(t *testing.T) {
	var opts options
	opts.InstanceID = "tg-spam"

	current := rules.RuleSet{
		WorkspaceID:   "tg-spam",
		Version:       5,
		SchemaVersion: rules.CurrentSchemaVersion,
		Detection:     rules.DetectionRules{MaxEmoji: 2},
	}
	got, changed := backfillRuleSetSchema(current, opts)

	assert.False(t, changed, "a current-schema ruleset must not be changed")
	assert.Equal(t, current, got)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestBackfillRuleSetSchema -v`
Expected: FAIL — `backfillRuleSetSchema` undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/runtime_assembly.go`, add the function:

```go
// backfillRuleSetSchema seeds new fields into a ruleset persisted before the
// current schema version. Identity and versioning fields are preserved; only the
// detection/LLM/prompt fields are seeded from env-derived defaults. The second
// return value reports whether the ruleset was modified.
func backfillRuleSetSchema(rs rules.RuleSet, opts options) (rules.RuleSet, bool) {
	if rs.SchemaVersion >= rules.CurrentSchemaVersion {
		return rs, false
	}
	seed := bootstrapRuleSet(opts)
	rs.SchemaVersion = rules.CurrentSchemaVersion
	rs.Detection = seed.Detection
	rs.LLM = seed.LLM
	rs.OpenAI.Prompt = seed.OpenAI.Prompt
	rs.OpenAI.VisionModel = seed.OpenAI.VisionModel
	rs.Gemini.Prompt = seed.Gemini.Prompt
	rs.Gemini.VisionModel = seed.Gemini.VisionModel
	return rs, true
}
```

Then wire it into `Build` (the assembly function) right after `activeRuleSet` is loaded. Replace lines 108-112:

```go
	activeRuleSet, err := ruleSets.Active(ctx, opts.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("can't load active rule set, %w", err)
	}
	if backfilled, changed := backfillRuleSetSchema(activeRuleSet, opts); changed {
		log.Printf("[INFO] backfilling rule set schema to version %d", rules.CurrentSchemaVersion)
		if _, err = ruleSets.Update(ctx, backfilled); err != nil {
			return nil, fmt.Errorf("can't backfill rule set schema, %w", err)
		}
		activeRuleSet = backfilled
	}
	applyExplicitRuleSetOverrides(&activeRuleSet, opts)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/ -run TestBackfillRuleSetSchema -v`
Expected: PASS (both tests).

- [ ] **Step 5: Run the full build**

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 6: Commit**

```bash
git add app/runtime_assembly.go app/runtime_assembly_test.go
git commit -m "feat(config): backfill new ruleset fields for pre-schema rulesets"
```

---

## Task 9: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Run the whole test suite with the race detector**

Run: `go test -race ./...`
Expected: all packages PASS.

- [ ] **Step 2: Run the linter**

Run: `golangci-lint run`
Expected: no new findings in `app/rules`, `app/`, `app/settings_precedence.go`.

- [ ] **Step 3: Normalize comments**

Run: `make unfuck-ai-comments`
Expected: no diff, or only intended lowercase fixes — review and re-commit if it changes files.

- [ ] **Step 4: Commit any normalization changes**

```bash
git add -A
git commit -m "chore: normalize comments"
```

(Skip this commit if Step 3 produced no changes.)

---

## Self-Review notes

- **Spec coverage:** Part 2a (RuleSet extension) — Tasks 1-4. Part 2b (precedence: empty-env fix, env-pinned reporting) — Tasks 5-7. Migration backfill — Task 8. Part 2c prompt *fields* — Tasks 1, 3 (`LLMRules.Prompt`/`VisionModel`). Prompt *hot-reload wiring into live checkers* and vision-prompt *persistence* (InMemory→File registry) are deliberately deferred to Plan 2 — the spec's Part 2c open item; this plan only adds the storage fields.
- **Out of this plan:** the editable web UI (Plan 3) consumes `envPinnedKeys()` and the new fields via `PUT /api/rules/`.
- **Type consistency:** `DetectionRules`, `LLMCommonRules`, `CurrentSchemaVersion`, `backfillRuleSetSchema`, `envPinnedKeys`, `applyExplicitDetectionOverrides`, `envPinnedRegistry` are used consistently across tasks.
