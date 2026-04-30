# 2026-04-13 Text Normalization Seam Brainstorm

## Problem

Text normalization still lives implicitly inside `lib/tgspam/detector.go` as ad hoc lower-casing, invisible-character stripping, and whitespace cleanup. That makes normalization hard to test or evolve independently from detector internals.

## Constraints

- Keep moderation behavior stable for the current detector path.
- Extract a reusable normalization seam without dragging in broader replay or action-journal work.
- Preserve room for future script-folding hooks.

## Decision

Add a small shared `lib/textnorm` package with explicit stages for lower-case, trim, invisible-character cleanup, canonical whitespace, and optional script folding. Rewire detector cleanup helpers to use it while keeping current bot request shape unchanged.

## Verification Target

- the normalizer stages are covered directly in package tests
- detector invisible-character cleanup still behaves the same
- existing bot and detector tests stay green with the extracted seam
