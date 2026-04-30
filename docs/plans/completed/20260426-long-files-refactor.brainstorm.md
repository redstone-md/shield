# Refactor Long Files Brainstorm

## Current State

The repository maintainability limit is `file_max_loc: 500`. Current Go offenders are mostly package-level test files:

- `app/events/listener_test.go`
- `lib/tgspam/detector_test.go`
- `app/webapi/webapi_test.go`
- `app/events/reports_test.go`
- `app/events/admin_test.go`
- `app/storage/samples_test.go`
- `app/bot/spam_test.go`
- `app/storage/engine/convert_test.go`
- `app/main_test.go`
- `app/events/events_test.go`
- `app/bot/mocks/detector.go`
- `lib/tgspam/plugin_test.go`
- `lib/tgspam/duplicate_test.go`
- `app/storage/approved_users_test.go`
- `lib/tgspam/metachecks_test.go`
- `lib/tgspam/openai_test.go`
- `app/storage/storage_test.go`
- `app/storage/engine/engine_test.go`

Non-Go files also exceed 500 LOC (`README.md`, site docs, completed plans, `go.sum`, local DB), but those are outside the primary code refactor scope. `data/tg-spam.db.local` is local data and should not be treated as source.

## Problem

Large test files hide independent behavioural areas, increase merge conflicts, and violate repository limits. The highest-risk file is the generated mock `app/bot/mocks/detector.go`; hand-editing generated moq output would create a maintenance trap because regeneration will overwrite manual splits.

## Options

1. Split long test files by package behaviour into multiple smaller `_test.go` files.
   - Pros: no runtime behaviour change, keeps tests near package boundaries, low risk.
   - Cons: imports need to be preserved or gofmt/go test will fail.

2. Refactor production interfaces to reduce generated mock size.
   - Pros: fixes the generated mock at the source.
   - Cons: changes public package contracts and may need broad code updates. This is too risky for the first tracer bullet.

3. Document a generated-code exception for `app/bot/mocks/detector.go`.
   - Pros: truthful and maintainable.
   - Cons: leaves one Go file over the limit until the `bot.Detector` interface is intentionally split.

## Recommended Direction

Use a tracer bullet: mechanically split long handwritten Go test files by top-level declarations into cohesive files with shared package scope. Do not change runtime code. Keep the generated moq file as an explicit remaining item unless a later interface split is approved.

## Constraints

- Keep each edited Go file under 500 LOC.
- Preserve package names and package-level helper visibility.
- Do not touch existing unrelated dirty working-tree files.
- Do not manually edit generated mock internals.
- Use repository commands from `AGENTS.md`: `make test` for broad verification.

## Risks

- Import sets can become stale after moving tests.
- Splitting helper declarations away from their first use can still work in Go package scope, but files must remain in the same package.
- Broad `make test` may expose pre-existing unrelated failures; package-local tests should run first.

