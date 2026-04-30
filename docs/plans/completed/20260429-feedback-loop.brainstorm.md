# Stage 8 Brainstorm: Feedback Loop & Knowledge Base

## Current State
- Samples (spam/ham) stored in `storage.Samples` — no versioning, no review
- Dictionary (stop phrases, ignored words) in `storage.Dictionary` — no versioning, no review
- `DetectedSpam` has binary `added` flag — no formal review workflow
- `RuleSets` has proper versioning (active pointer + immutable history) — good pattern to reuse
- Incidents have status workflow (open→reviewing→resolved→closed) — reusable
- Appeals have replay concept — replay decision against current detector
- Classifier supports incremental learn/unlearn + full reload
- `SampleUpdater` interface: Append/Remove — already wired to storage
- `reloadAndNotify()` pattern in DictionaryService propagates changes

## Gaps
- No label concept (confirmed_spam / false_positive / missed_spam)
- No candidate generation from labeled data
- No review-before-publish for new samples/rules
- No versioning on samples or dictionary (can't rollback)
- No tenant-local vs global knowledge separation

## Option A: Label-First Pipeline (Recommended)
Build labeling on top of incidents, then derive candidates from labels.

1. Add `Label` to incidents (confirmed_spam, false_positive, missed_spam, policy_override)
2. From confirmed_spam incidents → candidate stop phrases (high-frequency n-grams)
3. From false_positive incidents → ham sample candidates
4. Review queue for candidates → approve → auto-promote to samples/dictionary
5. Version snapshots of samples+dictionary using rule_sets pattern
6. Rollback = restore previous version snapshot

Pros: Builds on existing incidents. Labeling is the foundation for everything else.
Cons: Requires incidents to exist first (they do now from Stage 7).

## Option B: Direct Sample Versioning
Version the samples and dictionary tables directly, skip labeling.

Pros: Simpler, doesn't depend on incidents.
Cons: No feedback signal. No way to know *why* a sample was added. No candidate generation.

## Option C: Full Knowledge Base Rewrite
New unified knowledge package with versioning, diffing, A/B rollout.

Pros: Clean architecture.
Cons: Massive scope. Premature for MVP.

## Recommendation
**Option A**. Label-first pipeline. Pragmatic MVP scope:
- Labels on incidents
- Candidate generation (simple n-gram extraction)
- Review queue for candidates
- Knowledge versioning (snapshot model)
- Rollback

Defer: A/B rollout, shadow mode, vector patterns, offline evaluation, tenant-local/global split.
