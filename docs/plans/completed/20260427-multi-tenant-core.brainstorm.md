# Brainstorm: Multi-tenant Core (Stage 3)

Date: 2026-04-27

## Current State

1. **`gid` (group ID)** — every table has a `gid TEXT` column; `engine.SQL.GID()` returns it; all queries filter by it.
2. **`InstanceID`** — CLI flag `--instance-id` / env `INSTANCE_ID`, defaults to `"tg-spam"`. Passed as `gid` to `engine.New()`.
3. **Workspaces** — `workspaces` + `workspace_members` tables exist with roles (`owner/admin/viewer`). Bootstrap creates one workspace named `InstanceID`.
4. **`tenant_id`** — only `incoming_events` has it; written but never queried.
5. **`lib/tgspam/`** — zero tenant awareness; pure in-memory detection engine.

### Isolation Model Today
- **SQLite**: one file per group → physical isolation.
- **Postgres**: shared DB, `gid` column filtering → logical isolation at query level.
- **No routing middleware**: webapi uses `Settings.InstanceID` for authz.

## Key Design Questions

### Q1: Is `tenant_id` a replacement for `gid` or an addition?
**Option A**: Rename `gid` → `tenant_id` everywhere. Single tenancy concept.
- Pros: simpler schema, fewer columns, less confusion.
- Cons: migration complexity, breaks existing API contract.

**Option B**: Add `tenant_id` alongside `gid`. `gid` becomes migration bridge.
- Pros: backward compatible, gradual migration.
- Cons: two similar columns, potential for inconsistency.

**Option C**: `tenant_id` is a higher-level concept. `gid` stays for storage isolation. `tenant_id` maps to one or more `gid` values.
- Pros: flexible, supports future workspace-within-tenant.
- Cons: indirection, more complex.

**Recommendation**: **Option A with migration bridge**. Add `tenant_id` column to all tables, migrate `gid` → `tenant_id` values, then deprecate `gid`. During transition, code checks `tenant_id` first, falls back to `gid`. This is cleanest long-term.

### Q2: How does the web API know which tenant a request belongs to?
**Option A**: Tenant ID in URL path: `/api/v1/{tenant_id}/approved-users`.
- Pros: explicit, cacheable.
- Cons: breaks existing routes, verbose URLs.

**Option B**: Tenant ID from auth token/session.
- Pros: clean URLs, standard SaaS pattern.
- Cons: requires auth middleware first.

**Option C**: Tenant ID from config (`InstanceID`) — single-tenant mode.
- Pros: backward compatible.
- Cons: doesn't enable multi-tenant.

**Recommendation**: **Start with Option C (backward compat), evolve to Option B**. In this stage, keep `InstanceID`-based routing but ensure all code paths use `tenant_id` concept. Actual multi-tenant routing is a later stage concern.

### Q3: Do we need the full tenant table structure now?
The roadmap calls for: `tenants`, `tenant_chats`, `tenant_memberships`, `tenant_limits`, `tenant_statuses`.

**Assessment**: Most of these map to concepts we already have or can defer:
- `tenants` → already have `workspaces` (close enough for MVP)
- `tenant_chats` → new, but can derive from pipeline events
- `tenant_memberships` → already have `workspace_members`
- `tenant_limits` → defer to Stage 4/5 (quotas)
- `tenant_statuses` → `workspaces.status` already exists

**Recommendation**: Introduce a `tenants` table as the canonical tenant registry. Reuse `workspaces` for membership. Defer `tenant_chats`, `tenant_limits` to later stages.

### Q4: Does `lib/tgspam/` need tenant awareness?
**No.** The library is a pure detection engine. Tenant isolation happens at the orchestration layer (`app/`), not in the library. Each tenant gets its own `Detector` instance or the caller passes tenant-scoped data.

## Slicing Strategy

Given the roadmap has 12 tasks and 3 completion criteria, here's the proposed slice order:

### Slice 1: Tenant Schema
- Create `tenants` table in storage
- Add `tenant_id` column to all existing tables (alongside `gid`)
- Migration: backfill `tenant_id = gid` for existing rows
- **Goal**: Every row has both `gid` and `tenant_id`

### Slice 2: Tenant Repository
- `TenantStore` interface + implementation (CRUD for tenants)
- `TenantID()` accessor on engine or context
- **Goal**: Can create/list/get tenants through Go API

### Slice 3: Storage Layer Switch (gid → tenant_id)
- All WHERE clauses: add `tenant_id = ?` alongside `gid = ?`
- All INSERT: set `tenant_id` from context or config
- All UNIQUE constraints: include `tenant_id`
- **Goal**: Queries filter by `tenant_id`, `gid` is still set for compat

### Slice 4: Domain Contracts
- Thread `tenantID` through control plane services
- Thread `tenantID` through pipeline (events, moderation)
- Thread `tenantID` through webapi handlers
- **Goal**: All Go-level contracts explicitly pass `tenantID`

### Slice 5: Cache Keys
- `RuleSet` cache: key = `{tenantID}:{workspaceID}`
- Policy profile cache: tenant-scoped
- **Goal**: Cache isolation between tenants

### Slice 6: Tenant-Aware Auth
- WebAPI middleware resolves tenant from `InstanceID` (single-tenant mode)
- Authz checks include tenant scope
- **Goal**: All control plane operations are tenant-scoped

### Slice 7: Migration Bridge
- Single-tenant → first tenant: copy `gid` values to `tenant_id`
- Ensure existing single-tenant deployments continue working
- **Goal**: Zero-downtime migration path

### Slice 8: Isolation Integration Tests
- Two tenants in shared DB, verify no data leakage
- Test every storage module for cross-tenant access
- Test that queries without `tenant_id` scope fail
- **Goal**: Completion criteria 1 & 3 verified

### Slice 9: Soft-Delete & Offboarding
- `tenants.status` field: `active`, `suspended`, `deleted`
- Soft-delete: set status, purge cache
- **Goal**: Safe tenant lifecycle

### Slice 10: Federation Hooks (Stub)
- Interface for shared bans / inherited policies
- Not enabled by default
- **Goal**: Future extensibility point

## Risks

1. **Schema migration on large tables** — adding `tenant_id` to millions of rows is slow. Mitigation: do it column-by-column with background migration.
2. **Breaking existing single-tenant deployments** — Mitigation: `tenant_id` defaults to `gid`, fallback logic in queries.
3. **Test explosion** — every storage test now needs multi-tenant variants. Mitigation: parameterized test helpers.
4. **LOC budget** — storage files are already large (samples.go: 491 LOC). Adding tenant logic may push them over 500. Mitigation: extract tenant-aware query builders.

## Open Questions

1. Should `tenant_id` be a UUID, auto-increment integer, or string (like current `gid`)?
2. Should we rename `engine.SQL.GID()` → `TenantID()` or keep both?
3. Is the workspace → tenant relationship 1:1 or 1:N?

## Proposed Answers

1. **String** (TEXT) — consistent with `gid`, easy migration, no schema changes for key type.
2. **Keep both during transition** — add `TenantID()` method, deprecate `GID()` later.
3. **1:1 for now** — one workspace per tenant. Evolve to 1:N in later stages.
