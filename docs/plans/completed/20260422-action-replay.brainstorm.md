# 20260422 Action Replay Brainstorm

## Goal

Make moderation command execution replay-safe for the same `idempotency_key` so Telegram retries do not repeat already completed `ban` or `delete` operations.

## Constraints

- Keep the slice atomic and limited to the action journal and executor boundary.
- Do not change moderation policy semantics.
- Preserve current queue and `incoming_events` replay behavior.

## Approach

1. Extend observability metadata to carry `idempotency_key`.
2. Add a lookup on `moderation_actions` for the latest attempt of a command target.
3. Let `ActionExecutor` skip Telegram calls when the same command already completed and increment `attempt` after failed executions.
4. Verify with storage and executor tests.
