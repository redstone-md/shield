# Readable Logs Plan

## Scope

In scope:

- Central stdout log formatting.
- Correlation metadata formatting in `app/observability`.
- Focused tests for level preservation and compact metadata.

Out of scope:

- Replacing the logging library.
- Rewriting all moderation log messages.
- Changing spam audit JSON logs.

## Steps

- [x] Read architecture and logging entry points.
- [x] Capture brainstorm and choose a low-blast-radius direction.
- [x] Add regression tests for context-aware log formatting.
- [x] Implement compact metadata and level-preserving `Logf`.
- [x] Simplify stdout logger format in `setupLog`.
- [x] Run focused tests.
- [x] Run formatting and relevant verification.
- [x] Archive plan records under `docs/plans/completed/`.

## Verification

- `GOCACHE=/tmp/go-build go test ./app/observability`
- `GOCACHE=/tmp/go-build go test ./app -run TestMakeSpamLogWriter`

Broader `GOCACHE=/tmp/go-build go test ./app/observability ./app` was attempted, but the full `app` package is blocked in this sandbox by TCP socket permission errors in server/probe tests and an unrelated active-rule-set assertion failure.
