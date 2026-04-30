# Brainstorm: warn command executor

## Problem

Phase 1 still leaves `WarnUser` outside the shared `ActionExecutor` surface. Admin `/warn` deletes messages and sends the warning directly through `tbAPI`, so it bypasses action journaling and replay boundaries.

## Goal

Route warn-side effects through the same explicit executor contract as delete, mute, and ban without changing visible admin behaviour.

## Constraints

- Keep the slice atomic.
- Preserve existing `/warn` message text and deletion behaviour.
- Reuse existing moderation action journal and idempotency metadata flow.
- Keep files under repo maintainability limits.

## Options

1. Add `WarnUser` to `ActionExecutor` and route admin `/warn` through it.
   - Pros: closes the remaining roadmap item cleanly, reuses replay/journal seam, minimal surface change.
   - Cons: needs runtime wiring so admin handler sees initialized executor.
2. Create a separate warning sender abstraction.
   - Pros: narrower local change.
   - Cons: duplicates the executor boundary and leaves roadmap intent partially unmet.

## Decision

Choose option 1. Add `WarnUser`, wire admin handler to use it, initialize the executor before admin/reports handlers are constructed, and add focused regression tests.
