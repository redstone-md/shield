# Audit Writer Plan

## Scope

Extract moderation-result recording from the queue worker path into a dedicated audit writer without changing current runtime behavior.

## Implementation steps

1. [x] Confirm the remaining audit writes inside `processQueuedEvent`
2. [x] Add `AuditWriter` and default implementation
3. [x] Switch the worker path to emit an `AuditRecord`
4. [x] Add focused audit-writer tests
5. [x] Update roadmap and architecture docs
6. [x] Run focused verification
7. [ ] Review and commit atomically

## Validation

1. `gofmt -w app/events/*.go`
2. `go test ./app/events -run 'TestDefaultAuditWriter|TestTelegramListener_DoWithBotBan|TestTelegramListener_ProcEventsPublishesIncomingEvent' -count=1`
3. `git diff --check`
