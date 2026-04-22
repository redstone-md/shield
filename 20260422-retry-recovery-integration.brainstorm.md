# 20260422 Retry Recovery Integration Brainstorm

## Goal

Make duplicate Telegram retries recover correctly after executor failures while still suppressing duplicate successful processing.

## Risks

- `incoming_events` currently treats any completed snapshot as processed.
- failed action attempts can block retries or create duplicate audit entries.

## Approach

1. Persist failed action snapshots without `processed_at`.
2. Let `Reserve` distinguish:
   - processed duplicate
   - failed prior attempt eligible for retry
   - still in-flight duplicate
3. Only write the final audit sink after successful completion or no-side-effect completion.
4. Add storage and listener integration tests for:
   - successful duplicate suppression
   - retry recovery after Telegram API failure
