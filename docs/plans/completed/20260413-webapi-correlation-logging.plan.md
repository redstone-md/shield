# 2026-04-13 WebAPI Correlation Logging Plan

1. Add request metadata generation and middleware to `app/webapi`.
2. Attach metadata to request context and response headers for all web API routes.
3. Switch request-scoped `app/webapi` logs to `app/observability`.
4. Add focused tests for downstream context propagation and request-scoped logging.
5. Update roadmap and architecture docs to mark the `app/webapi` leg complete.
6. Run targeted Go tests and `git diff --check`.

## Validation Skills

- `mcaf-observability`: confirm request correlation across the `app/webapi` boundary
- `mcaf-testing`: confirm middleware and downstream propagation with automated tests
