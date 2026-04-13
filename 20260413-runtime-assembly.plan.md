# 2026-04-13 Runtime Assembly Plan

1. Extract explicit runtime assembly structs and helper builders from `app/main.go`.
2. Change `execute` to orchestrate those higher-level assemblies instead of wiring concrete chains inline.
3. Keep web runtime activation based on interface-shaped dependencies rather than concrete storage types.
4. Add or reuse focused tests that prove startup behavior remains unchanged.
5. Update roadmap and architecture docs to reflect the assembly boundary.
6. Run targeted Go tests and `git diff --check`.

## Validation Skills

- `mcaf-solid-maintainability`: confirm startup wiring is more cohesive and explicit
- `mcaf-testing`: confirm startup behavior is preserved
