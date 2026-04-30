# Policy Engine Plan

## Scope

Extract the moderation decision from the worker path into a dedicated policy engine with explicit `allow/delete/restrict/ban` outcomes.

## Implementation steps

1. [x] Confirm the remaining policy logic inside `processQueuedEvent`
2. [x] Add a default policy engine and tests
3. [x] Wire the worker path to call the policy engine after detection
4. [x] Switch action execution to follow the policy decision instead of inline branching
5. [x] Update roadmap and architecture docs
6. [x] Run focused verification
7. [ ] Review and commit atomically

## Validation

1. `gofmt -w app/events/*.go`
2. `go test ./app/events -run 'TestDefaultPolicyEngine|TestTelegramListener_DoWithBotBan|TestTelegramListener_ProcEventsPublishesIncomingEvent' -count=1`
3. `git diff --check`
