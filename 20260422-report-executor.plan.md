# 20260422 Report Executor Plan

1. Extend `userReports` to accept `ActionExecutor` and wire it from `TelegramListener`.
2. Add a helper that builds deterministic observability metadata for report moderation actions.
3. Replace direct report-path delete/ban calls with `ActionExecutor.DeleteMessage` and `ActionExecutor.ApplyBan`.
4. Add focused tests proving report approval and auto-ban use the executor.
5. Update architecture docs and roadmap status if needed.
6. Run focused regressions and `git diff --check`.
