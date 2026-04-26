# Plan: Control Plane MVP (Phase 2)

## Approach: incremental slices, each slice delivers a working end-to-end increment

### Slice 1 — RuleSet CRUD API (read + update)
- Add `RuleSetService` interface in `app/controlplane/ruleset.go`
- Implement: `GetRuleSet(ctx, workspaceID) (RuleSet, error)`, `UpdateRuleSet(ctx, workspaceID, RuleSet) (version int, error)`
- UpdateRuleSet persists a new `rule_set_versions` row + bumps `rule_sets.active_version`
- Wire `RuleSetService` into `webapi.Config` as `RuleSetProvider`
- Add API routes: `GET /api/rules`, `PUT /api/rules` (JSON in/out, behind existing auth)
- Settings page `/list_settings` becomes writable: HTMX form posts to `PUT /api/rules`

### Slice 2 — In-process cache + invalidation
- Add `CacheStore` interface in `app/controlplane/cache.go`: `GetRuleSet(ctx, workspaceID) (RuleSet, error)`, `InvalidateRuleSet(ctx, workspaceID) error`
- Implement `InProcessCache`: wraps `RuleSetService`, caches active RuleSet in `sync.RWLocker + atomic.Value`
- On `UpdateRuleSet`: persist → invalidate cache → next `GetRuleSet` reloads from DB
- Wire cache into `runtimeAssembly.ActiveRuleSet` path so listener/detector read from cache

### Slice 3 — Chat/workspace/admin/role DB tables
- Add `workspaces` table: `id TEXT PK, name TEXT, owner_id TEXT, created_at, updated_at`
- Add `workspace_members` table: `workspace_id, user_id, role (owner/admin/viewer), created_at`
- Add `WorkspaceService` in `app/controlplane/workspace.go`
- Bootstrap single workspace on startup from `InstanceID`

### Slice 4 — Role-based auth middleware
- Add `RoleAuthorizer` in `app/controlplane/auth.go`
- Middleware extracts user from Basic Auth → looks up role in `workspace_members`
- `owner/admin`: full CRUD, `viewer`: read-only
- Apply to `/api/rules` and future control plane endpoints

### Slice 5 — Move approved users + dictionary under control plane
- Add `ApprovedUsersService` in `app/controlplane/approved_users.go`: wraps storage, invalidates cache
- Add `DictionaryService` in `app/controlplane/dictionary.go`: wraps storage, invalidates cache
- Webapi handlers call services instead of storage/detector directly

### Slice 6 — Read model for incidents + actions
- Add `IncidentReadModel` in `app/controlplane/incidents.go`
- Queries `detected_spam` + `moderation_actions` + `incoming_events` for recent activity
- API: `GET /api/incidents?limit=N&offset=N`
- Web UI page: `/incidents`

### Slice 7 — Telegram Mini App auth contract
- Define `TelegramAuthService` interface in `app/controlplane/telegram_auth.go`
- Validate `initData` / `login_url` signature
- Map Telegram user → workspace member
- Wire as alternative auth middleware for `/api/*` routes

## Execution order
1. Slice 1 (RuleSet CRUD) — the most impactful single change
2. Slice 2 (cache + invalidation) — makes Slice 1 useful at runtime
3. Slice 3 (workspace DB) — foundation for roles
4. Slice 4 (role auth) — security boundary
5. Slice 5 (approved users + dict under CP) — cleanup
6. Slice 6 (read model) — operational visibility
7. Slice 7 (TMA auth) — future auth path

## Verification
- Each slice: unit tests + integration test through webapi
- Slice 1+2 combined: acceptance test — update RuleSet via API, verify new events use updated rules without restart

## Current iteration — finish Slice 1+2 tracer bullet

1. [x] Replace the ad-hoc single cached rule set in `RuleSetService` with the existing in-process cache contract.
2. [x] Add focused tests for cache hit, invalidation, and update notification.
3. [x] Add webapi handler tests for `GET /api/rules` and `PUT /api/rules`.
4. [x] Add runtime acceptance coverage proving a `RuleSetService.Update` applies to listener/detector without restart.
5. [x] Update roadmap progress for the completed part of Stage 2.
6. [x] Run focused tests and `make test`.

Focused verification passed:

- `go test ./app/controlplane ./app/webapi ./app`
- `make test`

## Current iteration — Slice 3 workspace bootstrap

1. [x] Add `WorkspaceService` in `app/controlplane` with typed roles and single-workspace bootstrap.
2. [x] Wire `WorkspaceService.EnsureDefaultWorkspace` into runtime assembly startup.
3. [x] Add tests for service validation, bootstrap idempotency, membership bootstrap, and runtime assembly wiring.
4. [x] Update roadmap progress for completed workspace foundation.
5. [x] Run focused tests and `make test`.

Focused verification passed:

- `go test ./app/controlplane ./app`
- `make test`

## Current iteration — Slice 4 role-based control plane auth

1. [x] Add `RoleAuthorizer` in `app/controlplane` with read/write decisions over workspace membership.
2. [x] Wire role authorization into `/rules` webapi routes while keeping existing Basic Auth as authentication.
3. [x] Add controlplane and webapi tests for owner/admin/viewer permissions and denial paths.
4. [x] Update roadmap progress for completed role auth foundation.
5. [x] Run focused tests and `make test`.

Focused verification passed:

- `go test ./app/controlplane ./app/webapi ./app`
- `make test`
