# Brainstorm: Phase 2 — Control Plane MVP

## Problem
Settings in webapi are read-only (`webapi.Settings` populated once at startup from env/flags). Admins cannot change rules without restarting. Dictionary/approved-user management goes directly to storage/detector from web handlers, bypassing any domain service layer.

## Key Insight
The project already has `RuleSets` in storage with `EnsureBootstrap` + `Active`. Phase 1 made the runtime load `ActiveRuleSet` at startup. The missing piece: a **service** layer that can update `RuleSet`, persist new versions, and signal the runtime to hot-reload.

## Approach Options

### A. Thin service layer on existing webapi
- Add `RuleSetService` to webapi.Config
- Add CRUD endpoints next to existing `/settings`
- Pro: minimal structural change
- Con: webapi becomes even larger (already 1422 lines)

### B. Separate `app/controlplane` package with services, consumed by webapi
- New package: interfaces + implementations for rule management, cache, roles
- webapi gains a `ControlPlane` dependency that it delegates to
- Pro: clean separation, testable, aligns with roadmap bounded-context goal
- Con: more files, need to thread the new dependency through main.go

### C. Full TMA from the start
- Build Telegram Mini App before stabilizing backend
- Con: premature UI investment before API is stable

## Decision: B
`app/controlplane` owns the domain logic (update rules, invalidate cache, check roles). `app/webapi` stays a thin HTTP adapter that calls controlplane services. This matches the roadmap bounded-context boundary and keeps webapi from bloating further.

## Slice Order (first meaningful increment)
1. `app/controlplane` package with `RuleSetService` interface + implementation
2. PUT /rules endpoint in webapi that updates RuleSet via service
3. In-process `CacheStore` that the listener/detector reads from, invalidated on rule update
4. Verify: admin changes rule via API → worker picks up new config without restart

## GID / Multi-tenant Note
Phase 2 stays single-tenant. All controlplane services take `gid` from the SQL engine. Role tables are added but only one workspace is populated at this stage.
