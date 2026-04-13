# 2026-04-13 RuleSet Persistence Brainstorm

## Problem

Phase 1 starts with a gap: moderation rules still live in runtime flags only. Before idempotency or control-plane work, the repo needs a persistent single-tenant `RuleSet` entity with bootstrap defaults taken from current options.

## Constraints

- Keep the slice behavior-preserving.
- Do not rewire detectors to read from `RuleSet` yet.
- Introduce durable storage and bootstrap loading only.

## Options

### Option 1: Store raw options in one JSON blob in `app/main`

- Pro: fast to add
- Con: poor domain boundary and hard to evolve into versioned `RuleSet`

### Option 2: Add a small domain package plus storage repository

- Pro: creates the real phase-1 domain seam and keeps persistence out of `app/main`
- Con: slightly more code now

## Decision

Add `app/rules` as the domain package, store active/versioned rules in `app/storage`, and bootstrap the first persisted rule set from existing options during startup.

## Verification Target

- startup persists a bootstrap `RuleSet` when none exists
- repeated startup does not create duplicate versions
- active `RuleSet` can be read back from storage with the expected fields
