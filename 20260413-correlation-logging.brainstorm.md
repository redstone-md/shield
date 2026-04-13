# 2026-04-13 Correlation Logging Brainstorm

## Problem

Phase 0 still lacks stable `event_id` and `correlation_id` propagation across the moderation tracer-bullet path. The queue worker logs both IDs, but downstream detection, action, and storage logs lose them.

## Constraints

- Keep the slice atomic and limited to the existing moderation hot path.
- Avoid broad interface churn outside the pipeline.
- Preserve current behavior and existing tests.
- Keep files under repo maintainability limits.

## Options

### Option 1: Add IDs as explicit parameters everywhere

- Pro: very explicit
- Con: too much signature churn across unrelated code paths

### Option 2: Store IDs on `bot.Message`

- Pro: easy for detection
- Con: transport metadata leaks into domain message shape and does not help storage calls that already use `context.Context`

### Option 3: Carry IDs in `context.Context` and add small logging helpers

- Pro: fits existing storage method signatures, keeps propagation local to the moderation flow, and lets downstream code opt in without breaking old call sites
- Con: requires optional context-aware adapters for detection and action

## Decision

Use context metadata plus a tiny observability helper package. Stamp IDs once in `processQueuedEvent`, add an optional context-aware detection hook, move action executor methods to accept context, and update storage logs that already receive context.

## Verification Target

- targeted tests for tracer-bullet flow prove detection, action, audit, and storage receive the same metadata
- targeted Go tests pass for `app/events`, `app/bot`, and `app/storage`
