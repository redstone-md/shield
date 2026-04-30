# Plan: Slow Path LLM/Vision (Stage 6)

Date: 2026-04-28
Roadmap: `docs/plans/roadmap/06-slow-path-llm-and-vision.md`
Brainstorm: `20260428-slow-path-llm-vision.brainstorm.md`

## Completion Criteria

1. LLM/Vision not called on every message — only on explicit escalation reasons.
2. Every slow-path decision records model, provider, prompt version, latency, token usage, budget impact.
3. Disabling slow path leaves fast path + policy fully functional.
4. Both OpenAI and Gemini supported as text LLM providers.
5. Both OpenAI and Gemini supported as vision providers (image analysis).
6. Circuit breaker prevents cascading failures when provider degrades.
7. Budget enforcement limits per-tenant spend; fallback on exhaustion.

## Design Decisions

1. **New `app/slowpath/` package** — isolated, same pattern as `app/policy/`.
2. **`LLMProvider` interface** — adapters wrap existing `openAIChecker`/`geminiChecker` from `lib/tgspam/`.
3. **`VisionProvider` interface** — separate adapters using `go-openai` `ChatMessagePart` (image URL) and `genai.Blob` (inline data).
4. **`sony/gobreaker/v2`** — circuit breaker per provider. New dependency.
5. **In-memory budget tracker** — `golang.org/x/time/rate` for sliding window. Per-tenant counters.
6. **File-based prompt registry** — JSON files in config dir. Version + provider + active flag.
7. **Feature flag** — `RuleSet.SlowPathEnabled bool`. Default false = current inline behavior.
8. **Two-pass policy** — fast result → tentative action → slow result → merged → final action overrides.
9. **Backward compat** — `SlowPathEnabled=false` keeps LLM inline in `Detector.Check()`. No behavior change.

## Slice Order

### Slice 1: Types + interfaces
**Files**: `app/slowpath/types.go`, `app/slowpath/types_test.go`
- `EscalationReason` string enum: `ambiguous_fast`, `image_content`, `user_report`, `high_risk_policy`, `force_llm`
- `BudgetClass` string enum: `standard`, `elevated`, `critical`
- `ProviderRequest`: message, history, prompt version, image data + mime
- `ProviderResult`: spam, confidence, reason, token usage (input/output), latency, model, provider, raw response
- `LLMProvider` interface: `Name() string`, `Check(ctx, ProviderRequest) (*ProviderResult, error)`
- `VisionProvider` interface: `Name() string`, `AnalyzeImage(ctx, imageData, mime, prompt) (*ProviderResult, error)`
- `SlowPathRequest` struct: event ID, correlation ID, tenant ID, escalation reason, fast result, content, prompt version, budget class, image data + mime
- `SlowPathResult` struct: provider result, merged detection result, invocation record
- `SlowPathInvocation` struct: full audit record (model, provider, prompt version, tokens, latency, raw response, parsed result)
- Tests: struct construction, enum values

### Slice 2: OpenAI text adapter
**Files**: `app/slowpath/openai_adapter.go`, `app/slowpath/openai_adapter_test.go`
- `OpenAIAdapter` implements `LLMProvider`
- Wraps `openAIClient` interface (same as `lib/tgspam` uses)
- `Check()` — builds `ChatCompletionRequest` with system prompt from registry, sends via client
- Parses JSON response → `llmResponse` → `ProviderResult`
- Records token usage from `resp.Usage`
- Records latency via `time.Since(start)`
- Constructor: `NewOpenAIAdapter(client, model, maxTokens, maxSymbols)`
- Tests: mock client, verify spam/ham, token counting, latency recording

### Slice 3: Gemini text adapter
**Files**: `app/slowpath/gemini_adapter.go`, `app/slowpath/gemini_adapter_test.go`
- `GeminiAdapter` implements `LLMProvider`
- Wraps `geminiClient` interface (same as `lib/tgspam` uses)
- `Check()` — builds `GenerateContentConfig` with system prompt, sends via client
- Parses response → `ProviderResult`
- Records latency
- Constructor: `NewGeminiAdapter(client, model, maxOutputTokens, maxSymbols)`
- Tests: mock client, verify spam/ham, latency recording

### Slice 4: Budget tracker
**Files**: `app/slowpath/budget.go`, `app/slowpath/budget_test.go`
- `BudgetConfig`: max requests/hour, max tokens/hour, max cost/hour (USD)
- `budgetCounter`: request count, token count, cost estimate, reset time
- `BudgetTracker`: map[tenantID]→budgetCounter, map[tenantID]→BudgetConfig
- `BudgetTracker.Allow(tenantID) bool` — checks all limits
- `BudgetTracker.Record(tenantID, tokens, cost)` — post-call accounting
- `BudgetTracker.SetConfig(tenantID, BudgetConfig)` — runtime config update
- Default config: 100 req/hr, 100k tokens/hr, $1/hr
- Tests: allow within budget, reject over budget, reset after window, multiple tenants

### Slice 5: Circuit breaker
**Files**: `app/slowpath/breaker.go`, `app/slowpath/breaker_test.go`
- `ProviderBreaker` wraps `gobreaker.CircuitBreaker[*ProviderResult]`
- `ProviderBreaker.Execute(fn) (*ProviderResult, error)` — delegates to breaker
- Constructor: `NewProviderBreaker(name string, opts ...BreakerOption)`
- Default settings: 5 consecutive failures → open, 30s timeout → half-open, 3 max half-open requests
- `BreakerOption` functional options for customization
- State change logging
- Tests: open after failures, half-open after timeout, close after successes

### Slice 6: Prompt registry
**Files**: `app/slowpath/prompt_registry.go`, `app/slowpath/prompt_registry_test.go`
- `PromptEntry`: ID, version, provider, system prompt, custom prompts, created at, active
- `PromptRegistry`: in-memory map, loads from JSON file
- `LoadPrompts(path) error` — reads JSON file
- `GetActive(provider string) (*PromptEntry, error)` — returns active prompt for provider
- `GetVersion(provider string, version int) (*PromptEntry, error)` — specific version
- `Reload() error` — re-reads file (for config hot-reload)
- Default entry: built-in `defaultPrompt` as version 1 for both providers
- JSON format: `{ "prompts": [...] }`
- Tests: load, get active, fallback to default, reload

### Slice 7: Slow path engine
**Files**: `app/slowpath/engine.go`, `app/slowpath/engine_test.go`
- `Engine` struct: providers map[string]LLMProvider, visionProviders map[string]VisionProvider, budget *BudgetTracker, breakers map[string]*ProviderBreaker, registry *PromptRegistry
- `Engine.Check(ctx, SlowPathRequest) (*SlowPathResult, error)`
  - Resolve prompt from registry
  - Check budget → reject if exhausted
  - Select provider (from request or round-robin)
  - Execute via circuit breaker
  - Record budget usage
  - Build `SlowPathResult` with invocation record
- `Engine.AnalyzeImage(ctx, SlowPathRequest) (*SlowPathResult, error)` — vision path
  - Same flow but calls `VisionProvider.AnalyzeImage`
- Constructor: `NewEngine(opts ...EngineOption)` with functional options
- Tests: budget reject, breaker open, successful check, provider selection

### Slice 8: Vision adapters (OpenAI + Gemini)
**Files**: `app/slowpath/openai_vision.go`, `app/slowpath/openai_vision_test.go`, `app/slowpath/gemini_vision.go`, `app/slowpath/gemini_vision_test.go`
- `OpenAIVisionAdapter` implements `VisionProvider`
  - `AnalyzeImage()` — builds `ChatCompletionRequest` with `MultiContent` parts: text prompt + `ChatMessagePartTypeImageURL` with base64 data URL (`data:image/jpeg;base64,...`)
  - Parses same JSON response format
  - Model defaults to `gpt-4o-mini`
- `GeminiVisionAdapter` implements `VisionProvider`
  - `AnalyzeImage()` — builds `[]*genai.Content` with text + `InlineData` blob
  - Calls `GenerateContent`
  - Parses same response
  - Model defaults to `gemini-2.5-flash`
- Tests: mock clients, verify multimodal request structure, parse responses

### Slice 9: Merge logic
**Files**: `app/slowpath/merge.go`, `app/slowpath/merge_test.go`
- `MergeResults(fast DetectionResult, slow *SlowPathResult) DetectionResult`
  - Fast signals kept
  - Slow signal appended with provider name as `DetectionSignal.Name`
  - Score recalculated: `max(fast.Score, slow.Confidence/100)` if slow says spam, else `fast.Score * 0.8` (discount)
  - Spam flag: slow overrides if high confidence (>80%), otherwise fast prevails
  - Reason: concatenate fast + slow explanations
- Tests: slow confirms spam, slow overrides ham, low-confidence slow ignored, no slow result

### Slice 10: Wire into pipeline
**Files**: `app/events/pipeline.go`, `app/events/policy.go`, `app/events/listener.go`
- `TelegramListener` gets `slowPathEngine *slowpath.Engine` field
- `shouldEscalate(checkResults, req) (bool, EscalationReason)` — determines if slow path needed:
  - `req.ForceLLM` → `force_llm`
  - `req.Meta.Images > 0 || req.Meta.HasVideo` → `image_content`
  - Score in gray zone (within 10% of threshold) → `ambiguous_fast`
  - User report context → `user_report`
- When `SlowPathEnabled=true` and escalation triggered:
  - Build `SlowPathRequest`
  - Publish to slow queue
  - Apply tentative fast-path action
  - Slow worker picks up → `Engine.Check` → merge → override action if needed
- `applyRuleSet` recreates slow path engine when config changes
- Feature flag: `SlowPathEnabled` in `RuleSet`
- Tests: escalation decisions, queue publish, merge+override flow

### Slice 11: Audit integration
**Files**: `app/events/audit_writer.go`, `app/slowpath/types.go` (extend)
- `AuditRecord` gets `SlowPath *SlowPathInvocation` field
- When slow path runs, invocation written to audit
- Includes: model, provider, prompt version, input/output tokens, latency ms, raw response, parsed result
- Tests: audit record contains slow path data

### Slice 12: Integration tests + benchmarks
**Files**: `app/slowpath/integration_test.go`, `app/slowpath/benchmark_test.go`
- Integration tests (~15 subtests):
  - Full flow: fast → escalate → slow → merge → policy
  - Budget exhaustion → fallback to fast result
  - Circuit breaker open → fallback
  - OpenAI text check → spam detected
  - Gemini text check → ham confirmed
  - OpenAI vision → image spam
  - Gemini vision → image ham
  - Prompt version selection
  - Multiple providers, round-robin
  - Concurrent requests, no data race
  - Backward compat: SlowPathEnabled=false, inline LLM still works
- Benchmarks:
  - `Engine.Check` — target <50ms (mock provider, no network)
  - `MergeResults` — target <1μs
  - `BudgetTracker.Allow` — target <100ns

## Verification

After each slice:
1. `make build` — compiles
2. `make test` — all tests pass
3. `go vet ./...` — clean

After final slice:
1. All 7 completion criteria verified
2. Full test suite green
3. No file exceeds 500 LOC
4. `app/slowpath/` fully isolated
5. `go get github.com/sony/gobreaker/v2` in go.mod

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Breaking inline LLM | Feature flag default false; inline path untouched |
| gobreaker new dep | Stable, zero transitive deps, 8k+ stars |
| Budget accuracy | Post-call tracking, enforce on next call |
| Provider API changes | Adapter pattern isolates SDK coupling |
| Vision model limits | Token/image size limits in adapter config |
| Two-pass policy confusion | Clear logging: "tentative" vs "final" action |

## Maintainability Estimates

- `types.go`: ~120 LOC
- `openai_adapter.go`: ~100 LOC
- `gemini_adapter.go`: ~90 LOC
- `budget.go`: ~100 LOC
- `breaker.go`: ~60 LOC
- `prompt_registry.go`: ~100 LOC
- `engine.go`: ~150 LOC
- `openai_vision.go`: ~80 LOC
- `gemini_vision.go`: ~80 LOC
- `merge.go`: ~60 LOC
- All well within 500 LOC per file
