# Action Executor Brainstorm

## Problem

The queue worker exists, but the worker path in `app/events/pipeline.go` still applies sanctions directly through Telegram API calls and listener helper methods. Phase 0 explicitly calls for moving sanction application out of `app/events` into an action executor.

## Current facts

- `processQueuedEvent` still does:
  - ban/restrict execution
  - extra message deletion
  - reply deletion
- `banUserOrChannel` is a free function in `app/events/events.go`.
- `deleteExtraMessages` is still a `TelegramListener` method in `app/events/listener.go`.
- `admin.go` and `reports.go` also call `banUserOrChannel`.

## Recommended direction

- Introduce `ActionExecutor` in `app/events/action_executor.go`.
- Move Telegram action application primitives there:
  - `ApplyBan`
  - `DeleteMessage`
  - `DeleteExtraMessages`
- Reuse the same executor from the worker path.
- Keep admin and reports on the shared `banUserOrChannel` helper for now, but relocate that helper into the executor file so action-specific code is co-located.

## Why this slice is small enough

- No policy or audit behavior changes.
- Existing listener tests already cover the main ban/delete paths.
- The change reduces event-layer responsibility without changing runtime contracts.
