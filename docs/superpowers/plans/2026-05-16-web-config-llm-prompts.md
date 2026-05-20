# Web Config — LLM Prompts (Plan 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make the OpenAI/Gemini text system prompt and the slowpath vision prompt editable through the `RuleSet`, remove the dead `PromptRegistry`, and hot-reload prompt/model changes without a restart.

**Architecture:** A single shared `RuleSet.LLM.VisionPrompt` and the per-provider `RuleSet.OpenAI.Prompt` / `RuleSet.Gemini.Prompt` (added in Plan 1) become the prompt sources. The slowpath `Engine` holds the prompts in plain fields/maps set via `SetVisionPrompt` / `SetSystemPrompt`; `PromptRegistry` and friends are deleted. The OpenAI/Gemini text checker construction is extracted into `applyLLMCheckers`, called both at assembly time and from `wireLiveReload`, so prompt/model edits apply live.

**Tech Stack:** Go 1.24, `stretchr/testify`, `sashabaranov/go-openai`, `google.golang.org/genai`.

**Depends on:** Plan 1 (`feat/web-config-backend`). This plan's branch `feat/web-config-llm-prompts` is stacked on it.

---

## File Structure

- `app/rules/ruleset.go` — add `LLMCommonRules.VisionPrompt`; bump `CurrentSchemaVersion` to 2.
- `app/slowpath/engine.go` — add `visionPrompt` field + `systemPrompts` map + `SetVisionPrompt`/`SetSystemPrompt`; `checkVision` and `resolvePrompt` use them; remove `registry` field + `SetPromptRegistry`.
- `app/slowpath/interfaces.go` — remove `PromptRegistry` interface.
- `app/slowpath/prompt_registry.go` — delete file.
- `app/slowpath/prompt_registry_test.go` — delete file (if present).
- `app/slowpath/types.go` — remove `PromptEntry` struct.
- `app/assembly.go` — replace `configureSlowPathPrompts` with `applySlowPathPrompts(eng, ruleSet)`; `makeSlowPathEngine` takes a `ruleSet` param; extract `applyLLMCheckers(detector, opts, ruleSet)`.
- `app/runtime_assembly.go` — update `makeSlowPathEngine` caller; `wireLiveReload` rebuilds checkers + re-pushes slowpath prompts.

---

## Task 1: Add `LLMCommonRules.VisionPrompt`, bump schema version

**Files:**
- Modify: `app/rules/ruleset.go`
- Test: `app/rules/ruleset_test.go`

- [ ] **Step 1: Write the failing test**

Append to `app/rules/ruleset_test.go`:

```go
func TestRuleSet_VisionPromptJSONRoundTrip(t *testing.T) {
	rs := RuleSet{LLM: LLMCommonRules{Mode: "flagged", VisionPrompt: "scan this image"}}

	data, err := json.Marshal(rs)
	require.NoError(t, err)

	var got RuleSet
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "scan this image", got.LLM.VisionPrompt)
}
```

Also update the existing `TestCurrentSchemaVersion` test — change its expected value from `1` to `2`:

```go
func TestCurrentSchemaVersion(t *testing.T) {
	assert.Equal(t, 2, CurrentSchemaVersion)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/rules/ -run 'TestRuleSet_VisionPrompt|TestCurrentSchemaVersion' -v`
Expected: FAIL — `VisionPrompt` undefined, `CurrentSchemaVersion` still 1.

- [ ] **Step 3: Write minimal implementation**

In `app/rules/ruleset.go`, add `VisionPrompt` to `LLMCommonRules`:

```go
// LLMCommonRules holds LLM settings shared across providers.
type LLMCommonRules struct {
	Mode         string `json:"mode"`      // "" | missed | flagged | always
	Consensus    string `json:"consensus"` // any | all
	VisionPrompt string `json:"vision_prompt"`
}
```

Change the `CurrentSchemaVersion` constant from `1` to `2`:

```go
// CurrentSchemaVersion is the RuleSet payload schema version. Bump it whenever new
// fields are added so older persisted rulesets can be detected and backfilled.
const CurrentSchemaVersion = 2
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/rules/ -v`
Expected: PASS (all `app/rules` tests).

- [ ] **Step 5: Commit**

```bash
git add app/rules/ruleset.go app/rules/ruleset_test.go
git commit -m "feat(rules): add LLM vision prompt field, bump schema to 2"
```

---

## Task 2: Slowpath engine — configurable vision prompt

**Files:**
- Modify: `app/slowpath/engine.go`
- Test: `app/slowpath/engine_test.go` (add test to the existing file)

- [ ] **Step 1: Write the failing test**

Add to `app/slowpath/engine_test.go`:

```go
func TestEngine_SetVisionPrompt(t *testing.T) {
	e := NewEngine(EngineConfig{})
	assert.Equal(t, "", e.visionPromptOrDefault("")) // sanity: helper exists

	e.SetVisionPrompt("custom vision prompt")
	assert.Equal(t, "custom vision prompt", e.visionPromptOrDefault(""))
}

func TestEngine_VisionPromptFallsBackToDefault(t *testing.T) {
	e := NewEngine(EngineConfig{})
	assert.Equal(t, defaultVisionPrompt, e.visionPromptOrDefault(defaultVisionPrompt))
}
```

Note: `visionPromptOrDefault` is a tiny helper added in Step 3 purely so the field is testable. If the existing `engine_test.go` lacks the testify import, add `"github.com/stretchr/testify/assert"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/slowpath/ -run TestEngine_SetVisionPrompt -v`
Expected: FAIL — `SetVisionPrompt` / `visionPromptOrDefault` / `visionPrompt` undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/slowpath/engine.go`, add a `visionPrompt` field to the `Engine` struct:

```go
type Engine struct {
	providers map[string]LLMProvider
	chat      map[string]ChatProvider
	vision    map[string]VisionProvider
	breakers  map[string]*ProviderBreaker
	budget    BudgetTracker
	systemPrompts map[string]string
	visionPrompt  string
	config    EngineConfig
}
```

In `NewEngine`, initialise the map:

```go
func NewEngine(cfg EngineConfig) *Engine {
	return &Engine{
		providers:     make(map[string]LLMProvider),
		chat:          make(map[string]ChatProvider),
		vision:        make(map[string]VisionProvider),
		breakers:      make(map[string]*ProviderBreaker),
		systemPrompts: make(map[string]string),
		config:        cfg,
	}
}
```

Add the setter and helper (place near `SetBudgetTracker`):

```go
// SetVisionPrompt sets the system prompt used for vision (image) checks.
func (e *Engine) SetVisionPrompt(prompt string) { e.visionPrompt = prompt }

// visionPromptOrDefault returns the configured vision prompt, or fallback when unset.
func (e *Engine) visionPromptOrDefault(fallback string) string {
	if e.visionPrompt != "" {
		return e.visionPrompt
	}
	return fallback
}
```

In `checkVision`, replace the hardcoded line `prompt := defaultVisionPrompt` with:

```go
	prompt := e.visionPromptOrDefault(defaultVisionPrompt)
```

(`systemPrompts` is added now but used in Task 3 — adding it here keeps `NewEngine` edited once.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/slowpath/ -run TestEngine_SetVisionPrompt -v` and `go test ./app/slowpath/ -run TestEngine_VisionPrompt -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/slowpath/engine.go app/slowpath/engine_test.go
git commit -m "feat(slowpath): configurable vision prompt on Engine"
```

---

## Task 3: Slowpath engine — replace PromptRegistry with system-prompt map

**Files:**
- Modify: `app/slowpath/engine.go`
- Test: `app/slowpath/engine_test.go`

- [ ] **Step 1: Write the failing test**

Add to `app/slowpath/engine_test.go`:

```go
func TestEngine_SetSystemPrompt(t *testing.T) {
	e := NewEngine(EngineConfig{})

	e.SetSystemPrompt("openai", "openai system prompt")
	system, customs, ver, err := e.resolvePrompt("openai", "v1")

	require.NoError(t, err)
	assert.Equal(t, "openai system prompt", system)
	assert.Nil(t, customs)
	assert.Equal(t, "v1", ver)
}

func TestEngine_ResolvePromptUnknownProvider(t *testing.T) {
	e := NewEngine(EngineConfig{})
	system, _, _, err := e.resolvePrompt("missing", "")
	require.NoError(t, err)
	assert.Equal(t, "", system)
}
```

Add `"github.com/stretchr/testify/require"` to the test imports if absent.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/slowpath/ -run TestEngine_SetSystemPrompt -v`
Expected: FAIL — `SetSystemPrompt` undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/slowpath/engine.go`:

Remove the `registry PromptRegistry` field from the `Engine` struct (the `systemPrompts` field added in Task 2 replaces it).

Remove the `SetPromptRegistry` method:

```go
func (e *Engine) SetPromptRegistry(pr PromptRegistry) { e.registry = pr }
```

Add `SetSystemPrompt` (place next to `SetVisionPrompt`):

```go
// SetSystemPrompt sets the system prompt used for text checks of the given provider.
func (e *Engine) SetSystemPrompt(provider, prompt string) { e.systemPrompts[provider] = prompt }
```

Replace the body of `resolvePrompt` entirely:

```go
func (e *Engine) resolvePrompt(provider, version string) (system string, customs []string, ver string, err error) {
	return e.systemPrompts[provider], nil, version, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/slowpath/ -run TestEngine_SetSystemPrompt -v` and `go test ./app/slowpath/ -run TestEngine_ResolvePrompt -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/slowpath/engine.go app/slowpath/engine_test.go
git commit -m "feat(slowpath): replace PromptRegistry with system-prompt map on Engine"
```

---

## Task 4: Delete the dead PromptRegistry types

**Files:**
- Modify: `app/slowpath/interfaces.go` (remove `PromptRegistry` interface)
- Delete: `app/slowpath/prompt_registry.go`
- Delete: `app/slowpath/prompt_registry_test.go` (if it exists)
- Modify: `app/slowpath/types.go` (remove `PromptEntry` struct)

- [ ] **Step 1: Confirm nothing else references the types**

Run: `grep -rn "PromptRegistry\|PromptEntry\|FilePromptRegistry\|InMemoryPromptRegistry\|NewFilePromptRegistry\|NewInMemoryPromptRegistry" app/ --include=*.go`
Expected: matches only in `app/slowpath/interfaces.go`, `app/slowpath/prompt_registry.go`, `app/slowpath/prompt_registry_test.go`, `app/slowpath/types.go`, and `app/assembly.go` (`configureSlowPathPrompts`, handled in Task 5).

If any other file references them, STOP and report BLOCKED with the locations.

- [ ] **Step 2: Delete and remove**

```bash
git rm app/slowpath/prompt_registry.go
git rm app/slowpath/prompt_registry_test.go
```

(If `prompt_registry_test.go` does not exist, skip that line.)

In `app/slowpath/interfaces.go`, delete the `PromptRegistry` interface block (the `type PromptRegistry interface { ... }` declaration and its doc comment).

In `app/slowpath/types.go`, delete the `PromptEntry` struct declaration and its doc comment.

- [ ] **Step 3: Verify the slowpath package still builds (except the assembly caller)**

Run: `go build ./app/slowpath/...`
Expected: builds clean. `app/assembly.go` still references the deleted types — that is fixed in Task 5; do NOT touch it here.

Run: `go test ./app/slowpath/ 2>&1 | tail -5`
Expected: slowpath package tests pass.

- [ ] **Step 4: Commit**

```bash
git add -A app/slowpath/
git commit -m "refactor(slowpath): delete unused PromptRegistry and PromptEntry"
```

---

## Task 5: Assembly — slowpath prompts from the ruleset

**Files:**
- Modify: `app/assembly.go` (`makeSlowPathEngine`, `configureSlowPathPrompts`)
- Modify: `app/runtime_assembly.go` (`makeSlowPathEngine` caller, ~line 115)
- Test: `app/assembly_test.go` or `app/main_runtime_assembly_test.go` (add test, package `main`)

- [ ] **Step 1: Write the failing test**

Add to the assembly test file (package `main`):

```go
func TestApplySlowPathPrompts_FromRuleSet(t *testing.T) {
	eng := slowpath.NewEngine(slowpath.EngineConfig{})
	rs := rules.RuleSet{
		OpenAI: rules.LLMRules{Prompt: "openai sys"},
		Gemini: rules.LLMRules{Prompt: "gemini sys"},
		LLM:    rules.LLMCommonRules{VisionPrompt: "vision sys"},
	}

	applySlowPathPrompts(eng, rs)

	system, _, _, err := slowpath.ExportResolvePrompt(eng, "openai")
	require.NoError(t, err)
	assert.Equal(t, "openai sys", system)
	assert.Equal(t, "vision sys", slowpath.ExportVisionPrompt(eng))
}
```

This test needs two tiny test-only exporters in the slowpath package (added in Step 3) because `resolvePrompt`/`visionPrompt` are unexported. If you prefer, instead assert behaviourally — but the exporters keep the test simple.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestApplySlowPathPrompts -v`
Expected: FAIL — `applySlowPathPrompts` and the exporters undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/slowpath/`, add a test-support file `app/slowpath/export_test_support.go`:

```go
package slowpath

// ExportResolvePrompt exposes resolvePrompt for tests in other packages.
func ExportResolvePrompt(e *Engine, provider string) (string, []string, string, error) {
	return e.resolvePrompt(provider, "")
}

// ExportVisionPrompt exposes the configured vision prompt for tests in other packages.
func ExportVisionPrompt(e *Engine) string { return e.visionPrompt }
```

In `app/assembly.go`, replace `configureSlowPathPrompts(eng *slowpath.Engine, opts options)` with:

```go
func applySlowPathPrompts(eng *slowpath.Engine, ruleSet rules.RuleSet) {
	if eng == nil {
		return
	}
	eng.SetSystemPrompt("openai", ruleSet.OpenAI.Prompt)
	eng.SetSystemPrompt("gemini", ruleSet.Gemini.Prompt)
	eng.SetVisionPrompt(ruleSet.LLM.VisionPrompt)
}
```

Delete the now-unused helpers `readPromptOverride` and `resolveSlowPathPrompt` from `app/assembly.go` IF they are no longer referenced anywhere (run `grep -rn "readPromptOverride\|resolveSlowPathPrompt" app/ --include=*.go` to confirm; if still used elsewhere, leave them).

Change `makeSlowPathEngine` to take a ruleset and call the new function:

```go
func makeSlowPathEngine(opts options, ruleSet rules.RuleSet) *slowpath.Engine {
```

and replace the `configureSlowPathPrompts(eng, opts)` call inside it with:

```go
	applySlowPathPrompts(eng, ruleSet)
```

In `app/runtime_assembly.go`, update the caller (~line 115) from `makeSlowPathEngine(opts)` to `makeSlowPathEngine(opts, activeRuleSet)`. Read the surrounding code to confirm `activeRuleSet` is the ruleset variable in scope at that point (it is — loaded just above).

- [ ] **Step 4: Run test and build**

Run: `go test ./app/ -run TestApplySlowPathPrompts -v`
Expected: PASS.
Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add app/assembly.go app/runtime_assembly.go app/slowpath/export_test_support.go <test file>
git commit -m "feat(slowpath): source slowpath prompts from the ruleset"
```

---

## Task 6: Assembly — extract `applyLLMCheckers`

**Files:**
- Modify: `app/assembly.go` (`makeDetectorWithRuleSet`)
- Test: `app/assembly_test.go` or `app/main_runtime_assembly_test.go`

- [ ] **Step 1: Write the failing test**

Add to the assembly test file (package `main`):

```go
func TestApplyLLMCheckers_NoLLMConfigured(t *testing.T) {
	// no tokens configured -> applyLLMCheckers must be a safe no-op
	var opts options
	detector := tgspam.NewDetector(tgspam.Config{})
	rs := rules.RuleSet{}

	assert.NotPanics(t, func() { applyLLMCheckers(detector, opts, rs) })
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestApplyLLMCheckers -v`
Expected: FAIL — `applyLLMCheckers` undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/assembly.go`, extract the OpenAI and Gemini checker construction blocks from `makeDetectorWithRuleSet` into a new function. The current blocks in `makeDetectorWithRuleSet` are the `if ruleSet.OpenAI.Enabled && ...` block and the `if ruleSet.Gemini.Enabled && ...` block. Move them verbatim into:

```go
// applyLLMCheckers (re)builds and attaches the OpenAI and Gemini text checkers on the
// detector from the current ruleset. Safe to call repeatedly for live reload.
func applyLLMCheckers(detector *tgspam.Detector, opts options, ruleSet rules.RuleSet) {
	if ruleSet.OpenAI.Enabled && (opts.OpenAI.Token != "" || opts.OpenAI.APIBase != "") {
		openAIConfig := tgspam.OpenAIConfig{
			SystemPrompt:                 ruleSet.OpenAI.Prompt,
			CustomPrompts:                ruleSet.OpenAI.CustomPrompts,
			Model:                        ruleSet.OpenAI.Model,
			MaxTokensResponse:            opts.OpenAI.MaxTokensResponse,
			MaxTokensRequest:             opts.OpenAI.MaxTokensRequest,
			MaxSymbolsRequest:            opts.OpenAI.MaxSymbolsRequest,
			RetryCount:                   opts.OpenAI.RetryCount,
			ReasoningEffort:              opts.OpenAI.ReasoningEffort,
			CheckShortMessagesWithOpenAI: ruleSet.OpenAI.CheckShortMessages,
		}
		config := openai.DefaultConfig(opts.OpenAI.Token)
		if opts.OpenAI.APIBase != "" {
			config.BaseURL = opts.OpenAI.APIBase
		}
		debugLogFields("openai config", openAIConfig)
		detector.WithOpenAIChecker(openai.NewClientWithConfig(config), openAIConfig)
	}

	if ruleSet.Gemini.Enabled && opts.Gemini.Token != "" {
		geminiConfig := tgspam.GeminiConfig{
			SystemPrompt:       ruleSet.Gemini.Prompt,
			CustomPrompts:      ruleSet.Gemini.CustomPrompts,
			Model:              ruleSet.Gemini.Model,
			MaxOutputTokens:    opts.Gemini.MaxTokensResponse,
			MaxSymbolsRequest:  opts.Gemini.MaxSymbolsRequest,
			RetryCount:         opts.Gemini.RetryCount,
			CheckShortMessages: ruleSet.Gemini.CheckShortMessages,
		}
		client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
			APIKey:  opts.Gemini.Token,
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			log.Printf("[ERROR] failed to create gemini client: %v", err)
			return
		}
		debugLogFields("gemini config", geminiConfig)
		detector.WithGeminiChecker(client.Models, geminiConfig)
	}
}
```

Note: `SystemPrompt` and `CustomPrompts` now come from `ruleSet.OpenAI.Prompt`/`ruleSet.OpenAI.CustomPrompts` (previously `opts.OpenAI.Prompt`/`opts.OpenAI.CustomPrompts`). The Gemini-client error path uses `log.Printf` + `return` instead of the original `log.Fatalf`, because this function is now also called during live reload where a fatal exit is unacceptable.

In `makeDetectorWithRuleSet`, replace the two removed blocks with a single call:

```go
	applyLLMCheckers(detector, opts, ruleSet)
```

placed where the OpenAI block used to start (before `detector.WithMetaChecks(...)`).

- [ ] **Step 4: Run test and build**

Run: `go test ./app/ -run TestApplyLLMCheckers -v`
Expected: PASS.
Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add app/assembly.go <test file>
git commit -m "refactor(assembly): extract applyLLMCheckers for reuse on live reload"
```

---

## Task 7: Hot-reload LLM checkers and slowpath prompts

**Files:**
- Modify: `app/runtime_assembly.go` (`wireLiveReload`, ~line 484-505)
- Test: `app/runtime_assembly_test.go`

- [ ] **Step 1: Write the failing test**

Add to `app/runtime_assembly_test.go`:

```go
func TestWireLiveReload_AppliesLLMAndSlowPathPrompts(t *testing.T) {
	var opts options
	opts.InstanceID = "tg-spam"

	a := &runtimeAssembly{
		Detector:       tgspam.NewDetector(tgspam.Config{}),
		SlowPathEngine: slowpath.NewEngine(slowpath.EngineConfig{}),
	}
	rs := rules.RuleSet{LLM: rules.LLMCommonRules{VisionPrompt: "live vision prompt"}}

	applyLiveReload(a, opts, rs)

	assert.Equal(t, "live vision prompt", slowpath.ExportVisionPrompt(a.SlowPathEngine))
}
```

This test calls `applyLiveReload` — a named function extracted from the `wireLiveReload` `OnChange` closure so it is unit-testable.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/ -run TestWireLiveReload_AppliesLLMAndSlowPathPrompts -v`
Expected: FAIL — `applyLiveReload` undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/runtime_assembly.go`, read the current `wireLiveReload` body. It registers an `OnChange` closure that calls `applyExplicitRuleSetOverrides`, `TelegramListener.ApplyRuleSet`, `Detector.UpdateConfig`, `Detector.ReplaceMetaChecks`, `SpamBot.ApplyRuleSet`, and sets `a.ActiveRuleSet`.

Extract the closure body into a named method/function `applyLiveReload(a *runtimeAssembly, opts options, rs rules.RuleSet)` that performs exactly what the closure currently does, and have the `OnChange` closure simply call it. Then add two new actions inside `applyLiveReload`, after the existing `Detector` block:

```go
	if a.Detector != nil {
		applyLLMCheckers(a.Detector, opts, rs)
	}
	if a.SlowPathEngine != nil {
		applySlowPathPrompts(a.SlowPathEngine, rs)
	}
```

The resulting `wireLiveReload` should look like:

```go
func (a *runtimeAssembly) wireLiveReload(opts options) {
	a.RuleSetService.OnChange(func(rs rules.RuleSet) {
		applyLiveReload(a, opts, rs)
	})

	if a.ApprovedUsersService != nil {
		// ... existing unchanged code ...
	}
}
```

and `applyLiveReload` contains the former closure body plus the two new blocks above. Preserve the existing log lines (`rule set changed`, `live reload applied`).

- [ ] **Step 4: Run test and build**

Run: `go test ./app/ -run TestWireLiveReload -v`
Expected: PASS.
Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add app/runtime_assembly.go app/runtime_assembly_test.go
git commit -m "feat(config): hot-reload LLM checkers and slowpath prompts on ruleset change"
```

---

## Task 8: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Build**

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 2: Tests**

Run: `go test ./app/ ./app/rules/ ./app/slowpath/ ./lib/... 2>&1 | tail -20`
Expected: all listed packages PASS. (Container-backed `app/storage` Postgres tests fail only when Docker is unavailable — that is environmental, not a regression.)

- [ ] **Step 3: Vet**

Run: `go vet ./app/ ./app/rules/ ./app/slowpath/`
Expected: clean.

- [ ] **Step 4: Lint**

Run: `golangci-lint run ./app/slowpath/... ./app/rules/...`
Expected: no findings (ignore Windows CRLF gofmt noise if it appears).

---

## Self-Review notes

- **Spec coverage:** Part 2c (prompts consolidation) — Tasks 1-7. `LLMRules.Prompt` was added in Plan 1; this plan adds `LLMCommonRules.VisionPrompt`, makes the engine consume both, removes `PromptRegistry`, and adds prompt hot-reload.
- **Schema bump:** `CurrentSchemaVersion` → 2. Plan 1's `backfillRuleSetSchema` already re-seeds `rs.LLM` wholesale, so a Plan-1-era ruleset (schema 1) is upgraded to include the empty `VisionPrompt` automatically.
- **Out of this plan:** the editable web UI (Plan 3); the slowpath `ChatRequest`/`Reply` path keeps using `resolvePrompt`, now backed by the system-prompt map.
- **Type consistency:** `applyLLMCheckers`, `applySlowPathPrompts`, `applyLiveReload`, `SetVisionPrompt`, `SetSystemPrompt`, `visionPromptOrDefault`, `ExportResolvePrompt`, `ExportVisionPrompt` used consistently across tasks.
