# 2026-04-13 Idempotent Replay Plan

1. Extend `incoming_events` storage with processed decision/action columns and a safe migration.
2. Add store methods to reserve an event idempotency key and complete it after successful moderation.
3. Short-circuit duplicate retries in the listener before queue publication when replay state already exists.
4. Mark successful worker completions with the final decision/action snapshot.
5. Add focused storage and listener tests for replay completion and duplicate suppression.
6. Update roadmap and architecture docs to record the replay seam.

## Validation Skills

- `mcaf-solid-maintainability`: keep replay state in one authoritative ingress ledger
- `mcaf-testing`: prove duplicate retries do not re-enter the worker
