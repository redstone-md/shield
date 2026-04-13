# 2026-04-13 Correlation Logging Plan

1. Add a small observability helper for `event_id` and `correlation_id` context propagation and log formatting.
2. Stamp moderation event metadata into the worker context in `app/events/pipeline.go`.
3. Propagate the context into detection and action execution without breaking existing non-pipeline call sites.
4. Update moderation-path logs in `app/bot` and `app/storage` to include metadata when present in context.
5. Extend tracer-bullet style tests to assert metadata propagation through detection, action, audit, and locator calls.
6. Run targeted tests and `git diff --check`.

## Validation Skills

- `mcaf-observability`: confirm cross-boundary correlation is possible in the moderation path
- `mcaf-testing`: confirm the path is covered by stable automated tests
