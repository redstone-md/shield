# 2026-04-22 Action Journal Seam Brainstorm

## Problem

The moderation pipeline can now suppress duplicate ingress events, but executor side effects still disappear after the Telegram call returns. There is no durable command journal for bans, restrictions, or deletes, so later retry-safe execution work has no authoritative action history.

## Constraints

- Keep current Telegram behavior unchanged.
- Add durable command naming and attempt records without implementing replay in the same slice.
- Keep the journal close to the action executor boundary instead of scattering writes through the worker.

## Decision

Add `moderation_actions` storage plus explicit executor command names, and record each completed or failed Telegram command attempt from the action executor.

## Verification Target

- moderation action attempts persist with command, status, and last error
- ban/restrict/delete executor paths emit the expected journal records
- runtime assembly wires the new journal store without changing current moderation behavior
