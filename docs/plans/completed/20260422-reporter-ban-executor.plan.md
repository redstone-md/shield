# 20260422 Reporter Ban Executor Plan

1. Update `callbackReportBanReporterConfirm` to build report moderation context.
2. Replace direct `banUserOrChannel` call with the shared executor when available.
3. Add focused tests for the executor path.
4. Update roadmap/docs if the report-penalty item is fully closed.
5. Run focused regressions and `git diff --check`.
