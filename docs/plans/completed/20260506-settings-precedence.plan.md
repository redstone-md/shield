# Settings Precedence Plan

## Checklist

- [x] Add explicit CLI/env detection for rule-set-backed options.
- [x] Apply explicit overrides after DB active rule set load and during live reload.
- [x] Update runtime tests for explicit override behavior.
- [x] Document config precedence in architecture overview and AGENTS preferences.
- [x] Run targeted tests, then build/test gates.

## Verification

- `go test ./app/events` passed.
- `go test ./app` passed.
- `make build` passed; Makefile printed existing `date: 1778069270: No such file or directory` warning.
- `make test` passed.
- `golangci-lint run` not run: `golangci-lint` is not installed in this environment.
