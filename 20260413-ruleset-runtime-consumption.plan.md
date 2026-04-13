# 2026-04-13 RuleSet Runtime Consumption Plan

1. Move detector construction behind `RuleSet.Active()` during runtime assembly.
2. Add a detector builder that accepts a `RuleSet` and uses it for duplicate, meta, abnormal-spacing, and LLM moderation settings.
3. Wire listener moderation/report config from the loaded active `RuleSet`.
4. Keep secrets, prompts, and provider clients in runtime options for now.
5. Add focused tests proving active `RuleSet` values override bootstrap defaults.
6. Update roadmap and architecture docs to record the runtime source-of-truth change.

## Validation Skills

- `mcaf-solid-maintainability`: centralize moderation config in one runtime source of truth
- `mcaf-testing`: prove active rule-set override behavior at startup
