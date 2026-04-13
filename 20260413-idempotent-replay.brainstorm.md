# 2026-04-13 Idempotent Replay Brainstorm

## Problem

`incoming_events` now records a durable idempotency key, but Telegram retries still flow through the worker and can repeat bans, deletes, and audit writes.

## Constraints

- Keep replay state close to the ingress ledger instead of inventing a separate action journal in the same slice.
- Suppress duplicate retries before they reach the worker.
- Only mark an event replayable after a successful moderation pass.

## Decision

Extend `incoming_events` with a completed decision/action snapshot, mark success from the worker, and have the listener short-circuit duplicate retries using that stored replay state.

## Verification Target

- successful moderation marks an ingress event as processed
- duplicate retries skip the worker when a completed replay snapshot exists
- pending or failed events remain retriable because they are not marked complete
