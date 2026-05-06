# Slowpath Reason Notifications Plan

## Checklist

- [x] Add helper for extracting slowpath reason from check results.
- [x] Append reason to auto-warning public chat message.
- [x] Append reason to admin warning and punishment notifications when provided.
- [x] Add or update tests for public warning and admin notifications.
- [x] Run targeted tests.
- [x] Run formatting and relevant verification.

## Follow-up Test Fixes

- [x] Update admin notification tests to current HTML output format.
- [x] Fix approved users migration tenant backfill.
- [x] Update log metadata tests to current compact log keys.
- [x] Preserve active rule set during runtime assembly instead of overwriting with CLI defaults.

## Verification

- `go test ./app/events`
- `go test ./app ./app/storage ./app/webapi`
- `make build`
- `make test`
