# Brainstorm: Formal schema migration for rule_sets, rule_set_versions, incoming_events, moderation_actions

## Problem

Roadmap phase-1 task "Создать миграцию для таблиц rule_sets, rule_set_versions, incoming_events, moderation_actions" is the last unchecked item. All four tables already have `CREATE TABLE IF NOT EXISTS` DDL and are wired into `InitTable`, but there is no explicit migration logic that handles upgrades from a prior (possibly missing-column) schema to the current canonical schema. Two of the four have no-op `migrate()`, and `incoming_events` has a probe-style migration for six columns that were added incrementally.

## Current state

| Table | DDL | migrate() | Risk |
|---|---|---|---|
| `rule_sets` + `rule_set_versions` | Full DDL in `CmdCreateRuleSetsTables` (both tables in one command) | `return nil` | If old DB has table without `gid`, migration won't add it |
| `incoming_events` | Full DDL in `CmdCreateIncomingEventsTable` (all columns included) | Probe `processed_at IS NULL` → iterates 6 ALTER TABLE ADD COLUMN commands | Works but probes by selecting from table with a column that might not exist; also, CREATE TABLE already includes all columns so new installs are fine |
| `moderation_actions` | Full DDL in `CmdCreateModerationActionsTable` | `return nil` | Same risk as rule_sets — no column evolution handling |

## Options

1. **Keep no-op migrate, rely on CREATE TABLE IF NOT EXISTS** — simplest, but doesn't handle column additions on existing databases
2. **Add probe-style migration for each table** — consistent with `detected_spam` and `approved_users` pattern; probe for newest column, ALTER TABLE for any missing
3. **Add a version tracking table** — overkill for single-tenant; the project pattern is probe-based
4. **Consolidate migration into a single migration runner** — breaks the per-table pattern

## Decision

Option 2. Each table gets a proper `migrate()` that:
- Probes for the newest/most-recently-added column by attempting a SELECT with it
- If the probe succeeds → already migrated, return
- If the probe fails → iterate ALTER TABLE ADD COLUMN for any columns that might be missing from older installs

This matches the existing `detected_spam` and `incoming_events` patterns and is idiomatic for the codebase.

## Columns that need migration coverage

### rule_sets
- `gid` — added for multi-group support; existing single-group DBs might not have it

### rule_set_versions
- `gid` — same reason as rule_sets

### incoming_events
Already has migration for: `decision_action`, `decision_reason`, `decision_score`, `action_applied`, `action_error`, `processed_at`
- `gid` — if the table was created before gid was added
- `correlation_id` — might be missing in very old installs
- `tenant_id` — same

### moderation_actions
- `gid` — might be missing
- `correlation_id` — might be missing
- `idempotency_key` — might be missing

## DBCmd namespace collision

Currently `rule_sets` (iota+500) collides with `samples` (iota+500) and `reports` (iota+500). Need to renumber to avoid silent overwrites in the query map.
