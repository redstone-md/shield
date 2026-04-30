# 20260422 Audit Idempotency Link Brainstorm

## Goal

Persist `idempotency_key` alongside enriched audit records so each moderation decision can be tied to both the active `RuleSet` version and the retry-safe ingress key.

## Scope

- `detected_spam` schema and migrations
- enriched audit path
- focused tests and roadmap criterion
