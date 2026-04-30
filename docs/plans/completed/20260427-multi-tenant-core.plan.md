# Plan: Multi-tenant Core (Stage 3)

Date: 2026-04-27
Roadmap: `docs/plans/roadmap/03-multi-tenant-core.md`

## Completion Criteria

1. Two tenants can moderate different chats in parallel without config/history overlap.
2. Every mutable and queryable entity has an explicit `tenant_id`.
3. Isolation tests fail on any query without tenant scope.

## Design Decisions

1. `tenant_id` is TEXT — consistent with `gid`, zero-friction migration.
2. `tenant_id` **replaces** `gid` as the primary filter. `gid` column stays but is only written, never queried. Migration bridge: backfill `tenant_id = gid`.
3. `engine.SQL` gains `TenantID() string` alongside `GID()`. During transition, `TenantID()` returns `GID()`. Storage modules migrate query-by-query.
4. 1:1 workspace → tenant. Workspace `name` becomes tenant identifier.
5. New `tenants` table as canonical registry. Existing `workspaces` continue for membership.
6. `lib/tgspam/` stays tenant-agnostic. Isolation is at orchestration layer.
7. WebAPI continues using `InstanceID`-based routing in this stage. Multi-tenant routing is deferred.

## Slice Order

### Slice 1: Tenant table + storage module
**Files**: `app/storage/tenants.go`, `app/storage/tenants_test.go`
- Create `tenants` table: `id SERIAL, name TEXT UNIQUE, gid TEXT, status TEXT DEFAULT 'active', created_at, updated_at`
- `TenantStore` struct with `Add(ctx, name)`, `Get(ctx, name)`, `GetByID(ctx, id)`, `List(ctx)`, `UpdateStatus(ctx, name, status)`
- Engine migration: no-op (new table)
- Tests: CRUD + status transitions

### Slice 2: Add `tenant_id` column to all tables
**Files**: All migration functions in `app/storage/*.go`
- Add `tenant_id TEXT NOT NULL DEFAULT ''` to every table via ALTER TABLE in migration funcs
- Backfill: `UPDATE table SET tenant_id = gid WHERE tenant_id = ''`
- Add indexes on `tenant_id` where needed
- No query changes yet — column exists but queries still use `gid`
- Tests: verify column exists after migration

### Slice 3: Storage query migration (gid → tenant_id)
**Files**: All storage modules in `app/storage/*.go`
- Replace `WHERE gid = ?` with `WHERE tenant_id = ?` in every query
- Replace `s.GID()` with `s.TenantID()` in query params
- Keep setting `gid` on INSERT for backward compat
- Update UNIQUE constraints to use `tenant_id` instead of `gid`
- Per-module, in order: `approved_users`, `dictionary`, `samples`, `detected_spam`, `locator`, `incoming_events`, `moderation_actions`, `reports`, `rule_sets`, `workspaces`
- Tests: existing tests continue passing (same `gid` = same `tenant_id`)

### Slice 4: Thread tenantID through control plane
**Files**: `app/controlplane/*.go`, `app/runtime_assembly.go`
- Add `tenantID` parameter to service constructors and methods
- `ApprovedUsersService`, `DictionaryService`, `DetectedSpamService`, `RuleSetService`, `WorkspaceService`
- Wire in `runtime_assembly.go`: pass `opts.InstanceID` as `tenantID`
- Cache keys: `{tenantID}:{workspaceID}`
- Tests: unit tests with explicit tenantID

### Slice 5: Thread tenantID through pipeline
**Files**: `app/events/*.go`, `app/moderation/*.go`
- `TelegramListener` gets explicit `TenantID` field (reusing `InstanceID` for now)
- Pipeline helpers pass `tenantID` to storage calls
- `IncomingEvent.TenantID` becomes the primary field
- Tests: pipeline tests with tenant scoping

### Slice 6: Thread tenantID through webapi
**Files**: `app/webapi/*.go`
- Resolver helpers accept `tenantID` parameter
- Handlers extract tenant from `Settings.InstanceID` (single-tenant mode)
- Authz checks include tenant scope
- Tests: webapi tests pass tenant explicitly

### Slice 7: Single-tenant migration bridge
**Files**: `app/runtime_assembly.go`, `app/main.go`
- On startup: if no `tenants` row exists, create one from `InstanceID`
- Backfill: set `tenant_id = gid` for all tables
- Ensure zero-downtime: old code with `gid` still works, new code uses `tenant_id`
- Tests: migration from fresh DB + migration from existing DB

### Slice 8: Isolation integration tests
**Files**: `app/storage/isolation_test.go`
- Create two tenants in shared Postgres
- For every storage module: write data as tenant A, verify tenant B cannot see it
- Negative test: query without tenant scope returns error or empty
- Tests: 10+ subtests covering all entities

### Slice 9: Tenant-aware cache keys
**Files**: `app/controlplane/cache.go`
- Cache keys prefixed with `tenantID`
- `InvalidateAll(ctx)` clears only current tenant's entries
- Tests: two tenants with overlapping keys, verify isolation

### Slice 10: Per-tenant quotas (stubs)
**Files**: `app/storage/tenant_limits.go`, `app/controlplane/quota_service.go`
- `tenant_limits` table: `tenant_id, limit_type, limit_value, current_usage, window_start`
- `QuotaService` interface with `Check(ctx, tenantID, limitType)`, `Increment(ctx, tenantID, limitType)`
- Stub implementation: always allows, logs usage
- Tests: interface contract tests

### Slice 11: Federation hooks (stubs)
**Files**: `app/controlplane/federation.go`
- `FederationService` interface: `SharedBans(ctx, tenantID)`, `InheritedPolicies(ctx, tenantID)`
- No-op implementation
- Feature flag: `--enable-federation` (default: off)
- Tests: interface contract tests

### Slice 12: Soft-delete & offboarding
**Files**: `app/storage/tenants.go`, `app/controlplane/tenant_service.go`
- `tenant_service.go`: `Suspend(ctx, tenantID)`, `Resume(ctx, tenantID)`, `SoftDelete(ctx, tenantID)`
- Soft-delete: set status = 'deleted', clear caches, revoke access
- `TenantResolver` middleware: reject requests for non-active tenants
- Tests: lifecycle tests

### Slice 13: API-level rate limits & authz
**Files**: `app/webapi/middleware.go`, `app/webapi/ratelimit.go`
- Per-tenant rate limiter (token bucket)
- Authz middleware: resolve tenant, verify membership, check status
- Apply to all control plane routes
- Tests: rate limit enforcement, authz rejection

### Slice 14: SQL audit & repository tests
**Files**: `app/storage/audit_test.go`
- Test every repository method requires `tenant_id` filter
- Test that raw SQL without `tenant_id` clause fails in test mode
- Regression safety net

## Verification

After each slice:
1. `make build` — compiles
2. `make test` — all existing tests pass
3. `go vet ./...` — clean

After final slice:
1. All 3 completion criteria verified
2. Full test suite green
3. No file exceeds 500 LOC

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Large table migration | Backfill in batches, column defaults to `gid` |
| Test explosion | Multi-tenant test helper, parameterized |
| LOC overflow | Extract tenant query helpers if needed |
| Breaking single-tenant | `tenant_id` defaults to `gid`, fallback queries |
