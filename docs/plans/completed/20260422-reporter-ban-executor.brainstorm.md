# 20260422 Reporter Ban Executor Brainstorm

## Goal

Move the remaining report-side direct ban path onto the shared `ActionExecutor`.

## Scope

- `userReports.callbackReportBanReporterConfirm`
- focused report tests

## Approach

1. Reuse the synthetic report moderation metadata helper.
2. Route reporter ban through `ActionExecutor.ApplyBan`.
3. Add tests proving the callback uses the shared executor.
