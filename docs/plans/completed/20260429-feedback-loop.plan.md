# Stage 8 Plan: Feedback Loop & Knowledge Base

## Approach
Pragmatic MVP. Label model on detected_spam. Candidate generation from incidents. Review-then-publish workflow. Version snapshots for rollback. Skip shadow/A/B rollout — premature for monolith MVP.

## Slices

### Slice 1: Label Types + Storage
- `app/feedback/types.go`: Label enum (confirmed_spam, false_positive, missed_spam, policy_override), LabelEntry struct (ID, DetectedSpamID, IncidentID, Label, LabeledBy, CreatedAt)
- `app/feedback/store.go`: LabelStore interface (Create, GetByDetectedSpamID, GetByLabel, List, Stats)
- `app/storage/labels.go`: SQL table + CRUD, tenant-isolated
- Tests in `app/storage/labels_test.go`

### Slice 2: Label Service
- `app/feedback/service.go`: FeedbackService — Label(ctx, entry), GetLabelsForSpam, ListByLabel, Stats
- Labeling a detected_spam entry as confirmed_spam → auto-add to spam samples
- Labeling as false_positive → auto-add to ham samples + create incident comment
- Tests with mock store

### Slice 3: Candidate Generator
- `app/feedback/candidates.go`: CandidateGenerator — FromIncident(ctx, incidentID), FromDetectedSpam(ctx, spamID)
- Extracts repeated tokens/phrases from incident message text
- Produces candidate entries: {Type: stop_phrase|regex, Value: string, Source: incident|detected_spam, SourceID, Score}
- Score = frequency heuristic (same phrase seen in N incidents)
- `app/storage/candidates.go`: SQL table + CRUD, tenant-isolated
- Tests

### Slice 4: Candidate Review Workflow
- `app/feedback/review.go`: ReviewService — ApproveCandidate, RejectCandidate, ListPending, ListApproved, ListRejected
- Approve: promote candidate to dictionary (stop_phrase → Dictionary.Add) or samples (regex → Samples.Add)
- Reject: mark rejected with reviewer info
- `app/feedback/types.go`: CandidateStatus (pending, approved, rejected), CandidateEntry
- Tests

### Slice 5: Knowledge Snapshot + Rollback
- `app/feedback/knowledge.go`: KnowledgeService — Snapshot(ctx, tenantID), ListSnapshots, Rollback(ctx, snapshotID)
- Snapshot: serialize current dictionary + samples counts + version into JSON blob, store in knowledge_snapshots table
- Rollback: load snapshot, clear current data, re-import from snapshot
- `app/storage/knowledge_snapshots.go`: SQL table + CRUD
- Tests

### Slice 6: Wire into Runtime + Web UI
- Create FeedbackService, ReviewService, KnowledgeService in runtime_assembly.go
- Add to webapi.Config
- `app/webapi/handlers_feedback.go`: API endpoints — POST /labels, GET /candidates, POST /candidates/{id}/approve, POST /candidates/{id}/reject, POST /knowledge/snapshot, POST /knowledge/rollback/{id}
- `app/webapi/assets/feedback.html`: labels + candidates management page
- Register routes

### Slice 7: Integration Tests
- Full flow: detect spam → label confirmed → candidate generated → review → approve → promoted to dictionary
- False positive flow: label FP → ham sample added → incident comment created
- Snapshot → approve new candidate → snapshot again → rollback → verify state

### Slice 8: Auto-Label from Appeals
- When appeal accepted → auto-label detected_spam as false_positive
- When appeal rejected → auto-label detected_spam as confirmed_spam
- Wire in AppealService.Accept/Reject → call FeedbackService.Label
- Skip: shadow/A/B rollout, tenant-local vs global knowledge split, offline evaluation pipeline

## Verification
- `make build` after each slice
- `make test` after each slice
- File LOC < 500, function LOC < 80
- All new tables tenant-isolated

## Risks
- Candidate quality: simple frequency heuristic may produce noise. Accept for MVP, human review catches bad candidates.
- Snapshot size: large sample sets → large JSON blobs. Accept for MVP, tenant count is small.
- Rollback atomicity: clear + re-import not transactional across tables. Document limitation.
