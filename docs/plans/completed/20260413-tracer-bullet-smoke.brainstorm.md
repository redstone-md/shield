# Tracer Bullet Smoke Brainstorm

## Problem

Phase 0 still lacks an explicit smoke test proving the end-to-end internal path:

`Telegram update -> queue -> worker -> detection -> policy -> action -> audit`

## Current facts

- The runtime already has extracted queue, policy, action, and audit seams.
- Existing tests cover each seam and several listener behaviors, but not one ordered end-to-end tracer-bullet path.
- `app/main.go` still builds the listener directly, but the worker internals now support injected seams for testing.

## Recommended direction

- Add a focused listener smoke test that runs through `TelegramListener.Do`.
- Inject spy implementations for:
  - detection via `Bot`
  - policy via `PolicyEngine`
  - action via `ActionExecutor`
  - audit via `AuditWriter`
- Assert both:
  - the full order of calls
  - the key data handed across the boundaries

## Why this slice stays small

- No production behavior changes are required if the current seams are sufficient.
- The smoke test gives a durable proof point for the roadmap phase.
