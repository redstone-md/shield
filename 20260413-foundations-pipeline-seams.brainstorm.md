# Foundations Pipeline Seams Brainstorm

## Problem

Phase 0 of `docs/ROADMAP.md` and `docs/plans/roadmap/00-foundations-and-internal-pipeline.md` requires the current single-process moderation flow to gain explicit domain boundaries and an internal async seam. Today the listener, bot, moderation action, and persistence concerns are still primarily coupled through `app/events` and `app/main.go`.

## Current facts

- Runtime entry point is `app/main.go`.
- Telegram ingestion and most orchestration live in `app/events`.
- Spam detection lives behind the `app/bot.Bot` interface and is implemented by `app/bot`.
- Audit-like persistence already exists in `app/storage`, but there is no transport-neutral moderation contract package.
- There is no `docs/Architecture.md`.
- Focused baseline verification with an isolated Go 1.25.3 toolchain in `/tmp/go1.25.3/go` passed:
  - `go test ./app/events -run TestTelegramListener_Do -count=1`
  - `go test ./app/bot -run TestSpamFilter_OnMessage -count=1`
- Broad `go test ./app/...` was not used as the baseline for this slice because it hung long enough to be non-actionable for the initial seam work.

## Constraints

- Keep the first increment atomic and low risk.
- Do not change user-visible moderation behaviour yet.
- Keep files under repo maintainability limits.
- Prefer existing repo boundaries: `app/`, `lib/`, `site/`.
- Leave a stable seam for later listener and worker rewiring.

## Options considered

### Option A: Rewire `app/events/listener.go` to a queue immediately

- Pros:
  - Fastest path toward the final tracer bullet.
- Cons:
  - Higher regression risk because current flow, contracts, and terminology are still implicit.
  - Harder to review atomically.

### Option B: Start with docs only

- Pros:
  - Lowest code risk.
- Cons:
  - Does not materially execute the roadmap in code.
  - Leaves the next slice still blocked on missing shared types and queue API.

### Option C: Add contracts, queue primitives, and architecture docs first

- Pros:
  - Small code delta with immediate value.
  - Gives the next slice a stable package to import.
  - Matches phase-0 tasks directly.
- Cons:
  - Does not yet move live traffic through the queue.

## Recommended direction

Choose Option C.

Deliverables for this slice:

- Create `app/moderation` with transport-neutral moderation contracts.
- Add `Queue` plus in-memory channel-backed implementation and unit tests.
- Write an ADR mapping the current repo to the roadmap bounded contexts.
- Create `docs/Architecture.md` as the repo architecture map.
- Update roadmap docs to point to the new artifacts and mark completed phase-0 items.

## Risks

- Contract fields may be too Telegram-shaped or too abstract.
  - Mitigation: keep the types centered on moderation semantics, not Telegram structs.
- Queue API may be harder to evolve if we over-design it now.
  - Mitigation: keep a minimal single-stream API that supports the tracer bullet.

## Acceptance for this slice

- New moderation contracts compile and have queue tests.
- Architecture docs exist and point to real repo modules.
- Phase-0 plan reflects the completed groundwork.
- Change remains behavior-preserving.
