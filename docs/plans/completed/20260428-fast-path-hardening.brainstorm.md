# Brainstorm: Fast Path Hardening (Stage 4)

Date: 2026-04-28

## Current State

### Detection Pipeline (`lib/tgspam/detector.go`, 493 LOC)

12 detectors, sequential hardcoded chain:
```
duplicate → approved-user bypass → stopWords → emoji → metaChecks[] → luaChecks[] →
CAS → multiLang → abnormalSpacing → [shortMsg gate] → similarity → classifier → LLM consensus
```

All return `spamcheck.Response{Spam: bool, Details: string}`. Pure boolean OR aggregation — any `Spam: true` → spam. LLM can veto/confirm.

### Gaps vs Roadmap

| Goal | Gap |
|------|-----|
| Unified `RiskSignal` | No score, weight, rule ID, signal type on response |
| Weighted aggregation | Boolean OR only. Classifier has internal prob but binarized |
| Explainability | String-only `Details`. No rule IDs, no weight breakdown, no matched fragment |
| Normalization | Missing: NFKC, confusables, mixed-script fold. Emoji done ad-hoc |
| Deterministic vs probabilistic split | All checks identical boolean |
| Rate-based heuristics | Only duplicate (exact hash). No freq tracking, burst detection |
| Sender profiling | Only approved-user count + basic meta |
| Replay harness | None |
| Benchmark suite | None |

## Design Decisions

### D1: RiskSignal extends spamcheck.Response, not replaces

Add fields to `spamcheck.Response`:
- `Score float64` — 0.0 (ham) to 1.0 (spam)
- `Weight float64` — configurable signal importance
- `RuleID string` — structured ID (e.g. `stopword:exact`, `meta:links`)
- `SignalType string` — `deterministic` or `probabilistic`
- `MatchedFragment string` — normalized text that triggered

Backward compat: existing checks set `Spam: bool` only. New aggregation reads `Score` when present, falls back to `Spam` bool.

### D2: Aggregation strategy — weighted sum with threshold

```
RiskScore = Σ(signal.Score × signal.Weight) / Σ(signal.Weight)
```

Configurable per-tenant via `RuleSet`. Default weights hard-coded from current behavior. Thresholds:
- `< 0.3` → ham
- `0.3-0.7` → ambiguous (escalate to slow path when available)
- `≥ 0.7` → spam

This preserves current boolean behavior: each deterministic check sets Score=1.0, Weight=1.0, threshold=0.5.

### D3: Normalization pipeline as ordered stages

```
raw → NFKC → strip-invisible → confusables → lowercase → canonical-whitespace → trim
```

New stages: NFKC normalization (unicode/norm package), confusables table (data from unicode security mechanisms). Script fold replaced by confusables which is superset.

### D4: lib/tgspam stays tenant-agnostic

Per AGENTS.md rule: `lib/tgspam/` stays tenant-agnostic. Isolation at orchestration. RiskSignal/Score lives in lib. Per-tenant weight config passed in from app layer.

### D5: Sender profiling as new MetaCheck

New checks registered via existing `WithMetaChecks`:
- Username entropy (random chars)
- Username similarity to display name
- Account age proxy (none available from TG API — defer)
- Mention count threshold (already exists)

### D6: Rate tracking in duplicate detector extension

Extend `duplicateDetector` to also track message frequency per user:
- Messages per minute window
- Burst detection (>N messages in window)
- This reuses existing LRU cache infrastructure

### D7: Replay harness as test helper

`lib/tgspam/replay/` package:
- Load anonymized cases from JSON
- Run through Detector.Check
- Compare expected vs actual
- Output FP/FN counts per signal

### D8: Feature flags for new checks

`--enable-sender-profiling`, `--enable-rate-tracking`, `--enable-scoring` — all default off. Existing boolean path unchanged when flags off.

## Slice Candidates

1. **RiskSignal fields on spamcheck.Response** — extend struct, backward compat
2. **Rule IDs on all checks** — add `RuleID` to each detector response
3. **Normalization: NFKC + confusables** — new pipeline stages
4. **Weighted aggregator** — RiskScore with configurable weights
5. **Explainability payload** — matched fragment, weight breakdown, JSON output
6. **Sender profiling MetaChecks** — username entropy, mention abuse
7. **Rate tracking in duplicate detector** — freq per user, burst detection
8. **Replay harness** — anonymized case runner
9. **Benchmark suite** — latency + burst load benchmarks
10. **Feature flag wiring** — CLI flags, runtime toggles

## Risks

| Risk | Mitigation |
|------|-----------|
| Breaking existing bool-based checks | Score defaults from Spam bool. Aggregation reads both |
| Confusables table size | Use unicode confusables data, ~10K entries, compile-time |
| Performance regression from scoring | Benchmark before/after. Aggregation is O(N) signals |
| classifier.go prob → score mapping | Classifier already returns float64. Map to [0,1] directly |
| `detector.go` at 493 LOC, close to 500 limit | Extract aggregator to separate file |
