# Bot Warning Autodelete Plan

## Checklist

- [x] Add warning delete duration to admin handler state.
- [x] Pass warning delete duration in direct `/warn` path.
- [x] Update rule-set live reload propagation.
- [x] Make direct `/warn` use the same strike count/escalation semantics as automatic warnings.
- [x] Add or update regression tests.
- [x] Run targeted events tests and relevant gates.

## Verification

- `go test ./app/events` passed.
- `go test ./app` passed.
- `make build` passed; Makefile printed existing `date: 1778069270: No such file or directory` warning.
- `make test` passed.
- `golangci-lint run` not run: `golangci-lint` is not installed in this environment.
