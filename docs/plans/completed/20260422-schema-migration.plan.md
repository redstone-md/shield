# Plan: Formalize Schema Migrations for Phase-1 Tables

Task: Create explicit migration logic for `rule_sets`, `rule_set_versions`,
`incoming_events`, `moderation_actions` so that existing databases created
before these tables get them on upgrade, and new databases get the full schema
on first run.

## Current State

All four tables already have `CREATE TABLE IF NOT EXISTS` DDL in the Go code
and are wired into `InitTable` via `TableConfig`. The DDL includes all columns
for new databases. However:

- `rule_sets` / `rule_set_versions`: `migrate()` is a no-op. This means
  existing databases that predate these tables won't get them, because
  `CREATE TABLE IF NOT EXISTS` inside `InitTable` handles creation — but only
  because `InitTable` runs the CREATE TABLE DDL before the migration function.
  **So for new installs, the tables are created correctly.** The risk is for
  databases that have an *older* version of the `rule_sets` or
  `rule_set_versions` schema (columns missing).

- `incoming_events`: Has probe-style migration that checks for `processed_at`
  column and runs `ALTER TABLE ADD COLUMN` for 6 columns that were added
  incrementally. The DDL now includes all columns, so the migration is only
  needed for existing databases that have the older schema without those
  columns.

- `moderation_actions`: `migrate()` is a no-op. Same situation as rule_sets.

**Key insight**: Since `InitTable` runs `CREATE TABLE IF NOT EXISTS` first,
all four tables will be created with the full current schema on a fresh
database. The migration logic is only needed for **existing databases** that
were created with an earlier schema version.

## Design Decision

The project uses a "probe-then-ALTER" migration pattern (see `detected_spam.go`,
`locator.go`, `approved_users.go`). Each `migrate()` function:

1. Probes for the presence of a new column via a `SELECT` that references it.
2. If the probe succeeds (no error), migration is already applied → return.
3. If the probe fails (column doesn't exist), run ALTER TABLE ADD COLUMN
   commands one by one, tolerating "duplicate column" errors.

This is the pattern we should formalize for the four phase-1 tables. Since
the current DDL already includes all columns, the migration only needs to
handle the upgrade path from a hypothetical older schema.

For `rule_sets` and `rule_set_versions`, the tables themselves are new
(phase-1 additions), so the migration is straightforward: if the table
doesn't exist yet, `CREATE TABLE IF NOT EXISTS` handles it. But we should
add probe-style migration for future columns.

For `incoming_events`, the migration already exists and works. We just need
to ensure it's correct and add a clear comment marking it as the formal
migration.

For `moderation_actions`, the table is new (phase-1 addition), same as
rule_sets.

**Conclusion**: The real work is:

1. Add `ALTER TABLE ADD COLUMN` query entries to `rule_sets.go` and
   `moderation_actions.go` for any columns that could be missing in an
   older schema. Currently, both tables have all columns in the DDL, and
   there is no older schema version in the wild. However, we should add
   probe-style migrations for robustness and to satisfy the roadmap item.

2. Since the tables are brand new (no older schema in the wild), the
   migration can simply verify the table has the expected structure. We
   can add a probe for a column that distinguishes the "full" schema from
   a hypothetical "initial" schema.

3. Add migration tests that create an "old" schema (without some columns),
   then verify the migration adds them.

## Implementation Steps

1. **`rule_sets.go`**: Add probe-based `migrate()` that checks for
   `updated_at` column. If missing, run ALTER TABLE ADD COLUMN. Add DBCmd
   entries for the ALTER queries. Also add a `CmdAddGidColumn` for
   `rule_sets` and `rule_set_versions` in case they were created without
   `gid`.

2. **`moderation_actions.go`**: Add probe-based `migrate()` that checks for
   `attempt` column. If missing, run ALTER TABLE ADD COLUMN for any missing
   columns. Add DBCmd entries for the ALTER queries.

3. **`incoming_events.go`**: The migration already exists. Clean it up with
   a clear marker comment. Ensure it probes correctly.

4. **Tests**: For each table, add a test that:
   - Creates the table with a minimal "old" schema (missing some columns)
   - Calls `New*()` which triggers `InitTable` + `migrate()`
   - Verifies all columns exist and the table is functional

5. **Documentation**: Update roadmap checkbox, Architecture.md.

6. **Verify**: Run full test suite.
