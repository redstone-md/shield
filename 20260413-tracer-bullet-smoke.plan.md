# Tracer Bullet Smoke Plan

## Scope

Add a focused smoke test that proves the internal moderation tracer bullet executes through queue, worker, detection, policy, action, and audit in order.

## Implementation steps

1. [x] Confirm the existing seam injection points on `TelegramListener`
2. [x] Add a tracer-bullet listener smoke test with injected spies
3. [x] Update roadmap and architecture docs
4. [x] Run focused verification
5. [ ] Review and commit atomically

## Validation

1. `gofmt -w app/events/*.go`
2. `go test ./app/events -run 'TestTelegramListener_TracerBulletSmoke|TestDefaultAuditWriter|TestDefaultPolicyEngine' -count=1`
3. `git diff --check`
