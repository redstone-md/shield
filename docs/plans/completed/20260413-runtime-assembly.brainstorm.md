# 2026-04-13 Runtime Assembly Brainstorm

## Problem

Phase 0 is almost complete, but `app/main.go` still assembles the runtime as one direct chain of concrete constructions. The roadmap completion criteria require `app/main.go` to build from domain seams rather than one inline graph.

## Constraints

- Keep the change behavior-preserving.
- Avoid widening public interfaces without need.
- Improve cohesion by moving wiring code out of `execute`, not by introducing framework-heavy abstractions.

## Options

### Option 1: Leave `execute` as the orchestrator

- Pro: no new types
- Con: keeps storage, gateway, bot, and web wiring coupled in one function

### Option 2: Extract a runtime assembly object with explicit subassemblies

- Pro: makes startup phases and dependencies explicit, improves testability, and narrows `app/main.go`
- Con: adds a few new helper types

## Decision

Extract a small runtime assembly layer in package `main` that prepares storage-backed services, telegram gateway wiring, and web runtime dependencies behind explicit structs. Keep `execute` focused on orchestration and lifecycle.

## Verification Target

- existing startup tests still pass
- server-only runtime still behaves the same
- listener wiring remains queue-backed and behavior-preserving
