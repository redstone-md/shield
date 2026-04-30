# 20260422 Audit Idempotency Link Plan

1. Add `idempotency_key` to `storage.DetectedSpamInfo` and `detected_spam` migrations.
2. Extend enriched audit persistence to write `record.Event.IdempotencyKey`.
3. Add storage/runtime tests covering the new field.
4. Update architecture docs and phase-1 completion criterion.
5. Run focused regressions and `git diff --check`.
