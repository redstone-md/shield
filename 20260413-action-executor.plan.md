# Action Executor Plan

## Scope

Extract sanction execution from the queue worker path into a dedicated action executor for phase 0.

## Implementation steps

1. [x] Confirm the remaining action logic inside `processQueuedEvent`
2. [x] Add `ActionExecutor` and default Telegram-backed implementation
3. [x] Move ban/restrict and message deletion helpers into the executor file
4. [x] Switch the worker path to use the executor
5. [x] Add focused executor tests
6. [x] Update roadmap progress and architecture docs
7. [x] Run focused verification
8. [ ] Review and commit atomically

## Validation

1. `gofmt -w app/events/*.go`
2. `go test ./app/events -run 'TestTelegramListener_DoWithBotBan|TestTelegramActionExecutor' -count=1`
3. `git diff --check`
