# Readable Logs Brainstorm

## Current State

- Runtime logs use the shared Go logger configured in `app/main.go`.
- Context-aware logs go through `app/observability.Logf`.
- `Logf` currently prepends metadata before the message level, so `[DEBUG]` messages become default `[INFO]` lines with a nested `[DEBUG]` token.
- Debug mode adds both caller file and caller function, which makes high-volume moderation logs wide and hard to scan.

## Target Outcome

- Preserve correlation across moderation flows.
- Make log lines shorter and easier to scan in Docker/stdout.
- Keep debug filtering correct.
- Avoid changing every individual logging call.

## Options

1. Replace logging with a structured logger.
   - More flexible long term.
   - Higher blast radius and likely dependency/API churn.

2. Keep current logger, fix central formatting and context prefixing.
   - Smallest change.
   - Fixes the visible `[INFO] ... [DEBUG]` problem.
   - Keeps existing log level conventions.

3. Rewrite noisy call sites into structured domain events.
   - Best long-term readability for moderation decisions.
   - Too broad for the immediate complaint.

## Recommended Direction

Use option 2 now:

- Move correlation metadata after the level token in `observability.Logf`.
- Shorten metadata keys to `evt`, `corr`, and `idem`.
- Use a concise stdout format from `setupLog`.
- Remove caller location from stdout formatting so high-volume moderation logs stay compact.

## Risks

- Tests that assert exact log formatting may need updates.
- Operators relying on full timestamp in app logs will now mostly use the container timestamp plus compact app time.
