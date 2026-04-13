# Foundations Pipeline Seams Plan

## Scope

Execute the first atomic slice of phase 0 from `docs/ROADMAP.md` and `docs/plans/roadmap/00-foundations-and-internal-pipeline.md` without changing live moderation behaviour.

## Baseline

- [x] Read roadmap and phase-0 plan
- [x] Inspect current runtime wiring in `app/main.go`, `app/events`, and `app/bot`
- [x] Focused baseline checks passed with `/tmp/go1.25.3/go/bin/go`
  - `go test ./app/events -run TestTelegramListener_Do -count=1`
  - `go test ./app/bot -run TestSpamFilter_OnMessage -count=1`
- [x] Full relevant verification after changes
  - `gofmt -w app/moderation/*.go`
  - `go test ./app/moderation ./app/events -run 'TestInMemoryQueue|TestTelegramListener_Do' -count=1`
  - `git diff --check`

## Implementation steps

1. [x] Decide the smallest safe roadmap slice: contracts + queue seam + architecture docs
2. [x] Add transport-neutral moderation contracts under `app/moderation`
3. [x] Add `Queue` and in-memory implementation with unit tests
4. [x] Write ADR for bounded-context mapping and internal pipeline seam
5. [x] Create `docs/Architecture.md` and link the ADR/contracts
6. [x] Update roadmap docs to reflect the completed phase-0 groundwork
7. [x] Run focused formatting and tests
8. [x] Review diff for scope, regressions, and file-size limits
9. [ ] Commit with a conventional, atomic message

## Validation steps

1. `gofmt -w app/moderation/*.go`
2. `go test ./app/moderation ./app/events -run 'TestInMemoryQueue|TestTelegramListener_Do' -count=1`
3. Review changed docs for real links and roadmap consistency
4. `git diff --check`

## Final validation skills

- `mcaf-adr-writing`
  - Reason: verify the ADR is explicit about decision, alternatives, and consequences.
- `mcaf-architecture-overview`
  - Reason: verify the architecture map is navigational and grounded in real repo names.

## Notes

- Use the isolated Go 1.25.3 toolchain in `/tmp/go1.25.3/go` for commands in this task.
- The next roadmap slice should rewire `app/events/listener.go` to publish `IncomingEvent` into the queue and add a worker that consumes it.
