# Vision Spam Info Plan

## Scope

- In scope: admin notification reason extraction and info callback fallback for existing diagnostics.
- Out of scope: new storage schema, new dependencies, frontend changes.

## Steps

- [x] Add regression tests for vision provider summaries in notifications.
- [x] Add regression test for `callbackShowInfo` using diagnostics already present in the admin message when locator misses.
- [x] Update helper logic to extract slow-path/provider vision reasons safely.
- [x] Update callback fallback logic without changing callback contracts.
- [x] Run focused `go test ./app/events`.
- [x] Run broader verification if focused tests pass.

## Verification

- `go test ./app/events`
- Broader repo gate if time and dependencies allow: `make test`

## Results

- Focused regression: `env GOCACHE=/tmp/go-build go test ./app/events -run 'TestSlowpathReasonIncludesVisionProviderChecks|TestAdminCallbackShowInfoFallsBackToExistingDiagnostics'`
- Related suite: `env GOCACHE=/tmp/go-build go test ./app/events`
