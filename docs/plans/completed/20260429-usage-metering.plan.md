# S9.1 Usage Metering — Plan

## Slices

### Slice 1: Usage Metering Storage
- `app/storage/usage_metering.go` — `UsageMetering` struct embedding `*engine.SQL`
- Table: `usage_meters` (id, gid, tenant_id, meter_type, count, window_start, updated_at)
- `UNIQUE(tenant_id, meter_type)`
- Methods: `Increment(ctx, meterType)`, `Get(ctx, meterType)`, `Reset(ctx, meterType)`, `List(ctx)`, `Stats(ctx) → map[string]int64`
- DBCmd iota base: `+1000`
- Tests: `usage_metering_test.go`

### Slice 2: Metering Service
- `app/controlplane/metering_service.go` — `MeteringService` wrapping storage
- Methods: `Increment(ctx, meterType)`, `GetUsage(ctx) → UsageReport`, `ResetWindow(ctx)`
- `UsageReport`: map of meter_type → count + window_start
- Wire `QuotaService` to use real `TenantLimits` + `UsageMetering` for quota checks
- Tests

### Slice 3: Pipeline Integration
- Wire metering into `app/events/pipeline.go` at `processQueuedEvent`
- Increment: `messages_received` (every message), `spam_detected` (ban/restrict), `ham_passed` (allow), `messages_deleted` (delete action)
- Non-blocking: log warning on error, never block pipeline
- Tests

### Slice 4: API Endpoint + DI Wiring
- `GET /api/usage` → current usage report
- `POST /api/usage/reset` → reset all meters
- Wire `MeteringService` into `runtime_assembly.go` + `webapi.Config`
- Tests

## Meter Types
- `messages_received` — every incoming message processed
- `spam_detected` — spam found + action taken
- `ham_passed` — clean message, no action
- `messages_deleted` — spam message deleted
- `bans_applied` — user banned/restricted
- `spam_checks` — spam check invocations (includes clean messages)
- `reports_filed` — user spam reports
- `appeals_filed` — appeal submissions

## Constraints
- Metering is best-effort — errors logged, never block pipeline
- Atomic increment via `UPDATE ... SET count = count + 1`
- Window-based: `window_start` tracks when counters were last reset
