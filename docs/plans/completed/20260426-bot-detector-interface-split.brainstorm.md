# Bot Detector Interface Split Brainstorm

## Current State

`app/bot/mocks/detector.go` is the only remaining Go file over `file_max_loc: 500`. It is generated from `app/bot.Detector`, which currently mixes message checking, sample loading, sample mutation, approved-user management, Lua plugin listing, and runtime config mutation.

## Problem

The generated mock is large because the source interface is too broad. Manually splitting generated output would be overwritten by `go generate` and would not improve the production boundary.

## Options

1. Split `bot.Detector` into smaller role interfaces and generate mocks for only the roles used by bot tests.
   - Pros: fixes the cause, improves dependency clarity, smaller generated files.
   - Cons: requires updating tests and constructor wiring.

2. Keep `Detector` broad and document a generated-code exception.
   - Pros: quick.
   - Cons: leaves the maintainability violation and broad contract.

3. Remove generated mocks and hand-write a test fake.
   - Pros: smallest file count.
   - Cons: diverges from repository moq pattern.

## Recommended Direction

Use role interfaces in `app/bot`:

- `MessageChecker` for `Check`.
- `SampleLoader` for `LoadSamples` and `LoadStopWords`.
- `SampleUpdater` for sample update/remove methods.
- `ApprovedUsers` for approval methods.

Keep `Detector` as a composed compatibility interface for callers passing `*tgspam.Detector`, but store the role interfaces on `SpamFilter`. Generate mocks for the small roles only.

## Risks

- Tests that currently rely on `DetectorMock` need targeted updates.
- Runtime assembly type assertions to `*tgspam.Detector` may need a concrete detector field.
- `moq` may not be installed; use repo Go tooling rather than manual generated edits.

