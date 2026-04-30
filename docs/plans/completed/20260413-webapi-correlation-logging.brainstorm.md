# 2026-04-13 WebAPI Correlation Logging Brainstorm

## Problem

The phase-0 roadmap item for `event_id` and `correlation_id` is still open because `app/webapi` requests do not attach correlation metadata to request contexts or request-scoped logs.

## Constraints

- Keep the change atomic and local to `app/webapi`.
- Reuse the existing `app/observability` helper instead of inventing another metadata format.
- Avoid changing unrelated handler behavior or the admin UI.

## Options

### Option 1: Patch individual handlers only

- Pro: small edits
- Con: no single source of truth for request metadata and easy to miss routes

### Option 2: Add one request middleware and propagate from request context

- Pro: consistent across all routes, fits existing `r.Context()` usage, easy to verify with mocks
- Con: requires light handler updates for request-scoped logs

## Decision

Add a request metadata middleware in `app/webapi` that generates or adopts `event_id` and `correlation_id`, stores them in request context, and returns them in response headers. Update request-scoped logs to use `app/observability`.

## Verification Target

- HTTP requests through `app/webapi` carry correlation metadata in context and response headers
- downstream storage calls receive the same metadata
- bad-request logging includes the metadata prefix
