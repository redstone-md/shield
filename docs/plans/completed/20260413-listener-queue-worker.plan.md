# Listener Queue Worker Plan

## Scope

Execute the next atomic phase-0 slice: `app/events/listener.go` publishes `IncomingEvent` into the internal queue and a worker consumes it while preserving current behavior.

## Baseline

- [x] Existing seam contracts and queue package are in place
- [x] Focused listener baseline passed before this slice
  - `go test ./app/moderation ./app/events -run 'TestInMemoryQueue|TestTelegramListener_Do' -count=1`

## Implementation steps

1. [x] Confirm the direct-call seam location in `procEvents`
2. [x] Add queue worker state and lifecycle management to `TelegramListener`
3. [x] Convert `procEvents` into adaptation + publish-to-queue
4. [x] Move the old moderation body behind a worker processor interface
5. [x] Add tests for event contract publication and queue-backed listener behavior
6. [x] Update phase-0 roadmap progress
7. [x] Run focused formatting and tests
8. [ ] Review and commit atomically

## Validation steps

1. `gofmt -w app/events/*.go`
2. `go test ./app/events ./app/moderation -run 'TestTelegramListener_Do|TestTelegramListener_ProcEventsPublishesIncomingEvent|TestInMemoryQueue' -count=1`
3. `git diff --check`

## Notes

- Keep behavior-preserving synchronous semantics by waiting for worker completion per event.
- Defer separate policy/action/audit packages to later phase-0 slices.
