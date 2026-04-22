# 20260422 Audit Enrichment Brainstorm

## Goal

Persist richer moderation audit context for each detected spam decision:

- signal source
- score
- matched rules
- rule set version

## Constraints

- Keep current `detected_spam` UI/API working.
- Avoid breaking the existing `SpamLogger` seam for unrelated call sites.

## Approach

1. Extend `detected_spam` schema with enriched audit columns and migrations.
2. Add an optional enriched audit logger interface used by `defaultAuditWriter`.
3. Implement the enriched sink in `makeSpamLogger`.
4. Pass active `rule_set_version` from runtime assembly into `AuditRecord`.
5. Verify storage, audit writer, and runtime logger behavior.
