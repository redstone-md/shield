# Runbook: Tenant Restore

## Symptoms
- Tenant data missing after accidental deletion
- Need to recover a single tenant from backup
- Migration between environments (staging → production)

## Prerequisites
- Valid backup file (from `GET /api/backup` or automated backup)
- Target tenant must not already exist in active state
- Database connectivity verified

## Procedure
1. **Verify backup contents**:
   ```bash
   head -50 backup.sql
   grep "INSERT INTO" backup.sql | head -5
   ```
   Confirm `tenant_id` values match target tenant.

2. **Create placeholder tenant** (if tenant was fully deleted):
   ```bash
   curl -X POST http://localhost:8080/api/tenants/onboard \
     -H "Content-Type: application/json" \
     -d '{"tenant_id":"TARGET_ID","name":"Restored Tenant","owner_id":"admin"}'
   ```

3. **Upload backup**:
   ```bash
   curl -X POST http://localhost:8080/api/tenants/TARGET_ID/restore \
     -F "file=@backup.sql"
   ```
   The restore service filters INSERT statements by `tenant_id`, skips DDL, and uses `INSERT OR REPLACE` for idempotency.

4. **Verify restore**:
   ```bash
   curl http://localhost:8080/api/tenants/TARGET_ID/status
   ```
   Check key tables:
   ```sql
   SELECT COUNT(*) FROM incidents WHERE tenant_id = 'TARGET_ID';
   SELECT COUNT(*) FROM detected_spam WHERE tenant_id = 'TARGET_ID';
   SELECT COUNT(*) FROM incoming_events WHERE tenant_id = 'TARGET_ID';
   ```

5. **Invalidate cache**:
   ```bash
   curl -X POST http://localhost:8080/api/tenants/TARGET_ID/offboard
   curl -X POST http://localhost:8080/api/tenants/onboard \
     -d '{"tenant_id":"TARGET_ID","name":"Restored","owner_id":"admin"}'
   ```

## Rollback
If restore fails or corrupts data:
1. Offboard the tenant: `POST /api/tenants/TARGET_ID/offboard`
2. Drop tenant data: `DELETE FROM incidents WHERE tenant_id = 'TARGET_ID'` (repeat for all tables)
3. Retry from step 2 with a different backup file

## Notes
- Restore is tenant-scoped — other tenants unaffected
- `INSERT OR REPLACE` means newer data wins if row ID matches
- Backup files contain only the tenant's own data (filtered by `tenant_id` during export)
- Retention policy may have cleaned old records — verify backup age vs retention TTL
