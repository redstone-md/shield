# 20260422 Report Executor Brainstorm

## Goal

Move report-driven moderation penalties onto the shared `ActionExecutor` so report auto-ban and report approval use the same execution, journaling, and replay path as the main moderation worker.

## Scope

- `userReports.applyImmediateReportModeration`
- `userReports.executeAutoBan`
- `userReports.callbackReportBan`

## Out of Scope

- Admin `/warn` flow
- Reporter-ban callbacks
- Audit enrichment

## Approach

1. Inject `ActionExecutor` into `userReports`.
2. Add a synthetic report moderation context carrying deterministic `event_id`, `correlation_id`, and `idempotency_key`.
3. Route report-driven delete and ban/restrict operations through the shared executor.
4. Verify with focused report tests.
