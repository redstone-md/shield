# 2026-04-13 Runtime Probes Plan

1. Add a small runtime probe type and `/healthz` + `/readyz` handlers in `app/main`.
2. Add runtime configuration for the probe listen address.
3. Start the probe server during runtime assembly and flip readiness when startup completes.
4. Extend tests to cover handler behavior and server-only runtime exposure.
5. Update roadmap and architecture docs to mark the probe requirement complete.
6. Run targeted Go tests and `git diff --check`.

## Validation Skills

- `mcaf-observability`: confirm probes represent runtime liveness and readiness clearly
- `mcaf-testing`: confirm probe behavior with deterministic tests
