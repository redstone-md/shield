# Plan: Fast Path Hardening (Stage 4)

Date: 2026-04-28
Roadmap: `docs/plans/roadmap/04-fast-path-hardening.md`

## Completion Criteria

1. Every detector returns structured `RiskSignal` with score, weight, rule ID — not just bool+string.
2. Weighted aggregation produces `RiskScore` from all signals; no boolean OR hardcoded.
3. Explainability payload: matched rule IDs, signal weights, normalized text fragment, human-readable reason.
4. Normalization pipeline includes NFKC, confusables, mixed-script fold.
5. Replay harness can score anonymized corpus and report FP/FN rates.

## Design Decisions

1. **Extend `spamcheck.Response`** — add `Score float64`, `Weight float64`, `RuleID string`, `NormalizedText string`. Backward compat: zero values = legacy behavior.
2. **New `ScoringEngine`** in `lib/tgspam/scoring.go` — collects signals, applies weights, returns `RiskScore`. Replaces `isSpamDetected()` boolean OR.
3. **`textnorm` gets confusables table** — built-in homoglyph fold (Cyrillic→Latin, etc). NFKC via `unicode/norm`. Mixed-script detection stays in detector.
4. **Replay harness** in `lib/tgspam/replay/` — reads JSONL, runs through `Detector.Check`, compares expected vs actual, reports metrics.
5. **Phase approach**: scoring engine runs in parallel with boolean OR. Feature flag `--scoring-engine` activates new path. Both paths tested.
6. **`lib/tgspam/` stays tenant-agnostic** — scoring/engine isolation is at orchestration layer per Stage 3 decision.
7. **No external deps for confusables** — inline lookup table (~200 entries), covers Cyrillic/Latin/Greek confusables.

## Slice Order

### Slice 1: Extend spamcheck.Response with scoring fields
**Files**: `lib/spamcheck/spamcheck.go`
- Add `Score float64`, `Weight float64`, `RuleID string`, `NormalizedText string` to `Response`
- Add `RiskScore` struct: `Total float64`, `Signals []Response`, `Decision bool`, `Reason string`
- Backward compat: all new fields zero-value, existing code unaffected
- Tests: struct creation, JSON serialization

### Slice 2: Build ScoringEngine
**Files**: `lib/tgspam/scoring.go`, `lib/tgspam/scoring_test.go`
- `ScoringEngine` struct with `AddSignal(r spamcheck.Response)`, `Score() RiskScore`
- Weighted sum: `total += weight * score` for each signal where `Spam==true`
- Decision: `total >= threshold` (configurable, default 1.0)
- Fallback: if no signals have weight > 0, fall back to boolean OR (backward compat)
- Tests: single signal, multiple signals, threshold boundary, zero-weight fallback

### Slice 3: Wire scoring into Detector.Check
**Files**: `lib/tgspam/detector.go`
- After collecting all `[]spamcheck.Response`, feed to `ScoringEngine`
- Add `Config.ScoringEngine bool` flag
- When enabled: `ScoringEngine.Score()` determines final decision
- When disabled: existing boolean OR (no behavior change)
- Tests: scoring engine on/off produces same result for legacy signals

### Slice 4: Add RiskSignal metadata to each check
**Files**: `lib/tgspam/detector_checks.go`, `lib/tgspam/metachecks.go`, `lib/tgspam/duplicate.go`
- Each check populates `RuleID`, `Score`, `Weight`, `NormalizedText` on response
- RuleID convention: `"<check-name>"` (e.g., `"stopword"`, `"similarity"`, `"meta-links"`)
- Score: 1.0 for deterministic checks (stopword, meta), classifier probability for probabilistic
- Weight: 1.0 default, configurable later
- NormalizedText: snippet of matched text (truncated to 64 chars)
- Tests: verify each check populates new fields

### Slice 5: NFKC normalization + confusables table
**Files**: `lib/textnorm/normalizer.go`, `lib/textnorm/confusables.go`, `lib/textnorm/confusables_test.go`
- Add `NFKC` option to `Options` — applies `unicode/norm.NFKC.String()`
- Add `ConfusablesFold` option — applies homoglyph mapping table
- `confusables.go`: inline map ~200 entries (Cyrillic а→a, е→e, о→o, etc, Greek α→a, etc)
- Pipeline order: StripInvisible → NFKC → ConfusablesFold → ScriptFold → LowerCase → CanonicalWhitespace → Trim
- Tests: known confusable pairs normalize identically

### Slice 6: Wire enhanced normalization into detector
**Files**: `lib/tgspam/detector_checks.go`
- `cleanText()` uses expanded normalizer with NFKC + ConfusablesFold
- `normalizeLookupText()` same
- Existing tests should pass unchanged (strict superset of current normalization)
- New tests: confusable spam words detected after normalization

### Slice 7: Replay harness
**Files**: `lib/tgspam/replay/replay.go`, `lib/tgspam/replay/replay_test.go`
- `ReplayCase` struct: `Msg string`, `UserID string`, `ExpectedSpam bool`, `Meta spamcheck.MetaData`
- `Replay(detector *Detector, cases []ReplayCase) ReplayReport`
- `ReplayReport`: `Total int`, `TP/FP/TN/FN int`, `Accuracy float64`, `Details []ReplayResult`
- Reads JSONL, runs `detector.Check()`, compares
- Tests: synthetic corpus with known outcomes

### Slice 8: Benchmark suite for fast path
**Files**: `lib/tgspam/benchmark_test.go`
- Go benchmark: `BenchmarkDetector_Check` — measures latency per message
- Burst scenario: 1000 messages rapid-fire, measure throughput
- Mixed scenario: 50% spam, 50% ham, measure FP/FN with scoring engine on
- No new production code, test file only

### Slice 9: Explainability payload in orchestration
**Files**: `app/bot/spam.go`, `app/moderation/contracts.go`
- `OnMessageWithContext` extracts `RiskScore` from check results
- Include structured explainability in `Response`: rule IDs, weights, score breakdown
- `moderation.IncomingEvent` gets `RiskScore` field for audit trail
- Tests: verify explainability payload in response

### Slice 10: Integration test — scoring end-to-end
**Files**: `lib/tgspam/detector_scoring_test.go`
- Full pipeline test: configure detector with scoring engine, load samples, check messages
- Verify scoring produces correct FP/FN on curated test set
- Verify explainability payload populated correctly
- Verify backward compat: scoring off = same results as before

## Verification

After each slice:
1. `make build` — compiles
2. `make test` — all existing tests pass
3. `go vet ./...` — clean

After final slice:
1. All 5 completion criteria verified
2. Full test suite green
3. No file exceeds 500 LOC
4. Scoring engine feature flag allows gradual rollout

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Breaking existing detection | Feature flag, scoring engine off by default, backward-compat zero values |
| Confusables table incomplete | Start with Cyrillic/Latin/Greek (covers >90% spam evasion) |
| LOC overflow in detector.go (493) | Extract scoring logic to scoring.go, LLM logic already in llm.go |
| Performance regression from normalization | NFKC + confusables are O(n) table lookups, benchmark in slice 8 |
| Replay corpus unavailable | Synthetic corpus for now, real corpus deferred to Phase 8 |

## Maintainability

- `scoring.go` target: <200 LOC
- `confusables.go` target: <250 LOC (data table)
- `replay.go` target: <150 LOC
- `detector.go` stays ≤500 LOC (scoring extracted out)
