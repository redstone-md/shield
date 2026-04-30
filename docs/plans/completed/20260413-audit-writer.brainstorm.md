# Audit Writer Brainstorm

## Problem

The worker path now has explicit queue, policy, and action seams, but moderation recording still happens inline in `processQueuedEvent` through `SpamLogger` and `Locator.AddSpam`. Phase 0 calls for a separate audit writer.

## Current facts

- The worker currently records spam via:
  - `SpamLogger.Save`
  - `Locator.AddSpam`
- Those writes happen only for actionable spam with `BanInterval > 0`.
- `DetectedSpamCounter` is used for strike counting but is not currently written by the worker path.

## Recommended direction

- Introduce `AuditWriter` in `app/events`.
- Pass it a full `AuditRecord` with:
  - incoming event
  - transformed message
  - detection response
  - policy decision
  - action result
- Keep the default writer behavior-preserving for this slice:
  - write to `SpamLogger`
  - write to `Locator.AddSpam`
- Defer `DetectedSpamCounter.Write` and richer persistence changes to the tracer-bullet audit slice.
