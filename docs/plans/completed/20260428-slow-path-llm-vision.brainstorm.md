# Stage 6: Slow Path LLM/Vision — Brainstorm

## Current State

LLM checks (OpenAI, Gemini) run **inline** inside `Detector.Check()`:
- `lib/tgspam/detector.go:299-330` — LLM called synchronously during fast path
- `lib/tgspam/openai.go` — `sashabaranov/go-openai` client, inline retry
- `lib/tgspam/gemini.go` — `google.golang.org/genai` client, inline retry
- `lib/tgspam/llm.go` — shared `runLLMProviderCheck`, JSON parsing, history append
- `app/events/reports.go:246` — report-triggered LLM inline call
- No queue separation, no budget, no circuit breaker, no prompt versioning

Existing infra:
- `app/moderation/queue.go` — `Queue` interface + `InMemoryQueue` (channel-backed)
- `app/moderation/contracts.go` — `IncomingEvent`, `DetectionResult`, `DetectionSignal`
- `app/policy/` — profile-driven engine, shadow mode, escalation
- `golang.org/x/time` already in `go.mod` — has `rate.Limiter`

## Goals

1. Extract LLM/Vision out of inline fast path into async slow path
2. Fast path produces initial `DetectionResult` quickly; slow path can override
3. Budget control per tenant — token spend, request count, cost estimation
4. Circuit breaker per provider — OpenAI, Gemini (and future)
5. Prompt/version registry — reproducible decisions
6. Vision/OCR provider interface — image analysis via Gemini multimodal
7. Merge slow+fast signals → unified `DetectionResult` for policy engine
8. Backward compat — feature flag controls sync vs async LLM

## Key Design Decisions

### D1: Package location

**Option A**: `app/slowpath/` — new isolated package
**Option B**: `lib/tgspam/slowpath/` — keeps detector coupling

**Pick: A** — `app/slowpath/`. Same pattern as `app/policy/`. Detectors stay in `lib/tgspam/`, slow path orchestrates them. Policy engine consumes merged results.

### D2: Queue architecture

Current: single `InMemoryQueue` with `IncomingEvent`.

Need: fast queue (current) + slow queue for LLM escalation.

**Pick: Add `SlowPathQueue` to `app/moderation/queue.go`**. Same `Queue` interface, typed to `SlowPathRequest`. No new package needed.

### D3: SlowPathRequest contract

```go
type SlowPathRequest struct {
    EventID       string
    CorrelationID string
    TenantID      string
    Reason        EscalationReason  // why we escalated
    FastResult    DetectionResult   // fast-path signals for context
    Content       Content           // original content
    PromptVersion string            // which prompt to use
    BudgetClass   BudgetClass       // cost tier
    ImageData     []byte            // for vision, nil if text-only
    ImageMIME     string            // e.g. "image/jpeg"
}
```

### D4: Escalation reasons

```go
type EscalationReason string

const (
    EscalationAmbiguousFast  EscalationReason = "ambiguous_fast"   // score near threshold
    EscalationImageContent   EscalationReason = "image_content"    // has media
    EscalationUserReport     EscalationReason = "user_report"      // manual report
    EscalationHighRiskPolicy EscalationReason = "high_risk_policy" // tenant policy says always LLM
    EscalationForceLLM       EscalationReason = "force_llm"        // explicit force flag
)
```

### D5: Provider abstraction

Current: `openAIChecker` and `geminiChecker` are concrete, tightly coupled to `Detector`.

**Pick: Extract `LLMProvider` interface in `app/slowpath/`.** Wraps existing checkers without rewriting them.

```go
type LLMProvider interface {
    Name() string
    Check(ctx context.Context, req ProviderRequest) (*ProviderResult, error)
}
```

`ProviderRequest` carries message + history + prompt version.
`ProviderResult` carries spam/ham + confidence + token usage + latency.

OpenAI and Gemini adapters wrap existing `openAIChecker`/`geminiChecker` internally.

### D6: Circuit breaker

**Pick: `sony/gobreaker/v2`** — already widely used, simple API, v2 supports generics.

One breaker per provider. Settings:
- `MaxRequests`: 3 (half-open probe)
- `Interval`: 60s (reset counters)
- `Timeout`: 30s (open→half-open)
- `ReadyToTrip`: 5 consecutive failures

### D7: Budget control

Per-tenant budget tracking. No external store — in-memory with periodic checkpoint.

```go
type BudgetConfig struct {
    MaxRequestsPerHour int
    MaxTokensPerHour   int
    MaxCostPerHour     float64 // USD estimate
}

type BudgetTracker struct {
    configs  map[string]BudgetConfig  // tenant → config
    counters map[string]*budgetCounter // tenant → current spend
}
```

Fallback when budget exhausted: skip slow path, use fast-path result only. Log warning.

### D8: Prompt registry

```go
type PromptEntry struct {
    ID        string
    Version   int
    Provider  string // "openai" | "gemini"
    SystemPrompt string
    CustomPrompts []string
    CreatedAt time.Time
    Active    bool
}
```

Storage: start with file-based (JSON/YAML) in config dir. DB later if needed.
Current prompts from env vars become `version=1, active=true` default entries.

### D9: Vision/OCR

Gemini supports multimodal via `genai.Blob{Data, MIMEType}`.
OpenAI supports image inputs via vision-capable models (`gpt-4o`, `gpt-4o-mini` with image content parts).

**Pick: `VisionProvider` interface, both adapters from day one.**

```go
type VisionProvider interface {
    Name() string
    AnalyzeImage(ctx context.Context, imageData []byte, mime string, prompt string) (*ProviderResult, error)
}
```

- `GeminiVisionAdapter` — uses `genai.Blob{Data, MIMEType}` in `GenerateContent` call
- `OpenAIVisionAdapter` — uses `openai.ChatMessagePartImageURL` (base64 data URL) in `CreateChatCompletion`

OCR is just a special case of vision with "extract text" prompt. No separate OCR interface needed initially.

### D10: Fast→Slow→Policy flow

```
Incoming msg
    → Fast path (regex, stopwords, meta, scoring)
    → Decision needed: escalate?
        → Yes: enqueue SlowPathRequest
        → No: PolicyEngine.Decide(fastResult) → action
    → Slow path worker picks up request
        → Budget check → Provider call (with circuit breaker)
        → Merge slow+fast signals
        → PolicyEngine.Decide(mergedResult) → action
        → Override fast-path action if different
```

Key: policy engine runs twice — once on fast result (tentative), once on merged result (final). Final overrides tentative.

### D11: Feature flag for backward compat

`RuleSet.SlowPathEnabled bool` — default false.
- false: LLM runs inline in detector (current behavior)
- true: LLM deferred to slow path, fast path skips LLM

Transition: deploy with false, enable per-tenant, verify, flip default.

### D12: Reproducibility

Every slow-path call records:
- `model` — which model
- `provider` — openai/gemini
- `prompt_version` — from registry
- `input_tokens` / `output_tokens`
- `latency_ms`
- `raw_response` — full JSON from provider
- `parsed_result` — structured spam/ham/confidence

Stored in `SlowPathInvocation` → audit trail.

## Slicing Strategy

**Slice 1**: Types — `SlowPathRequest`, `EscalationReason`, `BudgetConfig`, `LLMProvider` interface, `VisionProvider` interface, `ProviderRequest/Result`
**Slice 2**: Provider adapters — `OpenAIAdapter`, `GeminiAdapter` wrapping existing checkers
**Slice 3**: Budget tracker — in-memory per-tenant budget enforcement
**Slice 4**: Circuit breaker — gobreaker per provider
**Slice 5**: Prompt registry — file-based prompt storage + versioning
**Slice 6**: Slow path engine — `Engine.Check()` orchestrating provider + budget + breaker
**Slice 7**: Vision provider — Gemini multimodal image analysis
**Slice 8**: Merge logic — combine fast+slow signals into unified `DetectionResult`
**Slice 9**: Wire into pipeline — escalation decision, queue publish, slow worker
**Slice 10**: Feature flag — `SlowPathEnabled`, backward compat path
**Slice 11**: Audit integration — `SlowPathInvocation` in audit trail
**Slice 12**: Integration tests + benchmarks

## Risks

1. **Latency regression**: slow path adds async delay. Mitigation: fast-path tentative action applies immediately, slow path can override.
2. **Gemini multimodal API changes**: genai SDK is v1.x. Mitigation: adapter pattern isolates SDK changes.
3. **Budget accuracy**: token counting via provider response, not pre-estimation. Mitigation: track post-call, enforce on next call.
4. **gobreaker dependency**: new dep. Mitigation: `sony/gobreaker` is stable, 8k+ stars, zero transitive deps.

## New Dependencies

- `github.com/sony/gobreaker/v2` — circuit breaker (MIT, stable)

Everything else uses existing deps (`golang.org/x/time/rate`, `google.golang.org/genai`, `sashabaranov/go-openai`).
