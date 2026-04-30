# Stage 7 Plan: Audit, Review, Appeals

## Approach
Option A from brainstorm: thin incident wrapper. Derive incidents from existing data. New tables only add review state, timeline, appeals.

## Slices

### Slice 1: Types + Reason Taxonomy
- `app/audit/types.go`: Incident, IncidentComment, Appeal, ReasonCode constants, IncidentStatus, AppealStatus, IncidentSource, IncidentSeverity
- `app/audit/store.go`: IncidentStore, AppealStore interfaces
- No external deps

### Slice 2: SQL Storage — Incidents + Comments
- `app/storage/incidents.go`: incidents + incident_comments tables, CRUD, list with filters (status, source, severity, date range), tenant-isolated
- Follows existing storage patterns (engine.SQL, QueryMap, InitTable, migrate)
- Tests in `app/storage/incidents_test.go`

### Slice 3: SQL Storage — Appeals
- `app/storage/appeals.go`: appeals table, CRUD, status transitions, tenant-isolated
- Tests in `app/storage/appeals_test.go`

### Slice 4: Audit Service
- `app/audit/service.go`: AuditService — CreateIncident, AddComment, GetIncident, ListIncidents, UpdateStatus
- Creates incident from AuditRecord when spam detected (auto_mod source)
- Creates incident from user report when threshold reached (user_report source)
- Links to detected_spam + incoming_events via idempotency_key
- Tests with mock store

### Slice 5: Appeal Service
- `app/audit/appeal.go`: AppealService — Submit, Triage, Accept, Reject, Escalate, ListByStatus
- Accept: unban user (via Bot interface), add to ham samples, close incident
- Reject: close incident
- Escalate: change incident status to reviewing
- Tests with mock store

### Slice 6: Wire Audit into Pipeline
- Modify `defaultAuditWriter.Write()`: also call AuditService.CreateIncident on spam detection
- Modify `userReports.checkReportThreshold()`: create incident on report threshold
- Wire AuditService into TelegramListener, runtime_assembly.go
- Incident creation is fire-and-forget (error logged, not blocking pipeline)

### Slice 7: Reason Taxonomy Mapping
- Map existing spamcheck.Response.Name → ReasonCode
- Update PolicyDecision.Reason to use structured "code:text" format
- Update `auditSpamLogger.SaveAudit()` to extract and store ReasonCode
- Add ReasonCode to Incident on creation

### Slice 8: Replay Endpoint
- `app/webapi/handlers_replay.go`: POST /incidents/{id}/replay
- Load IncomingEvent by idempotency_key from incident
- Re-run spam detector (fast path) on stored text
- If slow-path conditions met, run slow path (dry)
- Run policy engine
- Return merged result without side effects
- Store replay result as incident_comment (action=replay)

### Slice 9: Web UI — Incident List + Detail
- `app/webapi/handlers_incidents.go`: GET /incidents, GET /incidents/{id}, POST /incidents/{id}/comment, POST /incidents/{id}/status
- `app/webapi/assets/incidents.html`: filterable list
- `app/webapi/assets/incident_detail.html`: signals, decision, timeline, comments, replay button
- Register routes in webapi.go

### Slice 10: Web UI — Review Queue + Appeals
- `app/webapi/handlers_incidents.go`: GET /review (status=open/reviewing filter)
- `app/webapi/handlers_appeals.go`: GET /appeals, GET /appeals/{id}, POST /appeals, POST /appeals/{id}/resolve
- `app/webapi/assets/review.html`: review queue
- `app/webapi/assets/appeal_detail.html`: resolution form
- Register routes in webapi.go

### Slice 11: Integration Tests
- Full walkthrough: message → detect → incident created → appeal submitted → triaged → accepted → user unbanned
- Replay test: incident → replay → compare results
- Review queue test: multiple incidents → filter → resolve

### Slice 12: Redaction + Retention Hooks
- Config structs in audit/types.go: RetentionConfig (TTL days, purge interval)
- MaskHelper for PII redaction in API responses (non-admin roles)
- Background purge goroutine skeleton (not yet wired, just the function)
- Document limitation: no auto-purge in MVP

## Verification
- `make build` after each slice
- `make test` after each slice
- File LOC < 500, function LOC < 80
- All new tables tenant-isolated (tenant_id in WHERE)

## Risks
- Replay fidelity: only normalized payload available, not raw Telegram update. Accept + document.
- Backfill: existing detected_spam rows don't get incidents in MVP. Future migration.
- Concurrent review: optimistic locking via updated_at. Accept race window for MVP.
