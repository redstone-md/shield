# Stage 7 Brainstorm: Audit, Review, Appeals

## Current State

### What exists
- `detected_spam` table: text, user_id, checks JSON, signal_source, score, matched_rules, rule_set_version, idempotency_key. Already an implicit incident record.
- `incoming_events` table: full ingress event with decision_action, decision_reason, decision_score, action_applied, processed_at. Already captures replay.
- `reports` table: user reports with reporter/reported, msg_text, notification state.
- `moderation_actions` table: executor command attempts, idempotent replay.
- `AuditRecord` struct: Event + Message + Response + PolicyDecision + ActionResult + SlowPath + RuleSetVersion. Written via `defaultAuditWriter` → `auditSpamLogger` → file log + detected_spam table.
- Web UI: `/detected_spam` page with filtering, add-to-samples. No incident detail, no review queue, no appeals.
- Admin chat: inline buttons (ban/unban/warn) for reported messages. Manual review via Telegram.
- Reports: threshold-based auto-ban, LLM auto-review, admin notification with callback buttons.
- Replay: `IncomingEvents.Reserve()` replays completed events for duplicate suppression. No dry-run replay endpoint.

### What's missing (roadmap items)
1. **Incident model**: unified entity linking event, signals, decision, action, comments.
2. **Full audit trail**: raw input → normalized → signals → policy → action → final status in one queryable chain.
3. **Incident timeline**: unified view of reports, admin actions, auto-mod in chronological order.
4. **Read model**: incident list, incident detail, review queue in UI.
5. **Manual review flow**: queue of user reports + ambiguous slow-path cases for human review.
6. **Replay endpoint**: re-run saved payload through current pipeline without side effects.
7. **Reason taxonomy**: structured reason codes, not just free text.
8. **Appeal workflow**: new → triaged → accepted → rejected → replayed → escalated.
9. **Redaction + retention**: PII handling, TTL on sensitive data.
10. **Integration tests**: full case walkthrough without prod logs.

## Design Options

### Option A: Thin Incident Wrapper (Recommended)

**Idea**: Don't build a separate incident table. Derive incidents from existing data. Add a thin `incidents` table that links existing records + adds appeal/review state.

```
incidents
├── id (PK)
├── gid, tenant_id
├── source (auto_mod, user_report, admin_action, appeal)
├── status (open, reviewing, resolved, appealed, closed)
├── severity (low, medium, high, critical)
├── idempotency_key → incoming_events.idempotency_key
├── detected_spam_id → detected_spam.id (nullable)
├── report_id → reports.id (nullable)
├── reason_code (structured taxonomy)
├── reason_text
├── created_at, updated_at, resolved_at
├── resolved_by
└── comment (free text from reviewer)
```

**incident_comments** (for timeline):
```
├── id, incident_id
├── author_type (system, admin, user)
├── author_id
├── action (created, escalated, reviewed, appealed, resolved, commented)
├── payload (JSON, arbitrary metadata)
├── created_at
```

**appeals**:
```
├── id, incident_id, gid, tenant_id
├── appellant_user_id, appellant_user_name
├── status (new, triaged, accepted, rejected, replayed, escalated)
├── appeal_text
├── resolution_text
├── resolved_by
├── replay_result (JSON, stored DetectionResult)
├── created_at, updated_at, resolved_at
```

**Why thin**: 
- Existing `detected_spam` + `incoming_events` + `reports` already hold 90% of incident data.
- New tables only add what's missing: review state, timeline comments, appeals.
- No data duplication. Incident detail = JOIN across existing tables.
- Migration: backfill `incidents` from existing `detected_spam` rows (auto_mod source).

### Option B: Fat Incident Table

Single `incidents` table with all fields denormalized. Duplicate data from detected_spam + reports + decisions.

**Pros**: Single query for everything. Simmer read path.
**Cons**: Data duplication, sync issues, migration pain. Reject.

### Option C: Event Sourcing

Append-only event log. Reconstruct state by replaying events.

**Pros**: Full audit by design. Perfect replay.
**Cons**: Complex queries, overkill for current scale. Reject.

## Reason Taxonomy

Structured codes for detection/policy/decision:

```go
type ReasonCode string

const (
    ReasonRegexMatch       ReasonCode = "regex_match"
    ReasonStopWord         ReasonCode = "stop_word"
    ReasonSimilarity       ReasonCode = "similarity"
    ReasonCAS              ReasonCode = "cas"
    ReasonMetaLink         ReasonCode = "meta_link"
    ReasonMetaMention      ReasonCode = "meta_mention"
    ReasonMultiLang        ReasonCode = "multi_lang"
    ReasonAbnormalSpacing  ReasonCode = "abnormal_spacing"
    ReasonEmojiSpam        ReasonCode = "emoji_spam"
    ReasonLLMOpenAI        ReasonCode = "llm_openai"
    ReasonLLMGemini        ReasonCode = "llm_gemini"
    ReasonVision           ReasonCode = "vision"
    ReasonUserReport       ReasonCode = "user_report"
    ReasonAdminAction      ReasonCode = "admin_action"
    ReasonEscalation       ReasonCode = "escalation"
    ReasonPolicyRule       ReasonCode = "policy_rule"
)
```

Policy decision writes `ReasonCode` into `PolicyDecision.Reason` (structured, not just text). Detection signals already have `Name` field — map to reason codes.

## Appeal Workflow

States: `new → triaged → accepted|rejected|replayed|escalated`

- **new**: User submits appeal (via bot command or web UI).
- **triaged**: Admin picks from review queue, sees full incident.
- **accepted**: False positive confirmed. Unban user, add to ham samples, update policy.
- **rejected**: Spam confirmed. No action.
- **replayed**: Admin re-runs detection on original payload. Stores new DetectionResult for comparison.
- **escalated**: Needs senior admin or developer review.

Appeal triggers:
1. User sends `/appeal` to bot in DM.
2. Admin creates appeal from incident detail page.
3. Auto-created when slow-path returns ambiguous result.

## Replay Endpoint

`POST /replay` or `POST /incidents/{id}/replay`:
1. Load original `IncomingEvent` from storage.
2. Re-run through fast path (spam detector).
3. If slow-path enabled + escalation conditions met, run slow path.
4. Run through policy engine.
5. Return `DetectionResult + PolicyDecision` without executing actions.
6. Store result in `incident_comments` as replay action.

## New Web UI Pages

1. `/incidents` — list with filters (status, source, severity, date range, tenant).
2. `/incidents/{id}` — detail: original message, signals, decision, action result, timeline, comments, replay button.
3. `/review` — queue of incidents needing manual review (status=open or reviewing).
4. `/appeals/{id}` — appeal detail with resolution form.

## Redaction + Retention

- `detected_spam.text` and `reports.msg_text` contain PII.
- Redaction: mask user_name, user_id in API responses for non-admin roles.
- Retention: configurable TTL. Background job purges records older than N days.
- Out of scope for initial implementation — add config hooks, implement later.

## Package Structure

```
app/
├── audit/                    # NEW: incident + appeal logic
│   ├── types.go             # Incident, Appeal, IncidentComment, ReasonCode
│   ├── store.go             # IncidentStore interface
│   ├── service.go           # AuditService: create incident, add comment, list, filter
│   ├── appeal.go            # AppealService: submit, triage, resolve, replay
│   └── *_test.go
├── storage/
│   ├── incidents.go         # NEW: SQL storage for incidents + comments
│   ├── appeals.go           # NEW: SQL storage for appeals
│   └── ...
├── webapi/
│   ├── handlers_incidents.go # NEW: incident list, detail, comment, review queue
│   ├── handlers_appeals.go   # NEW: appeal list, detail, submit, resolve, replay
│   ├── assets/
│   │   ├── incidents.html    # NEW
│   │   ├── incident_detail.html # NEW
│   │   ├── review.html       # NEW
│   │   └── appeal_detail.html # NEW
│   └── ...
└── events/
    ├── audit_writer.go       # MODIFY: also create incident on spam detection
    └── ...
```

## Slicing Order

1. **Types + store interfaces** (`app/audit/types.go`, `app/audit/store.go`)
2. **SQL storage: incidents + comments** (`app/storage/incidents.go`)
3. **Audit service** (`app/audit/service.go`) — create incident, add comment, list, filter
4. **Reason taxonomy** — map existing detection names to ReasonCode, update pipeline
5. **Wire audit into pipeline** — `defaultAuditWriter` creates incident on spam detection
6. **Appeal types + storage** (`app/storage/appeals.go`)
7. **Appeal service** (`app/audit/appeal.go`) — submit, triage, resolve
8. **Replay endpoint** — load event, re-run pipeline dry, store result
9. **Web UI: incident list + detail** — `/incidents`, `/incidents/{id}`
10. **Web UI: review queue + appeal detail** — `/review`, `/appeals/{id}`
11. **Integration tests** — full case: detect → incident → appeal → resolve → replay
12. **Redaction + retention hooks** — config structs, future implementation

## Dependencies on Previous Stages

- Stage 3 (multi-tenant): `tenant_id` on all new tables. Tenant isolation in queries.
- Stage 5 (policy): `PolicyDecision` already structured. `ReasonCode` extends it.
- Stage 6 (slow path): `SlowPathInvocation` in audit. Ambiguous slow-path → auto-incident.

## Risk / Open Questions

1. **Backfill**: Should existing `detected_spam` rows get incidents? Yes, one-time migration. But not blocking for MVP.
2. **Appeal UX**: Telegram-only (bot command) or web-only? Start with web UI, add bot command later.
3. **Replay fidelity**: Original payload in `incoming_events` is normalized. Can't replay raw Telegram update. Accept limitation, document it.
4. **Admin chat integration**: Current inline buttons (ban/unban) in admin chat — should they create incidents? Yes, as `admin_action` source.
5. **Concurrent review**: Two admins reviewing same incident? Optimistic locking via `updated_at` check.
