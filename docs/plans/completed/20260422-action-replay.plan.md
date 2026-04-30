# 20260422 Action Replay Plan

1. Extend `app/observability/context.go` to carry `idempotency_key` alongside `event_id` and `correlation_id`.
2. Add replay lookup primitives to `app/storage/moderation_actions.go` and cover them with tests.
3. Update `app/events/action_executor.go` to consult the journal before executing Telegram commands and to increment attempt counts.
4. Add executor tests for completed replay suppression and failed retry attempt numbering.
5. Update architecture docs and roadmap status for the completed journal replay slice.
6. Run focused regression tests and `git diff --check`.
