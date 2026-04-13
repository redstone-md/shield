# 2026-04-13 Incoming Events Idempotency Plan

1. Add `incoming_events` storage in `app/storage` with an idempotent insert keyed by `(gid, idempotency_key)`.
2. Extend `moderation.IncomingEvent` with Telegram transport IDs and `idempotency_key`.
3. Wire `IncomingEvents` into `TelegramListener` and persist the ingress record before queue publication.
4. Preserve edited-message handling by carrying `update_id` and `edited_message_id` through the existing listener path.
5. Add focused storage, listener, and runtime assembly tests.
6. Update phase-1 roadmap and architecture docs to record the new ingress persistence seam.

## Validation Skills

- `mcaf-solid-maintainability`: keep transport metadata explicit and avoid leaking storage concerns into the worker
- `mcaf-testing`: cover idempotent inserts and listener idempotency-key generation
