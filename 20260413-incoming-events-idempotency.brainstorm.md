# 2026-04-13 Incoming Events Idempotency Brainstorm

## Problem

Phase 1 still lacks a durable ingress record for Telegram updates. The queue worker can process a normalized `IncomingEvent`, but the runtime does not yet stamp or persist the transport-level idempotency key needed for safe replay and duplicate suppression.

## Constraints

- Keep the slice behavior-preserving: no replay logic yet.
- Persist the ingress record before queue publication.
- Keep Telegram-specific transport details at the edge while extending the shared moderation contract only with fields the rest of phase 1 needs.

## Options

### Option 1: Add idempotency only inside `app/events`

- Pro: smaller diff
- Con: no durable ingress record and no seam for later replay

### Option 2: Add `incoming_events` storage plus contract fields

- Pro: gives phase 1 a real persistence seam and keeps later replay work incremental
- Con: touches runtime assembly and listener wiring

## Decision

Add `incoming_events` storage in `app/storage`, extend `moderation.IncomingEvent` with Telegram transport IDs plus `idempotency_key`, and persist the record from the listener before queue publication.

## Verification Target

- listener computes a deterministic idempotency key for normal and edited Telegram messages
- ingress events are persisted idempotently by `(gid, idempotency_key)`
- runtime assembly wires the new store without changing current moderation behavior
