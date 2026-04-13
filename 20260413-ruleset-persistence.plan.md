# 2026-04-13 RuleSet Persistence Plan

1. Add a domain `RuleSet` type under `app/rules`.
2. Add `rule_sets` and `rule_set_versions` storage with bootstrap and read methods.
3. Convert current runtime options into a bootstrap `RuleSet` in `app/main`.
4. Persist the bootstrap `RuleSet` during runtime assembly without changing moderation behavior yet.
5. Add focused storage and startup tests.
6. Update roadmap and architecture docs to record the new persistence seam.

## Validation Skills

- `mcaf-solid-maintainability`: keep the new rule domain and storage seam explicit
- `mcaf-testing`: prove bootstrap persistence and idempotent startup behavior
