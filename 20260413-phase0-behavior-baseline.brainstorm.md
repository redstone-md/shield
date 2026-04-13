# 2026-04-13 Phase 0 Behavior Baseline Brainstorm

## Problem

The only remaining open phase-0 criterion is that the same spam case must pass end-to-end through the internal queue without changing user-visible behavior. The code and tests already prove this, but the roadmap docs do not say so explicitly.

## Constraints

- Keep this slice documentation-only.
- Use existing automated evidence instead of inventing a new parallel test shape.

## Decision

Close the criterion by recording the existing verification evidence:

- `TestTelegramListener_Do`
- `TestTelegramListener_DoWithBotBan`
- `TestTelegramListener_TracerBulletSmoke`

Together these show the queue-backed listener preserves the baseline moderation path while exercising the internal queue worker.
