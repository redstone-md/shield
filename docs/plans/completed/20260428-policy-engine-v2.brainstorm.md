# Stage 5: Policy Engine v2 — Brainstorm

## Current State

Policy logic lives in `app/events/policy.go` (127 LOC). Single `defaultPolicyEngine`:
- Allow/detect → Delete/Restrict/Ban based on `BanInterval`, `IsSuperUser`, escalation
- Escalation: `spamPenalty()` uses `ModerationConfig{FirstStrike, SecondStrike}` from RuleSet
- No profiles, no risk-type matrix, no shadow mode, no versioning

## Roadmap Requirements (Phase 5)

1. Separate package `app/policy` with `PolicyInput`, `Decision`, `ActionPlan`, `DecisionExplanation`
2. Severity profiles: `permissive`, `balanced`, `strict`
3. Action matrix by risk type: `spam`, `abuse`, `scam`, `raid`, `nsfw`, `unknown`
4. Escalation chain: warn → mute → delete+mute → ban (strike history + user role)
5. Signal source awareness: fast path, slow path, user report, manual admin
6. Dry-run + shadow-decision mode
7. Policy simulation on historical incidents
8. Action selection decoupled from Telegram layer
9. Policy versioning in audit trail
10. Control plane UI/API for policy profile management

## Scope Decision

### In Scope (modular monolith, no premature infra)
- `app/policy/` package with interfaces and engine
- 3 severity profiles as config presets
- Risk-type action matrix
- Enhanced escalation chain (warn → mute → delete+mute → ban)
- Dry-run + shadow mode
- Policy versioning in audit
- Tests: same signals → different outcomes per profile

### Deferred (needs UI/control plane changes beyond this stage)
- Policy simulation UI
- Control plane policy profile CRUD API endpoints
- Risk type classification in detectors (current detectors don't emit risk types yet)

## Design Options

### Option A: Table-Driven Policy

```go
type PolicyProfile struct {
    Name      string
    Version   int
    Matrix    map[RiskType]ActionLevel
    Escalate  EscalationConfig
    Overrides map[SignalSource]ActionLevel
}

type ActionLevel int
const (
    ActionNone ActionLevel = iota
    ActionWarn
    ActionMute
    ActionDeleteAndMute
    ActionBan
)

type RiskType string
const (
    RiskSpam   RiskType = "spam"
    RiskAbuse  RiskType = "abuse"
    RiskScam   RiskType = "scam"
    RiskRaid   RiskType = "raid"
    RiskNSFW   RiskType = "nsfw"
    RiskUnknown RiskType = "unknown"
)
```

Profile lookup: `profile.Matrix[riskType]` → base action level, then escalation bumps level by strike count.

**Pros:** Simple, testable, declarative, easy to serialize to JSON/config
**Cons:** Risk type must come from detectors (they don't classify yet)

### Option B: Rule Chain

```go
type PolicyRule interface {
    Evaluate(ctx context.Context, input PolicyInput) (Action, bool)
}
```

Chain of rules, first match wins. Like middleware.

**Pros:** Extensible, can add complex conditions
**Cons:** Hard to preview, hard to explain "why", ordering matters

### Option C: Hybrid — Profile + Overrides

Profile sets defaults per risk type. Override rules can modify based on context (source, user role, whitelist).

**Pros:** Best of both. Declarative baseline + escape hatches
**Cons:** Slightly more complex

## Recommendation: Option C (Hybrid)

Reason: Current codebase already has a hybrid approach (base decision from detectors, then override for superuser, then override for escalation). Extending this pattern is natural.

## Architecture

```
app/policy/
  engine.go         — PolicyEngine interface + engine impl
  profile.go        — PolicyProfile, RiskType, ActionLevel, presets
  escalation.go     — escalation chain logic
  decision.go       — PolicyInput, PolicyDecision, DecisionExplanation
  engine_test.go
  profile_test.go
  escalation_test.go
```

## Migration Strategy

1. Create `app/policy/` with all types + engine
2. Wire `app/policy` into `app/events/pipeline.go` replacing inline `defaultPolicyEngine`
3. Move `spamPenalty()` and `spamScore()` to `app/policy/`
4. Add `PolicyProfile` to `RuleSet` (in `app/rules/ruleset.go`)
5. Thread `PolicyProfile` through listener config → policy engine
6. Shadow mode: `PolicyEngine` returns both real + shadow decisions; pipeline logs shadow but applies real
7. Policy version: stored in profile, written to audit trail

## Risk Type Classification (Deferred)

Current detectors return `spamcheck.Response` with `Name` field (e.g. "stop-words", "similarity", "links"). No risk type. To classify:

```go
var nameToRiskType = map[string]RiskType{
    "stop-words":   RiskSpam,
    "similarity":   RiskSpam,
    "links":        RiskSpam,
    "classifier":   RiskSpam,
    "cas":          RiskSpam,
    "duplicate":    RiskRaid,
    "mentions":     RiskAbuse,
    "username-symbols": RiskAbuse,
}
```

This mapping is a stopgap. Proper classification should come from detectors in Stage 6+. For now, default to `RiskSpam` for all matches.

## Slice Plan (Draft)

1. Create `app/policy/` package with types + 3 profile presets
2. Implement engine with profile-driven decisions + escalation
3. Wire into pipeline, replace `defaultPolicyEngine`
4. Add shadow mode + dry-run awareness
5. Add policy versioning to audit
6. Integration tests: same signals, different profiles
7. Benchmarks + edge cases

## LOC Budget

- `engine.go`: ~120 LOC
- `profile.go`: ~100 LOC
- `escalation.go`: ~80 LOC
- `decision.go`: ~60 LOC
- Tests: ~300 LOC total
- All well within 500 LOC per file
