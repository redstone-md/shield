# Multiline Debug Configs Plan

## Scope

In scope:

- Startup debug config readability.
- Tests for the formatter.
- Focused verification for touched packages.

Out of scope:

- A full structured logging migration.
- Changing spam detection or moderation behavior.
- Changing Docker logging driver behavior.

## Steps

- [x] Inspect current startup debug config call sites.
- [x] Add a central multiline field formatter.
- [x] Apply it to the noisy config logs.
- [x] Replace admin handler pointer dump with explicit fields.
- [x] Add focused tests.
- [x] Run formatting and verification.

## Verification

- `GOCACHE=/tmp/go-build go test ./app/observability`
- `GOCACHE=/tmp/go-build go test ./app/observability ./app ./app/events -run 'TestFormatFields|TestMakeSpamLogWriter'`

Broader `GOCACHE=/tmp/go-build go test ./app/events` was attempted, but it is blocked by unrelated existing admin message expectation failures and sandbox TCP socket restrictions in `httptest`.
