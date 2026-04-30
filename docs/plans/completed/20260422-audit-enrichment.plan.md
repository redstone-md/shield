# 20260422 Audit Enrichment Plan

1. Extend `storage.DetectedSpamInfo` and `detected_spam` migrations with audit enrichment fields.
2. Add an optional enriched audit logger interface and route `defaultAuditWriter` through it when available.
3. Implement enriched `SaveAudit` in the runtime spam logger.
4. Carry `rule_set_version` from runtime assembly into `AuditRecord`.
5. Add storage, audit writer, and runtime tests.
6. Update architecture docs and roadmap status.
7. Run focused regressions and `git diff --check`.
