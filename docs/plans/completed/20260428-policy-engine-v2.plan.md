# Plan: Policy Engine v2 (Stage 5)

Date: 2026-04-28
Roadmap: `docs/plans/roadmap/05-policy-engine-v2.md`

## Completion Criteria

1. Same signals produce different actions depending on tenant's policy profile (permissive/balanced/strict).
2. Detector returns signals, does not decide ban — policy engine decides.
3. New policy rule enabled via config/version switch without rewriting detectors.
4. Escalation chain: warn → mute → delete+mute → ban based on strike history.
5. Shadow mode logs what would happen without applying.
6. Policy version recorded in audit trail.

## Design Decisions

1. **New `app/policy/` package** — separate from `app/events/`. Owns `PolicyProfile`, `RiskType`, escalation logic, engine.
2. **Hybrid approach** — profile sets defaults per risk type. Override rules modify based on context (source, user role, whitelist). Matches existing pattern in codebase.
3. **Risk type from check name** — stopgap mapping: `checkNameToRiskType` maps detector `Name`/`RuleID` to `RiskType`. Default `RiskUnknown`. Proper classification deferred.
4. **3 severity presets** — `PermissiveProfile`, `BalancedProfile`, `StrictProfile` as Go constructors. Stored in `PolicyProfile` struct, serializable to JSON.
5. **Shadow mode** — `PolicyEngine.Decide` returns both `Real` and `Shadow` outcomes when shadow enabled. Pipeline logs shadow, applies real.
6. **Policy version** — `PolicyProfile.Version` field. Written to `AuditRecord.PolicyVersion`.
7. **Backward compat** — `defaultPolicyEngine` stays in `app/events/` as thin wrapper delegating to `app/policy`. Existing tests pass unchanged.
8. **No UI changes** — control plane CRUD for profiles deferred to later stage.

## Slice Order

### Slice 1: Types and profile presets
**Files**: `app/policy/types.go`, `app/policy/profile.go`, `app/policy/profile_test.go`
- `RiskType` string enum: `spam`, `abuse`, `scam`, `raid`, `nsfw`, `unknown`
- `ActionLevel` int enum: `None`, `Warn`, `Mute`, `DeleteAndMute`, `Ban`
- `PolicyProfile` struct: `Name`, `Version`, `Matrix map[RiskType]ActionLevel`, `Escalation EscalationConfig`
- `EscalationConfig`: `Enabled bool`, `Levels []ActionLevel` (indexed by strike count)
- 3 presets: `PermissiveProfile()`, `BalancedProfile()`, `StrictProfile()`
- `checkNameToRiskType` mapping
- Tests: profile creation, risk type mapping

### Slice 2: Policy engine with profile-driven decisions
**Files**: `app/policy/engine.go`, `app/policy/engine_test.go`
- `PolicyInput` struct: signals, strike count, superuser flag, soft-ban flag, source
- `PolicyDecision` struct: action, duration, restrict, reason, explanation
- `DecisionExplanation`: matched rules, risk type, profile name, escalation level
- `Engine` struct: `Profile PolicyProfile`, `ModerationCfg ModerationConfig`
- `Engine.Decide(input PolicyInput) PolicyDecision`
- Logic: classify risk type → lookup profile matrix → apply escalation → produce decision
- Tests: same signals, different profiles → different actions

### Slice 3: Escalation chain
**Files**: `app/policy/escalation.go`, `app/policy/escalation_test.go`
- `applyEscalation(base ActionLevel, strikes int, cfg EscalationConfig) ActionLevel`
- Escalation levels: `[Warn, Mute, DeleteAndMute, Ban]`
- Strike 0 = base action, strike 1 = base+1, etc., capped at Ban
- `actionLevelToAction(level ActionLevel, cfg ModerationConfig) (Action, time.Duration, bool)`
- Maps `ActionLevel` to `moderation.Action` + duration + restrict flag
- Tests: escalation at each strike level, cap at Ban

### Slice 4: Shadow mode + dry-run awareness
**Files**: `app/policy/engine.go` (extend)
- `Engine` gets `ShadowMode bool` and `DryRun bool` fields
- When shadow: return `ShadowDecision` alongside real decision
- `PolicyOutcome` gets `Shadow *PolicyDecision` field
- When dry-run: `PolicyDecision.Action` stays but `Applied` = false (caller checks)
- Tests: shadow decision differs from real when profile changes mid-eval

### Slice 5: Wire into pipeline
**Files**: `app/events/pipeline.go`, `app/events/policy.go`
- `defaultPolicyEngine` wraps `policy.Engine`
- `PolicyRequest` extended with `PolicyProfile`
- `TelegramListener` gets `PolicyProfile` field set from `RuleSet`
- `ensurePipeline()` creates `policy.Engine` with current profile
- `RuleSet` gets `PolicyProfileName string` field (resolves to preset)
- Tests: pipeline integration with profile-driven engine

### Slice 6: Policy versioning in audit
**Files**: `app/events/audit_writer.go`, `app/moderation/contracts.go`
- `PolicyDecision` gets `PolicyVersion int` and `ProfileName string` fields
- `AuditRecord` gets `PolicyVersion int` field
- Pipeline writes policy version from profile
- Tests: audit record contains policy version

### Slice 7: Move spamPenalty/spamScore to policy package
**Files**: `app/policy/helpers.go`, `app/events/events.go`
- `spamPenalty` → `policy.ComputePenalty`
- `spamScore` → `policy.ComputeScore`
- `app/events/` calls `policy.*` wrappers — no logic duplication
- Original functions become thin wrappers for backward compat
- Tests: existing policy tests pass, new tests for migrated functions

### Slice 8: Integration tests — same signals, different profiles
**Files**: `app/policy/integration_test.go`
- End-to-end: configure engine with permissive/balanced/strict
- Feed same signal set, verify different outcomes
- Escalation: verify strike count progression
- Shadow mode: verify shadow logged but real applied
- Superuser override: verify exempt regardless of profile
- ~14 subtests

### Slice 9: Benchmarks + edge cases
**Files**: `app/policy/benchmark_test.go`, `app/policy/engine_test.go` (extend)
- Benchmark `Engine.Decide` — target <1μs per decision (pure logic, no I/O)
- Edge cases: empty signals, nil profile, zero strikes, max strikes, unknown risk type
- Verify no panics on malformed input

### Slice 10: Update listener ApplyRuleSet for policy profile reload
**Files**: `app/events/listener.go`, `app/rules/ruleset.go`
- `RuleSet` gets `PolicyProfileName string` field
- `TelegramListener.ApplyRuleSet` recreates `policy.Engine` when profile changes
- Profile name resolution: `"permissive"` → `PermissiveProfile()`, etc., unknown → `BalancedProfile()`
- Tests: reload changes profile, verify new decisions

## Verification

After each slice:
1. `make build` — compiles
2. `make test` — all existing tests pass
3. `go vet ./...` — clean

After final slice:
1. All 6 completion criteria verified
2. Full test suite green
3. No file exceeds 500 LOC
4. `app/policy/` package fully isolated from `app/events/`

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Breaking existing policy decisions | `defaultPolicyEngine` wraps new engine, existing tests unchanged |
| Profile not set in RuleSet | Default to `BalancedProfile()` — matches current hardcoded behavior |
| Risk type misclassification | Stopgap mapping + `RiskUnknown` fallback; proper classification deferred |
| LOC overflow in listener.go | Policy wiring stays thin (<30 LOC) |
| Shadow mode confusion | Shadow outcome only in log, never applied; explicit `Shadow != nil` check |

## Maintainability

- `types.go`: ~50 LOC
- `profile.go`: ~100 LOC
- `engine.go`: ~150 LOC
- `escalation.go`: ~80 LOC
- `helpers.go`: ~60 LOC
- Tests: ~400 LOC total across files
- All well within 500 LOC per file
