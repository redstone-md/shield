# Bot Detector Interface Split Plan

## Goal

Remove the remaining over-500-LOC generated bot mock by splitting the source interface that drives generation.

## Plan

1. Split `app/bot.Detector` into role interfaces while preserving `NewSpamFilter(detector Detector, params SpamConfig)`.
2. Update `SpamFilter` internals to depend on explicit role fields instead of embedding the broad interface.
3. Update runtime assembly/tests for any code that expected `SpamFilter.Detector`.
4. Regenerate bot mocks for the small interfaces.
5. Update bot tests to use role-specific mocks.
6. Run `gofmt`, recount Go LOC, run focused tests, then `make test`.
7. Commit only scoped changes.

## Validation Skills

- `mcaf-solid-maintainability`: verify the large generated file is gone and role interfaces are cohesive.
- `mcaf-testing`: verify bot behaviour and full repo regression.

## Progress

- [x] Identify broad source interface.
- [x] Split interfaces.
- [x] Regenerate mocks.
- [x] Update tests and callers.
- [x] Recount LOC.
- [x] Run focused tests.
- [x] Run broad tests.
- [x] Commit scoped changes.

## Notes

- Recount after deleting `app/bot/mocks/detector.go` leaves no Go files over 500 LOC.
- Focused checks passed: `go test ./app/bot`, `go test ./app/bot ./app/webapi`, `go test ./app`.
- Broad verification passed: `make test` (`go test -race -coverprofile=coverage.out ./...`, total coverage 81.2%).
