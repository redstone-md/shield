# 20260422 Retry Recovery Integration Plan

1. Update `incoming_events` completion/reserve semantics for failed action retries.
2. Update the moderation pipeline to skip final audit writes on failed Telegram actions.
3. Add storage tests for failed completion retryability.
4. Add listener integration tests for duplicate suppression and Telegram API recovery.
5. Update architecture docs and the remaining phase-1 completion criterion.
6. Run focused regressions and `git diff --check`.
