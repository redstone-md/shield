# Appeal Flow — Design

- **Date:** 2026-05-17
- **Status:** Approved (pending spec review)
- **Branch:** `feat/appeal-flow`, off `dev`.

## Problem

When the bot warns or bans a user, the user has no in-Telegram way to contest the
decision. The appeals/incidents subsystem exists (`app/audit` — `Appeal`, `Incident`,
`AppealService`; the web `/appeals` admin UI), but nothing lets an end user *file*
an appeal — appeals are an admin-only concept today. Separately, a ban currently
posts **no message** to the group chat (only the admin chat is notified), so a
banned user never even learns why or how to contest it.

## Goals

- A punished user can tap **"Обжаловать"** on the warn/ban message, land in the bot
  DM, and file an appeal in one tap.
- The appeal reaches the admin chat with **Accept / Reject** inline buttons.
- Accepting unbans the user (if banned) and clears their warnings; the user is
  notified of the outcome either way.
- A ban now posts a message to the group chat, auto-deleted the same way a warning is.

## Non-goals

- No free-text appeal reason — filing is one tap (decided in brainstorming).
- No re-appeal — one appeal per incident; an admin decision is final.
- No appeal flow for channel (`SenderChat`) bans — there is no user to unban.
- No new config/CLI/env option; the ban message reuses `WarnDeleteDuration`.
- No web-UI changes beyond what falls out of the shared `AppealService` behavior.

## Decisions (from brainstorming)

| Topic | Decision |
|---|---|
| Overall approach | Reuse the existing appeals/incidents subsystem (`AppealService`, `Incident`, `Appeal`) |
| Bot DM step | One tap — bot files the appeal immediately, no reason text, no DM state |
| Re-appeal | One appeal per incident; a decision (accept/reject) is final |
| Notify user of result | Yes — bot DMs the user the outcome |
| Appeal button type | Inline **URL** button → bot deep link (`t.me/<bot>?start=<incidentID>`) |
| Ban message | Now posted to the group chat; auto-deleted via `WarnDeleteDuration` |
| Channel bans | Unchanged — no ban message, no appeal button |
| Manual `/warn` `/ban` | Also get the appeal button; the plan ensures they create an incident |

## Existing infrastructure (reused)

| Component | Location | Role |
|---|---|---|
| `Incident` / `Appeal` models | `app/audit/types.go` | incident + appeal records, status enums |
| `AppealService` | `app/audit/appeal.go` | `Submit`, `Accept`, `Reject`, `Triage`, `Escalate` |
| `AppealStorage` | `app/storage/appeals.go` | appeals CRUD; `Create`, `Get`, `GetByIncident`, `UpdateStatus` |
| `IncidentStorage` | `app/storage/incidents.go` | incidents CRUD; `Get`, `Create`, `UpdateStatus` |
| `WarnUser` | `app/events/action_executor.go` | posts the warn message, schedules its delete |
| `banUserOrChannel` | `app/events/action_executor.go` | applies the Telegram ban; posts nothing today |
| `InlineCallbackHandler` | `app/events/admin_callbacks.go` | admin-chat inline-button callback dispatch |
| `handleCallback` / update loop | `app/events/listener.go` | update dispatch; today: groups + callbacks only |
| `deleteAllWarns` | `app/events/admin_commands.go` | clears a user's warn strikes (`detected_spam` rows) |
| `ReportBan` / `ReportWarn` | `app/events/admin.go` | admin-chat notifications |

The hot path already exists. The new work is: a user-facing entry point, a
group-chat ban message, and the admin-chat decision UI.

## End-to-end flow

1. The user spams; the pipeline decides warn or ban.
2. **The incident is created before the warn/ban message is posted**, so its ID is
   known when the message is built.
3. The bot posts a warn **or** ban message to the group chat. The message carries an
   inline URL button **"Обжаловать"** → `https://t.me/<botUsername>?start=<incidentID>`.
   Both messages auto-delete after `WarnDeleteDuration` (the ban message is new).
4. The user taps "Обжаловать" → Telegram opens the bot DM and sends `/start <incidentID>`.
5. The bot handles `/start <incidentID>` in a private chat: validates, creates the
   appeal, replies to the user.
6. The bot posts the appeal to the admin chat with **Accept / Reject** inline buttons.
7. An admin taps a button; the appeal resolves; the user is notified.
8. One appeal per incident — a second tap after a decision tells the user it is
   already resolved.

## Components

### C1 — Group-chat warn/ban message + appeal button

- The appeal button is `tbapi.NewInlineKeyboardButtonURL("Обжаловать",
  "https://t.me/<botUsername>?start=<incidentID>")`. A URL button needs no callback
  handling — Telegram opens the link.
- The bot username is already available to the listener (used as `BotUsername` in
  the settings template); it is threaded into the action executor.
- **Warn message:** `WarnUser` already posts it; it gains the inline keyboard. Text
  unchanged.
- **Ban message (new):** after a successful **user** ban, the ban path posts a short
  message to the group chat (e.g. `🚫 Пользователь забанен за спам`) carrying the
  same appeal button, and schedules its deletion exactly as `WarnUser` does
  (`WarnDeleteDuration`, reusing the existing send + scheduled-delete helper).
- **Channel bans** (`SenderChat`): no ban message, no button — unchanged.
- **Ordering:** the incident is created before the message is posted, so the
  incident ID is embedded in the button. The pipeline creates the incident, then
  passes the incident ID and bot username into the warn/ban action.

### C2 — `/start` deep-link handler

- The listener's update loop gains a branch: a **private-chat** message whose text is
  `/start <payload>` routes to a new appeal handler (a new unit in `app/events/`,
  e.g. `appeal_handler.go`). A bare `/start` with no payload is ignored / given a
  minimal reply — out of scope for this feature.
- The handler, given `<incidentID>`:
  1. Parse the payload to an incident ID. Invalid → reply "Неверная ссылка".
  2. Load the incident (`IncidentStorage.Get`). Not found → "Инцидент не найден".
  3. Verify the sender's user ID equals `incident.SpamUserID`. Mismatch → "Эта
     ссылка не для вас".
  4. If the incident is already closed/resolved → "Наказание уже неактивно".
  5. If an appeal already exists for the incident (`AppealStorage.GetByIncident`) →
     "Апелляция уже подана" (or "уже рассмотрена" if resolved).
  6. `AppealService.Submit(incidentID, userID, userName, "")` — empty appeal text.
     This sets the incident status to `appealed`.
  7. Reply to the user: "✅ Апелляция подана, ожидайте решения модераторов".
  8. Post the appeal to the admin chat (C3).
- **Double-tap race:** guarded by the step-5 existence check; the plan adds a unique
  index on `appeals.incident_id` if the storage lacks one, so a tight race fails the
  second insert cleanly rather than creating a duplicate.

### C3 — Admin-chat appeal notification + decision

- After the appeal is created, the bot posts a new message to the admin chat:
  `📩 Апелляция — <user link> по инциденту #N` plus context (reason, message
  snippet), styled like `ReportBan`. It carries inline buttons **"✅ Принять"** and
  **"❌ Отклонить"**.
- Callback data uses new prefixes carrying the appeal ID: `AA<appealID>` (accept),
  `AR<appealID>` (reject). `InlineCallbackHandler` dispatches them to new handlers
  `callbackAppealAccept` / `callbackAppealReject`.
- A second tap on an already-resolved appeal: the handler sees the resolved status
  and answers the callback with "Апелляция уже рассмотрена" — no double action.

### C4 — Resolution behavior (shared by Telegram and web)

`AppealService.Accept` / `Reject` already use a `BotService` adapter interface for
`UnbanUser`. The interface is extended with `ClearUserWarnings(userID)` and
`NotifyAppealResult(userID, accepted)`. Resolution behavior then lives in
`AppealService` and is identical whether the appeal is resolved from the Telegram
admin-chat buttons **or** the web `/appeals` UI:

- **Accept:** unban the user (idempotent — no-op if not banned), add the message as a
  ham sample (existing behavior), **clear all of the user's warnings**, close the
  incident, **DM the user** "Апелляция принята — вы разбанены, предупреждения сняты".
- **Reject:** close the incident, no unban, **DM the user** "Апелляция отклонена".
- The Telegram callback handler additionally edits the admin-chat message to show the
  outcome and the resolving admin, and removes the buttons.

`ClearUserWarnings` is implemented in the events/runtime layer (it wraps
`deleteAllWarns`); `audit` depends only on the adapter interface, never on `events`.
The user-facing DM is best-effort — if the user has blocked the bot, the failure is
logged and does not abort the unban.

## Data flow

```
spam → pipeline → create Incident ──┐
                                    ├─→ warn/ban action posts group message
                                    │     (inline URL button → t.me/bot?start=<incidentID>)
user taps button → /start <id> ─────┘
  → appeal handler validates → AppealService.Submit → Appeal(status=new), Incident(status=appealed)
  → admin-chat message [Принять | Отклонить]
admin taps → callbackAppealAccept/Reject
  → AppealService.Accept/Reject → adapter: UnbanUser + ClearUserWarnings + NotifyAppealResult
  → Appeal(status=accepted/rejected), Incident(status=closed)
  → admin message edited; user DM sent
```

## Error handling

- Invalid/garbage payload, unknown incident, wrong user, closed incident, duplicate
  appeal — each replies with a specific message to the user; nothing is created.
- Double admin tap — guarded by appeal status; the second tap is a no-op with a
  callback notice.
- Result DM failure (user blocked the bot) — logged, non-fatal; the unban / warning
  clearing still completes.
- Manual unban before the appeal is decided — `Accept`'s unban is idempotent.
- Incidents subsystem disabled — no incident is created, so no appeal button is
  attached; the flow degrades gracefully (the rest of warn/ban is unaffected).

## Testing

- `/start` payload parsing and the appeal handler: valid, garbage payload, wrong
  user, missing incident, duplicate appeal, already-closed incident.
- Action executor: the warn and ban messages carry the correct deep-link button; the
  ban message is posted for a user ban and **not** for a channel ban; the ban
  message's deletion is scheduled.
- Admin callbacks: `callbackAppealAccept` unbans + clears warnings + notifies;
  `callbackAppealReject` notifies; the double-tap guard holds.
- `AppealService` with the extended adapter: `Accept`/`Reject` invoke
  `ClearUserWarnings` and `NotifyAppealResult`; mocks regenerated via `moq` /
  `go:generate`.
- Existing warn/ban tests stay green; the new ban message must not break assertions
  that count posted messages.
- Project gates: `go test -race ./...`, `golangci-lint run`, `unfuck-ai-comments`.
- README updated: a ban now posts a (self-deleting) group message, and the appeal
  flow is documented.

## Risks

- **Appeal window.** The appeal button lives only as long as the warn/ban message,
  which auto-deletes after `WarnDeleteDuration`. A user who does not tap in time
  loses the entry point. Accepted — the user chose "delete like a warning".
- **Incident-first ordering.** Moving incident creation before the warn/ban message
  post is a localized pipeline reordering. If it proves invasive, the fallback is to
  post the message first and edit it to attach the button once the incident exists
  (`editMessageReplyMarkup`); the plan picks the concrete path.
- **`appeals.incident_id` uniqueness.** If the table has no unique constraint, a
  race could double-insert; the plan adds the index.
- **Private-chat handling is new.** The listener has not processed private chats
  before; the new branch must not disturb group-message processing.

## Out of scope (possible follow-ups)

- Free-text appeal reasons.
- A config toggle to suppress the group-chat ban message.
- Appeal support for channel bans.
