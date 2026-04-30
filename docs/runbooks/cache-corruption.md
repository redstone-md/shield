# Runbook: Cache Corruption

## Symptoms
- Stale rule sets applied (bans not matching current config)
- Tenant status checks return wrong state (active tenant blocked)
- Inconsistent behavior across restarts (in-memory cache lost)
- Error logs: "cache invalidate failed" or "rule set version mismatch"

## Diagnosis
1. Check cache state via API: `GET /api/metrics` — look for counter anomalies
2. Compare rule set version in response headers vs DB:
   ```sql
   SELECT version FROM rule_sets WHERE workspace_id = ? ORDER BY version DESC LIMIT 1;
   ```
3. Check tenant status in DB:
   ```sql
   SELECT id, status FROM tenants WHERE id = ?;
   ```
4. Verify in-memory cache entries match DB state

## Resolution
1. **Hot reload** — restart the service to rebuild cache from DB:
   ```bash
   docker-compose restart tg-spam
   ```
2. **Targeted invalidation** — call offboard/onboard cycle:
   ```bash
   curl -X POST http://localhost:8080/api/tenants/{id}/offboard
   curl -X POST http://localhost:8080/api/tenants/onboard -d '{"tenant_id":"...","name":"...","owner_id":"..."}'
   ```
3. **DB-level fix** — correct corrupted records:
   ```sql
   UPDATE rule_sets SET active = 0 WHERE workspace_id = ? AND version != (SELECT MAX(version) FROM rule_sets rs WHERE rs.workspace_id = rule_sets.workspace_id);
   ```

## Prevention
- Cache reads are eventually consistent — rule set updates invalidate cache immediately
- Onboarding service populates cache on tenant creation
- Retention service does not touch cache — status changes go through TenantService
