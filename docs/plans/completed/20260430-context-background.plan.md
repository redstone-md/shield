# Plan: Replace context.Background() in Business Logic

## Goal
Propagate real `ctx` from callers instead of `context.Background()` in business logic.

## Steps

### Step 1: Trivial — ctx already in function params (just use it)
- [x] `admin_callbacks.go:114` — `callbackBanConfirmed(ctx,...)` → use `ctx`
- [x] `admin_callbacks.go:288` — `deleteAndBan(ctx,...)` → use `ctx`
- [x] `admin.go:163` — `MsgHandler(ctx,...)` → use `ctx`
- [x] `dictionary_service.go:70` — `ReadWithIDs(_ ctx,...)` → stop discarding `ctx`, use it

### Step 2: Easy — add ctx param, pass from direct caller
- [x] `admin.go:220` — `msgHandlerFallback` → add `ctx` param, pass from `MsgHandler`
- [x] `reports.go:185` — `tryLLMReportModeration` → add `ctx` param, pass from `DirectUserReport`
- [x] `dictionary_service.go:88` — `reloadAndNotify` → add `ctx` param, pass from `Add`/`Delete`
- [x] `pipeline.go:47` — change caller in `listener.go:284` to `procEventsWithContext(ctx, ...)`

### Step 3: Medium — parameter threading through multiple functions
- [x] `admin.go:266` — `deleteUserMessages` → add `ctx` param; goroutine caller uses `context.WithoutCancel(ctx)`
- [x] `admin_commands.go:247` — `directReport` + `DirectXxxReport` → add `ctx` param, thread from `procSuperReply`
- [x] `ruleset_service.go:86` — `Invalidate` → add `ctx` param; callers pass short timeout ctx

### Step 4: Verify
- [x] `make test`
- [x] `make build`

### Skipped (justified uses)
- `assembly.go:282` — startup code, no request ctx
- `sampe_updater.go` — interface limitation (requires cross-module change)
- `spam.go:118` — legacy interface, already has `OnMessageWithContext`
- `pipeline.go:172` — background queue worker, no request ctx (could accept lifecycle ctx, deferred)
