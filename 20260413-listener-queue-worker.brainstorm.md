# Listener Queue Worker Brainstorm

## Problem

The previous phase-0 slice introduced `app/moderation` contracts and an in-memory queue, but `app/events/listener.go` still executes the moderation flow directly. The roadmap requires Telegram ingestion to publish `IncomingEvent` into the internal queue and a worker to consume it.

## Current facts

- `TelegramListener.Do` still calls `procEvents(update)` directly for regular messages.
- `procEvents` currently mixes:
  - Telegram transport filtering
  - bot message transformation
  - locator writes
  - spam detection
  - reply sending
  - action execution
  - audit-ish logging
- There is already a stable transport-neutral contract package in `app/moderation`.
- Existing focused tests prove the current listener flow and queue package separately.

## Constraints

- Preserve current user-visible behavior.
- Keep the change atomic.
- Do not yet split policy/action/audit into separate packages.
- Keep direct unit tests of `procEvents` workable.

## Options considered

### Option A: Make queue processing fully asynchronous

- Pros:
  - Closer to future architecture.
- Cons:
  - Existing tests and runtime semantics depend on the message being fully processed before the listener loop returns.
  - Shutdown/drain behavior becomes harder immediately.

### Option B: Publish to queue, consume on a worker, and synchronously wait for the event result

- Pros:
  - Makes the queue seam real.
  - Preserves current behavior and test expectations.
  - Keeps the worker implementation small.
- Cons:
  - The caller still blocks until moderation finishes.

## Recommended direction

Choose Option B.

Deliverables:

- Add a queue-backed worker in `app/events`.
- Make `procEvents` responsible for transport adaptation and queue publication.
- Move the current moderation execution body behind a worker processor interface.
- Add tests that prove `IncomingEvent` metadata is built and consumed through the queue path.

## Risks

- Pending event/result coordination may leak if queue lifecycle is mishandled.
  - Mitigation: store one pending result channel per event id and always clean it up in the worker/publish failure path.
- Shutdown may drop queued work.
  - Mitigation: close the queue and wait for the worker to drain before returning from `Do`.
