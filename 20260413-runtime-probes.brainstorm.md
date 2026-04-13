# 2026-04-13 Runtime Probes Brainstorm

## Problem

Phase 0 still lacks readiness and health endpoints for the main runtime. Today the process only exposes `/ping` if `app/webapi` is enabled, which does not cover Telegram-first deployments or distinguish alive vs ready.

## Constraints

- Keep the slice atomic and local to runtime assembly.
- Do not couple the probe surface to `app/webapi`.
- Preserve current startup behavior when no probe listen address is configured.

## Options

### Option 1: Reuse `app/webapi` for probes

- Pro: no new server type
- Con: does not solve the “main runtime, not only web API” requirement

### Option 2: Add a lightweight probe server in `app/main`

- Pro: independent of the admin UI, easy to keep minimal, fits runtime lifecycle
- Con: one more small HTTP listener to configure

## Decision

Add a tiny runtime probe server in `app/main` with `/healthz` and `/readyz`. Start it when configured, keep readiness false during startup, flip it to ready only after runtime assembly succeeds, and drop back on shutdown.

## Verification Target

- probe handler returns `200` for health while running and `503` for readiness until explicitly ready
- server-only runtime can expose `/healthz` and `/readyz` alongside the existing web UI `/ping`
