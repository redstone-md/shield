# Plan: warn command executor

1. Extend `ActionExecutor` with `WarnUser` and add journal/replay support in `telegramActionExecutor`.
2. Route `admin.DirectWarnReport` through shared delete/warn executor calls with synthetic moderation metadata.
3. Ensure listener runtime initializes `ActionExecutor` before admin and report handlers are constructed.
4. Add focused tests for executor warn journaling, admin warn executor usage, and listener `/warn` runtime wiring.
5. Update roadmap/doc artifacts and run focused Go test coverage plus `git diff --check`.
