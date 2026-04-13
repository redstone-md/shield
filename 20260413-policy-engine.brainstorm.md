# Policy Engine Brainstorm

## Problem

The queue worker and action executor are in place, but the moderation decision is still implicit inside `processQueuedEvent`. Phase 0 requires a separate policy layer with minimal `allow/delete/restrict/ban` behavior.

## Current facts

- Detection still returns `bot.Response`.
- `processQueuedEvent` currently decides:
  - whether a message is actionable
  - whether a superuser should be exempt
  - whether a strike escalates to restrict or ban
- `app/moderation` already defines `PolicyDecision`.

## Recommended direction

- Add a `PolicyEngine` to `app/events`.
- Have the worker compute detection facts, then ask policy for a `PolicyDecision`.
- Keep the output small but explicit:
  - action
  - reason
  - score
  - sanction duration
- Preserve current behavior:
  - `allow` when not spam
  - `allow` for superusers
  - `restrict` or `ban` based on strikes and soft-ban mode
  - `delete` when spam is actionable but no ban interval is requested
