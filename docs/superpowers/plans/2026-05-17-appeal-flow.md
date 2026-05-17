# Appeal Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a warned or banned user tap an "Обжаловать" button, file a one-tap appeal that reaches the admin chat with Accept/Reject buttons, and on acceptance get unbanned with warnings cleared — and make a ban post a self-deleting group message like a warning does.

**Architecture:** The moderation pipeline creates the `audit.Incident` *before* posting the warn/ban group message, so the incident ID can be embedded in a `t.me/<bot>?start=<incidentID>` deep-link button. A new private-chat `/start` handler files the appeal through the existing `audit.AppealService`, notifies the admin chat, and admin Accept/Reject callbacks resolve it. Resolution behavior (unban + clear warnings + DM the user) lives in `AppealService` behind the extended `audit.BotService` adapter, so the Telegram callbacks and the web `/appeals` UI share one code path.

**Tech Stack:** Go 1.24, `github.com/OvyFlash/telegram-bot-api`, SQLite/Postgres via `sqlx`, `testify`, `moq`-generated mocks (`app/events/mocks`), hand-written mocks in `app/audit`.

**Scope note — deferred:** This plan covers the *automatic* moderation pipeline (auto-detected warn/ban → appeal). The design's decision table also mentions manual `/warn` and `/ban` commands getting the appeal button; those go through `app/events/admin*.go` handlers that do not currently create incidents, and wiring incident creation into all four manual handlers is a separate, sizeable change. It is **deferred to a follow-up** and called out again in the execution handoff. Everything else in the design (C1–C4) is implemented here.

**Scope note — `appeals.incident_id` uniqueness:** The design suggested a unique index as a double-tap guard. It is **intentionally omitted**: `TelegramListener.handleUpdate` processes Telegram updates sequentially on a single goroutine, so two `/start` taps are handled one after another and the "appeal already exists" check (Task 7, step 5) fully prevents duplicates. Adding `CREATE UNIQUE INDEX` would also risk failing migration on any pre-existing duplicate rows. No storage migration is needed.

---

## File Structure

**Modified:**
- `app/audit/service.go` — `CreateIncident` returns the incident ID and is idempotency-aware.
- `app/audit/appeal.go` — extended `BotService` interface; `Accept`/`Reject` call the new methods; new `GetAppeal`, `GetIncident`, `SetBotService`.
- `app/audit/service_test.go` — extend `mockBotService`; new `CreateIncident` tests.
- `app/events/audit_writer.go` — `IncidentCreator.CreateIncident` returns `(int64, error)`.
- `app/events/incident_adapter.go` — adapter returns the incident ID.
- `app/events/action_executor.go` — appeal button on `WarnUser`; new `scheduleDelete`, `banMessageRequest`, `PostBanMessage`.
- `app/events/pipeline.go` — `pipelineContext.incidentID`; `ensureIncident`; warn button wiring; ban group message.
- `app/events/listener.go` — `IncidentCreator`/`AppealService`/`appealHandler` fields; `procAppealStart`; `handleUpdate` hook; `initHandlers` wiring.
- `app/events/admin.go` — `admin.appeals` field.
- `app/events/admin_callbacks.go` — `AA`/`AR` callback dispatch; `callbackAppealResolve`.
- `app/events/listener_part1_test.go` — `actionExecutorSpy` gains `PostBanMessage`.
- `app/runtime_assembly.go` — wire `IncidentCreator`, `AppealService`, `SetBotService`.
- `README.md` — document the ban group message and the appeal flow.

**Created:**
- `app/events/appeal_handler.go` — `appealHandler`: `/start <incidentID>` deep-link processing + admin notification.
- `app/events/appeal_handler_test.go` — handler tests.
- `app/events/appeal_resolution.go` — `appealBotAdapter` implementing `audit.BotService`.
- `app/events/appeal_resolution_test.go` — adapter tests.

---

## Task 1: Incident creation returns an ID (idempotency-aware)

**Files:**
- Modify: `app/audit/service.go:56-109` (`CreateIncident`)
- Modify: `app/events/audit_writer.go:35-47` (`IncidentCreator` interface), `:87` (call site)
- Modify: `app/events/incident_adapter.go:15-47` (`incidentAdapter.CreateIncident`)
- Test: `app/audit/service_test.go`

- [ ] **Step 1: Write the failing test**

Add to `app/audit/service_test.go`:

```go
func TestAuditService_CreateIncident_ReturnsID(t *testing.T) {
	incStore := newMockIncidentStore()
	svc := NewService(incStore)

	id, err := svc.CreateIncident(context.Background(), AuditEventData{
		IdempotencyKey: "key-1",
		ChatID:         100,
		SpamUserID:     200,
		MessageText:    "buy now",
		CheckResults:   []SpamCheckResult{{Name: "regex", Spam: true, Details: "matched"}},
	})
	require.NoError(t, err)
	assert.Positive(t, id)

	id2, err := svc.CreateIncident(context.Background(), AuditEventData{
		IdempotencyKey: "key-1",
		ChatID:         100,
		SpamUserID:     200,
	})
	require.NoError(t, err)
	assert.Equal(t, id, id2, "same idempotency key returns the same incident id")
	assert.Len(t, incStore.incidents, 1, "no duplicate incident inserted")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/audit/ -run TestAuditService_CreateIncident_ReturnsID -v`
Expected: compile failure — `CreateIncident` currently returns only `error`, so `id, err :=` does not type-check.

- [ ] **Step 3: Change `audit.Service.CreateIncident` signature and add the idempotency check**

In `app/audit/service.go`, replace the whole `CreateIncident` function (lines 56-109) with:

```go
func (s *Service) CreateIncident(ctx context.Context, data AuditEventData) (int64, error) {
	if data.IdempotencyKey != "" {
		existing, err := s.store.GetByIdempotencyKey(ctx, "", data.IdempotencyKey)
		if err == nil && existing.ID > 0 {
			return existing.ID, nil
		}
	}

	reasonCode := ReasonUnknown
	reasonText := "spam detected"
	for _, cr := range data.CheckResults {
		if cr.Spam {
			reasonCode = MapCheckNameToReason(cr.Name)
			reasonText = cr.Details
			break
		}
	}

	severity := ClassifySeverity(reasonCode)

	incident := Incident{
		Source:         SourceAutoMod,
		Status:         IncidentStatusOpen,
		Severity:       severity,
		IdempotencyKey: data.IdempotencyKey,
		ReasonCode:     reasonCode,
		ReasonText:     truncateMsg(reasonText, 500),
		SpamUserID:     data.SpamUserID,
		SpamUserName:   data.SpamUserName,
		ChatID:         data.ChatID,
		MessageText:    truncateMsg(data.MessageText, 1000),
	}

	created, err := s.store.Create(ctx, incident)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "duplicate key") {
			return 0, nil
		}
		return 0, fmt.Errorf("create incident: %w", err)
	}

	_, _ = s.store.AddComment(ctx, IncidentComment{
		IncidentID: created.ID,
		AuthorType: "system",
		AuthorID:   "pipeline",
		Action:     "created",
		Payload:    fmt.Sprintf(`{"source":"auto_mod","rule_set_version":%d}`, data.RuleSetVersion),
	})

	if data.SlowProvider != "" {
		_, _ = s.store.AddComment(ctx, IncidentComment{
			IncidentID: created.ID,
			AuthorType: "system",
			AuthorID:   "slow_path",
			Action:     "slow_path_invoked",
			Payload:    fmt.Sprintf(`{"provider":%q,"prompt_version":%q}`, data.SlowProvider, data.SlowPromptVer),
		})
	}

	return created.ID, nil
}
```

- [ ] **Step 4: Update the `IncidentCreator` interface**

In `app/events/audit_writer.go`, change the interface (lines 35-47) so `CreateIncident` returns `(int64, error)`:

```go
type IncidentCreator interface {
	CreateIncident(
		ctx context.Context,
		idempotencyKey string,
		chatID int64,
		ruleSetVersion int,
		spamUserID int64,
		spamUserName string,
		messageText string,
		checks []spamcheck.Response,
		slowPath *slowpath.SlowPathInvocation,
	) (int64, error)
}
```

- [ ] **Step 5: Update the `defaultAuditWriter.Write` call site**

In `app/events/audit_writer.go`, the call at line 87 currently begins `if err := w.incidentCreator.CreateIncident(ctx,`. Change it to discard the returned ID:

```go
		if _, err := w.incidentCreator.CreateIncident(ctx,
			record.Event.IdempotencyKey, record.ChatID, record.RuleSetVersion,
			record.SpamUserID, userName, msgText,
			record.Response.CheckResults, record.SlowPath,
		); err != nil {
			log.Printf("[WARN] incident creation failed: %v", err)
		}
```

- [ ] **Step 6: Update `incidentAdapter.CreateIncident`**

In `app/events/incident_adapter.go`, change the method signature (line 15) and return statement (line 46):

```go
func (a *incidentAdapter) CreateIncident(
	ctx context.Context,
	idempotencyKey string,
	chatID int64,
	ruleSetVersion int,
	spamUserID int64,
	spamUserName string,
	messageText string,
	checks []spamcheck.Response,
	slow *slowpath.SlowPathInvocation,
) (int64, error) {
```

and the final line:

```go
	return a.svc.CreateIncident(ctx, data)
```

- [ ] **Step 7: Run tests and build**

Run: `go test ./app/audit/ -run TestAuditService -v`
Expected: PASS, including the new test.

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 8: Commit**

```bash
git add app/audit/service.go app/audit/service_test.go app/events/audit_writer.go app/events/incident_adapter.go
git commit -m "feat: incident creation returns id and is idempotency-aware"
```

---

## Task 2: Appeal button on the warn message

**Files:**
- Modify: `app/events/action_executor.go:115-147` (`WarnUser`), `:251-257` (`warnRequest`)
- Test: `app/events/action_executor_test.go`

- [ ] **Step 1: Write the failing test**

Add to `app/events/action_executor_test.go`:

```go
func TestTelegramActionExecutor_WarnUserAppealButton(t *testing.T) {
	var sent tbapi.MessageConfig
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			sent = c.(tbapi.MessageConfig)
			return tbapi.Message{MessageID: 1}, nil
		},
	}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, nil)

	err := exec.WarnUser(context.Background(), warnRequest{
		chatID:      100,
		subjectID:   200,
		messageID:   5,
		text:        "warned",
		incidentID:  42,
		botUsername: "shield_bot",
	})
	require.NoError(t, err)

	markup, ok := sent.ReplyMarkup.(tbapi.InlineKeyboardMarkup)
	require.True(t, ok, "warn message must carry an inline keyboard")
	require.Len(t, markup.InlineKeyboard, 1)
	require.Len(t, markup.InlineKeyboard[0], 1)
	btn := markup.InlineKeyboard[0][0]
	assert.Equal(t, "Обжаловать", btn.Text)
	require.NotNil(t, btn.URL)
	assert.Equal(t, "https://t.me/shield_bot?start=42", *btn.URL)
}

func TestTelegramActionExecutor_WarnUserNoButtonWithoutIncident(t *testing.T) {
	var sent tbapi.MessageConfig
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			sent = c.(tbapi.MessageConfig)
			return tbapi.Message{MessageID: 1}, nil
		},
	}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, nil)

	err := exec.WarnUser(context.Background(), warnRequest{chatID: 100, subjectID: 200, text: "warned"})
	require.NoError(t, err)
	assert.Nil(t, sent.ReplyMarkup, "no incident id -> no appeal button")
}
```

If `action_executor_test.go` does not already import `github.com/umputun/tg-spam/app/events/mocks`, `github.com/stretchr/testify/assert`, `github.com/stretchr/testify/require`, or `tbapi "github.com/OvyFlash/telegram-bot-api"`, add them.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/events/ -run TestTelegramActionExecutor_WarnUser -v`
Expected: compile failure — `warnRequest` has no `incidentID` / `botUsername` field.

- [ ] **Step 3: Add the new fields to `warnRequest`**

In `app/events/action_executor.go`, replace the `warnRequest` struct (lines 251-257) with:

```go
type warnRequest struct {
	chatID      int64
	subjectID   int64
	messageID   int
	text        string
	warnDelTime time.Duration // time to delete the warning message, 0 to keep
	incidentID  int64         // incident backing the appeal button, 0 to omit the button
	botUsername string        // bot username for the appeal deep link
}
```

- [ ] **Step 4: Add the `appealKeyboard` helper**

In `app/events/action_executor.go`, add this function (place it just above `type banRequest struct`):

```go
// appealKeyboard builds the single-button "Обжаловать" inline keyboard that
// deep-links the punished user to the bot DM with the incident id as payload.
// The second return value is false when there is no incident to appeal.
func appealKeyboard(botUsername string, incidentID int64) (tbapi.InlineKeyboardMarkup, bool) {
	if botUsername == "" || incidentID <= 0 {
		return tbapi.InlineKeyboardMarkup{}, false
	}
	url := fmt.Sprintf("https://t.me/%s?start=%d", botUsername, incidentID)
	return tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonURL("Обжаловать", url),
		),
	), true
}
```

- [ ] **Step 5: Attach the keyboard in `WarnUser`**

In `app/events/action_executor.go`, in `WarnUser`, the block that builds `msgConfig` (lines 121-123) currently reads:

```go
	msgConfig := tbapi.NewMessage(req.chatID, req.text)
	msgConfig.ParseMode = tbapi.ModeHTML
	msgConfig.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
```

Insert the keyboard attachment right after it:

```go
	msgConfig := tbapi.NewMessage(req.chatID, req.text)
	msgConfig.ParseMode = tbapi.ModeHTML
	msgConfig.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	if kb, ok := appealKeyboard(req.botUsername, req.incidentID); ok {
		msgConfig.ReplyMarkup = kb
	}
```

- [ ] **Step 6: Run tests**

Run: `go test ./app/events/ -run TestTelegramActionExecutor_WarnUser -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add app/events/action_executor.go app/events/action_executor_test.go
git commit -m "feat: add appeal button to warn messages"
```

---

## Task 3: Self-deleting ban group message helper

**Files:**
- Modify: `app/events/action_executor.go:15-21` (`ActionExecutor` interface), `:115-147` (`WarnUser` — reuse helper)
- Modify: `app/events/listener_part1_test.go:49-77` (`actionExecutorSpy` struct), and add a `PostBanMessage` method
- Test: `app/events/action_executor_test.go`

- [ ] **Step 1: Write the failing test**

Add to `app/events/action_executor_test.go`:

```go
func TestTelegramActionExecutor_PostBanMessage(t *testing.T) {
	var sent tbapi.MessageConfig
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			sent = c.(tbapi.MessageConfig)
			return tbapi.Message{MessageID: 7}, nil
		},
	}
	exec := newTelegramActionExecutor(mockAPI, false, false, nil, nil)

	err := exec.PostBanMessage(context.Background(), banMessageRequest{
		chatID:      100,
		text:        "🚫 Пользователь забанен за спам",
		incidentID:  42,
		botUsername: "shield_bot",
	})
	require.NoError(t, err)
	assert.Equal(t, "🚫 Пользователь забанен за спам", sent.Text)
	markup, ok := sent.ReplyMarkup.(tbapi.InlineKeyboardMarkup)
	require.True(t, ok, "ban message must carry the appeal keyboard")
	require.NotNil(t, markup.InlineKeyboard[0][0].URL)
	assert.Equal(t, "https://t.me/shield_bot?start=42", *markup.InlineKeyboard[0][0].URL)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/events/ -run TestTelegramActionExecutor_PostBanMessage -v`
Expected: compile failure — `banMessageRequest` and `PostBanMessage` do not exist.

- [ ] **Step 3: Add `scheduleDelete` and refactor `WarnUser` to use it**

In `app/events/action_executor.go`, add this method (place it just below `WarnUser`):

```go
// scheduleDelete deletes a posted message after delTime in a background
// goroutine. delTime <= 0 keeps the message.
func (e telegramActionExecutor) scheduleDelete(ctx context.Context, chatID int64, msgID int, delTime time.Duration) {
	if delTime <= 0 {
		return
	}
	go func() {
		observability.Logf(ctx, "[DEBUG] scheduled message %d deletion in %v", msgID, delTime)
		time.Sleep(delTime)
		observability.Logf(ctx, "[DEBUG] deleting scheduled message %d after %v", msgID, delTime)
		if err := e.DeleteMessage(context.Background(), chatID, msgID); err != nil {
			observability.Logf(ctx, "[WARN] failed to delete scheduled message %d: %v", msgID, err)
		} else {
			observability.Logf(ctx, "[DEBUG] scheduled message %d deleted successfully", msgID)
		}
	}()
}
```

Then replace the tail of `WarnUser` (lines 130-146, from `e.recordAction(...)` after the successful send through the closing of the `if req.warnDelTime > 0` block) with:

```go
	e.recordAction(ctx, commandWarnUser, req.chatID, req.subjectID, req.messageID, attempt, nil)
	e.scheduleDelete(ctx, req.chatID, msg.MessageID, req.warnDelTime)
	return nil
}
```

- [ ] **Step 4: Add `banMessageRequest` and `PostBanMessage`**

In `app/events/action_executor.go`, add the request type next to `warnRequest`:

```go
type banMessageRequest struct {
	chatID      int64
	text        string
	incidentID  int64
	botUsername string
	delTime     time.Duration // time to auto-delete the ban message, 0 to keep
}
```

and add the method (place it just below `scheduleDelete`):

```go
// PostBanMessage posts a short ban notice to the group chat carrying the
// appeal button and schedules its deletion the same way a warning is deleted.
func (e telegramActionExecutor) PostBanMessage(ctx context.Context, req banMessageRequest) error {
	msgConfig := tbapi.NewMessage(req.chatID, req.text)
	msgConfig.ParseMode = tbapi.ModeHTML
	msgConfig.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	if kb, ok := appealKeyboard(req.botUsername, req.incidentID); ok {
		msgConfig.ReplyMarkup = kb
	}
	msg, err := e.tbAPI.Send(msgConfig)
	if err != nil {
		return fmt.Errorf("post ban message to chat %d: %w", req.chatID, err)
	}
	e.scheduleDelete(ctx, req.chatID, msg.MessageID, req.delTime)
	return nil
}
```

- [ ] **Step 5: Add `PostBanMessage` to the `ActionExecutor` interface**

In `app/events/action_executor.go`, replace the `ActionExecutor` interface (lines 15-21) with:

```go
type ActionExecutor interface {
	ApplyBan(ctx context.Context, req banRequest) error
	DeleteMessage(ctx context.Context, chatID int64, msgID int) error
	ForwardMessage(ctx context.Context, fromChatID, toChatID int64, msgID int) error
	DeleteExtraMessages(ctx context.Context, checkResults []spamcheck.Response, userID int64, username string, chatID int64) error
	WarnUser(ctx context.Context, req warnRequest) error
	PostBanMessage(ctx context.Context, req banMessageRequest) error
}
```

- [ ] **Step 6: Add `PostBanMessage` to the `actionExecutorSpy` test fake**

In `app/events/listener_part1_test.go`, the `actionExecutorSpy` struct (lines 49-77) implements `ActionExecutor`. Add these fields inside the struct (next to `warnUser` / `warnCalls`):

```go
	postBanMessage func(ctx context.Context, req banMessageRequest) error
	postBanCtxs    []context.Context
	postBanCalls   []banMessageRequest
```

and add this method next to the spy's `WarnUser` method (after line 194):

```go
func (s *actionExecutorSpy) PostBanMessage(ctx context.Context, req banMessageRequest) error {
	s.postBanCtxs = append(s.postBanCtxs, ctx)
	s.postBanCalls = append(s.postBanCalls, req)
	if s.postBanMessage != nil {
		return s.postBanMessage(ctx, req)
	}
	return nil
}
```

- [ ] **Step 7: Run tests and build**

Run: `go test ./app/events/ -run 'TestTelegramActionExecutor' -v`
Expected: PASS, including `TestTelegramActionExecutor_PostBanMessage` and the existing `TestTelegramActionExecutor_WarnUser`.

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 8: Commit**

```bash
git add app/events/action_executor.go app/events/action_executor_test.go app/events/listener_part1_test.go
git commit -m "feat: add self-deleting ban group message executor"
```

---

## Task 4: Incident-first pipeline ordering and ban group message

**Files:**
- Modify: `app/events/listener.go:50-109` (`TelegramListener` struct — add `IncidentCreator` field)
- Modify: `app/events/pipeline.go:208-217` (`pipelineContext`), `:388-446` (`processWarn`), `:448-475` (`applyBanAction`)
- Modify: `app/runtime_assembly.go:487-489` (audit writer wiring)
- Test: `app/events/pipeline_part*_test.go` (new test in whichever pipeline test file exists; if none, create `app/events/appeal_pipeline_test.go`)

- [ ] **Step 1: Write the failing test**

Create `app/events/appeal_pipeline_test.go`:

```go
package events

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/bot"
	"github.com/umputun/tg-spam/app/moderation"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

type incidentCreatorStub struct {
	id    int64
	calls int
}

func (s *incidentCreatorStub) CreateIncident(_ context.Context, _ string, _ int64, _ int,
	_ int64, _ string, _ string, _ []spamcheck.Response, _ *slowPathStub) (int64, error) {
	s.calls++
	return s.id, nil
}

func TestProcessWarn_PassesIncidentIDToWarnRequest(t *testing.T) {
	spy := &actionExecutorSpy{}
	creator := &incidentCreatorStub{id: 77}
	l := &TelegramListener{
		ActionExecutor:  spy,
		IncidentCreator: creator,
		BotUsername:     "shield_bot",
		AuditWriter:     &auditWriterSpy{},
	}

	pc := pipelineContext{
		event:    moderation.IncomingEvent{IdempotencyKey: "k1"},
		msg:      &bot.Message{ID: 5, From: bot.User{ID: 200}},
		resp:     bot.Response{Send: true, BanInterval: time.Hour, CheckResults: []spamcheck.Response{{Spam: true}}},
		fromChat: 100,
	}

	err := l.processWarn(context.Background(), pc)
	require.NoError(t, err)
	require.NotEmpty(t, spy.warnCalls)
	assert.Equal(t, int64(77), spy.warnCalls[0].incidentID)
	assert.Equal(t, "shield_bot", spy.warnCalls[0].botUsername)
	assert.Equal(t, 1, creator.calls)
}
```

Note: the `incidentCreatorStub` signature must match the real `IncidentCreator` interface exactly. If the compiler reports the `*slowPathStub` parameter type is wrong, change that last parameter to the real type `*slowpath.SlowPathInvocation` and import `github.com/umputun/tg-spam/app/slowpath`; delete the `slowPathStub` reference. (`slowPathStub` is a placeholder name only — the real type is `*slowpath.SlowPathInvocation`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/events/ -run TestProcessWarn_PassesIncidentIDToWarnRequest -v`
Expected: compile failure — `TelegramListener` has no `IncidentCreator` field and `pipelineContext` has no `incidentID`.

- [ ] **Step 3: Add the `IncidentCreator` field to `TelegramListener`**

In `app/events/listener.go`, inside the `TelegramListener` struct, add the field next to `AuditWriter` (line 86):

```go
	AuditWriter             AuditWriter
	IncidentCreator         IncidentCreator
```

- [ ] **Step 4: Add `incidentID` to `pipelineContext` and the `ensureIncident` helper**

In `app/events/pipeline.go`, add a field to `pipelineContext` (after `strikeCount int`, line 216):

```go
type pipelineContext struct {
	event       moderation.IncomingEvent
	msg         *bot.Message
	resp        bot.Response
	fromChat    int64
	spamUserID  int64
	banUserStr  string
	outcome     PolicyOutcome
	strikeCount int
	incidentID  int64
}
```

Add this helper to `app/events/pipeline.go` (place it just above `processWarn`):

```go
// ensureIncident creates the audit incident for a warn/ban before the group
// message is posted, so the incident id can be embedded in the appeal button.
// It is idempotent: the later AuditWriter.Write call resolves to the same
// incident. Returns 0 when incidents are disabled or the response is not
// actionable, in which case no appeal button is attached.
func (l *TelegramListener) ensureIncident(ctx context.Context, pc pipelineContext) int64 {
	if l.IncidentCreator == nil || !pc.resp.Send || pc.resp.BanInterval <= 0 {
		return 0
	}
	userName := ""
	if pc.msg != nil && pc.msg.From.ID != 0 {
		userName = pc.msg.From.DisplayName
	}
	msgText := ""
	if pc.msg != nil {
		msgText = pc.msg.Text
	}
	id, err := l.IncidentCreator.CreateIncident(ctx, pc.event.IdempotencyKey, pc.fromChat,
		l.RuleSetVersion, pc.spamUserID, userName, msgText, pc.resp.CheckResults, nil)
	if err != nil {
		observability.Logf(ctx, "[WARN] early incident creation failed: %v", err)
		return 0
	}
	return id
}
```

- [ ] **Step 5: Wire the incident id and bot username into `processWarn`**

In `app/events/pipeline.go`, in `processWarn`, after the `if !pc.resp.Send` guard (line 393) add the incident creation, and add the two new fields to the `warnRequest` literal (lines 405-411):

```go
func (l *TelegramListener) processWarn(ctx context.Context, pc pipelineContext) error {
	errs := new(multierror.Error)

	if !pc.resp.Send {
		return l.cleanupAfterAction(ctx, pc, errs)
	}

	pc.incidentID = l.ensureIncident(ctx, pc)

	warnNum := pc.strikeCount + 1
	warnTotal := l.ModerationConfig.WarnStrikes
	if warnTotal <= 0 {
		warnTotal = 3
	}

	warnText := buildWarningText(warnNum, warnTotal, pc.msg.From, pc.spamUserID, l.WarnMsg, slowpathReason(pc.resp.CheckResults))

	actionResult := l.makeActionResult(pc.event, moderation.ActionWarn, false)

	warnReq := warnRequest{
		chatID:      pc.fromChat,
		subjectID:   pc.spamUserID,
		messageID:   pc.msg.ID,
		text:        warnText,
		warnDelTime: l.ModerationConfig.WarnDeleteDuration,
		incidentID:  pc.incidentID,
		botUsername: l.BotUsername,
	}
```

Leave the rest of `processWarn` unchanged.

- [ ] **Step 6: Post the ban group message in `applyBanAction`**

In `app/events/pipeline.go`, add the `banGroupMessageText` constant and a `postBanGroupMessage` helper (place them just above `applyBanAction`):

```go
const banGroupMessageText = "🚫 Пользователь забанен за спам"

// postBanGroupMessage posts the self-deleting ban notice with the appeal
// button to the group chat. It is skipped for channel bans, restrictions
// (mutes), and dry/training runs where no real ban happened.
func (l *TelegramListener) postBanGroupMessage(ctx context.Context, pc pipelineContext, incidentID int64) {
	if l.Dry || l.TrainingMode || pc.outcome.Restrict || pc.resp.ChannelID != 0 {
		return
	}
	if err := l.ActionExecutor.PostBanMessage(ctx, banMessageRequest{
		chatID:      pc.fromChat,
		text:        banGroupMessageText,
		incidentID:  incidentID,
		botUsername: l.BotUsername,
		delTime:     l.ModerationConfig.WarnDeleteDuration,
	}); err != nil {
		observability.Logf(ctx, "[WARN] failed to post ban group message: %v", err)
	}
}
```

Then replace the whole `applyBanAction` function (lines 448-475) with:

```go
func (l *TelegramListener) applyBanAction(ctx context.Context, pc pipelineContext,
	actionResult *moderation.ModerationActionResult, errs *multierror.Error) {
	incidentID := l.ensureIncident(ctx, pc)
	banReq := banRequest{
		duration:  pc.outcome.Duration,
		userID:    pc.resp.User.ID,
		channelID: pc.resp.ChannelID,
		userName:  pc.banUserStr,
		chatID:    pc.fromChat,
		dry:       l.Dry,
		training:  l.TrainingMode,
		restrict:  pc.outcome.Restrict,
	}
	banStart := time.Now()
	if err := l.ActionExecutor.ApplyBan(ctx, banReq); err != nil {
		l.observeLatency("ban_latency", time.Since(banStart))
		l.incMetric("ban_errors")
		actionResult.Applied = false
		actionResult.Error = err.Error()
		multierror.Append(errs, fmt.Errorf("failed to apply %s for %s: %w",
			pc.outcome.Decision.Action, pc.banUserStr, err))
		return
	}
	actionResult.Applied = true
	if l.adminChatID != 0 && pc.msg.From.ID != 0 {
		l.adminHandler.ReportBan(pc.banUserStr, pc.msg, pc.outcome.Duration, pc.outcome.Restrict,
			slowpathReason(pc.resp.CheckResults))
	}
	l.postBanGroupMessage(ctx, pc, incidentID)
}
```

- [ ] **Step 7: Wire `listener.IncidentCreator` in runtime assembly**

In `app/runtime_assembly.go`, replace the audit-writer block (lines 487-489):

```go
	if a.AuditService != nil {
		incidentCreator := events.NewIncidentAdapter(a.AuditService)
		listener.AuditWriter = events.NewDefaultAuditWriter(a.SpamLogger, a.Locator, incidentCreator)
		listener.IncidentCreator = incidentCreator
	}
```

- [ ] **Step 8: Run tests and build**

Run: `go test ./app/events/ -run TestProcessWarn_PassesIncidentIDToWarnRequest -v`
Expected: PASS.

Run: `go test -race ./app/events/...`
Expected: PASS. If a pre-existing ban test now fails because `applyBanAction` posts an extra ban group message, it is a real-executor ban test (non-dry, non-training, non-restrict, non-channel) whose mock `TbAPI` now receives one extra `Send` call, or whose `actionExecutorSpy` now has one `postBanCalls` entry. Update that test's expected call count to include the ban message — do not suppress the new behavior.

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 9: Commit**

```bash
git add app/events/listener.go app/events/pipeline.go app/events/appeal_pipeline_test.go app/runtime_assembly.go
git commit -m "feat: create incident before warn/ban and post ban group message"
```

---

## Task 5: AppealService — lookups, extended BotService, resolution behavior

**Files:**
- Modify: `app/audit/appeal.go:9-12` (`BotService`), `:25-31` (add `SetBotService`), `:93-130` (`Accept`), `:132-157` (`Reject`), and add `GetAppeal`/`GetIncident`
- Modify: `app/audit/service_test.go:181-194` (`mockBotService`)
- Test: `app/audit/service_test.go`

- [ ] **Step 1: Write the failing test**

Add to `app/audit/service_test.go`:

```go
func TestAppealService_AcceptInvokesBotService(t *testing.T) {
	appeals := newMockAppealStore()
	incidents := newMockIncidentStore()
	inc, err := incidents.Create(context.Background(), Incident{SpamUserID: 555, MessageText: "spam text"})
	require.NoError(t, err)
	ap, err := appeals.Create(context.Background(), Appeal{IncidentID: inc.ID, AppellantUserID: 555, Status: AppealNew})
	require.NoError(t, err)

	bot := &mockBotService{}
	svc := NewAppealService(appeals, incidents, bot)

	require.NoError(t, svc.Accept(context.Background(), ap.ID, "admin", ""))
	assert.Equal(t, []int64{555}, bot.unbannedIDs)
	assert.Equal(t, []int64{555}, bot.clearedWarnings)
	require.Len(t, bot.notified, 1)
	assert.Equal(t, int64(555), bot.notified[0].userID)
	assert.True(t, bot.notified[0].accepted)
}

func TestAppealService_RejectNotifiesUser(t *testing.T) {
	appeals := newMockAppealStore()
	incidents := newMockIncidentStore()
	inc, err := incidents.Create(context.Background(), Incident{SpamUserID: 666})
	require.NoError(t, err)
	ap, err := appeals.Create(context.Background(), Appeal{IncidentID: inc.ID, AppellantUserID: 666, Status: AppealNew})
	require.NoError(t, err)

	bot := &mockBotService{}
	svc := NewAppealService(appeals, incidents, bot)

	require.NoError(t, svc.Reject(context.Background(), ap.ID, "admin", ""))
	assert.Empty(t, bot.unbannedIDs, "reject must not unban")
	require.Len(t, bot.notified, 1)
	assert.Equal(t, int64(666), bot.notified[0].userID)
	assert.False(t, bot.notified[0].accepted)
}
```

Note: `newMockAppealStore` is the appeal store mock already used by the existing `AppealService` tests in this file. If its name differs, use whatever appeal-store mock the existing `TestAppealService_*` tests construct.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/audit/ -run 'TestAppealService_AcceptInvokesBotService|TestAppealService_RejectNotifiesUser' -v`
Expected: compile failure — `mockBotService` has no `clearedWarnings` / `notified` fields and does not implement the extended `BotService`.

- [ ] **Step 3: Extend the `BotService` interface**

In `app/audit/appeal.go`, replace the `BotService` interface (lines 9-12) with:

```go
type BotService interface {
	UnbanUser(ctx context.Context, userID int64) error
	AddHamSample(ctx context.Context, text string) error
	ClearUserWarnings(ctx context.Context, userID int64) error
	NotifyAppealResult(ctx context.Context, userID int64, accepted bool) error
}
```

- [ ] **Step 4: Add `SetBotService`, `GetAppeal`, `GetIncident`**

In `app/audit/appeal.go`, add these methods (place `SetBotService` next to `SetFeedbackLabeler`, and the getters next to `GetForIncident`):

```go
func (s *AppealService) SetBotService(bot BotService) {
	s.bot = bot
}

func (s *AppealService) GetAppeal(ctx context.Context, appealID int64) (Appeal, error) {
	return s.appeals.Get(ctx, appealID)
}

func (s *AppealService) GetIncident(ctx context.Context, incidentID int64) (Incident, error) {
	return s.incidents.Get(ctx, incidentID)
}
```

- [ ] **Step 5: Call the new bot methods from `Accept`**

In `app/audit/appeal.go`, in `Accept`, replace the bot block (lines 112-117):

```go
	if s.bot != nil {
		_ = s.bot.UnbanUser(ctx, inc.SpamUserID)
		if inc.MessageText != "" {
			_ = s.bot.AddHamSample(ctx, inc.MessageText)
		}
		_ = s.bot.ClearUserWarnings(ctx, inc.SpamUserID)
		_ = s.bot.NotifyAppealResult(ctx, inc.SpamUserID, true)
	}
```

- [ ] **Step 6: Notify the user from `Reject`**

In `app/audit/appeal.go`, in `Reject`, after the `incidents.UpdateStatus(...)` block (line 144) and before `s.autoLabelAppeal(...)`, insert:

```go
	if s.bot != nil {
		_ = s.bot.NotifyAppealResult(ctx, ap.AppellantUserID, false)
	}
```

- [ ] **Step 7: Extend `mockBotService`**

In `app/audit/service_test.go`, replace the `mockBotService` struct (lines 181-184) with:

```go
type mockBotService struct {
	unbannedIDs     []int64
	hamAdded        []string
	clearedWarnings []int64
	notified        []struct {
		userID   int64
		accepted bool
	}
}
```

and add the two methods after `AddHamSample` (after line 194):

```go
func (m *mockBotService) ClearUserWarnings(_ context.Context, userID int64) error {
	m.clearedWarnings = append(m.clearedWarnings, userID)
	return nil
}

func (m *mockBotService) NotifyAppealResult(_ context.Context, userID int64, accepted bool) error {
	m.notified = append(m.notified, struct {
		userID   int64
		accepted bool
	}{userID: userID, accepted: accepted})
	return nil
}
```

- [ ] **Step 8: Run tests**

Run: `go test ./app/audit/ -v`
Expected: PASS, including the two new tests and all pre-existing `TestAppealService_*` tests.

- [ ] **Step 9: Commit**

```bash
git add app/audit/appeal.go app/audit/service_test.go
git commit -m "feat: appeal resolution unbans, clears warnings, notifies user"
```

---

## Task 6: appealBotAdapter — events-layer BotService implementation

**Files:**
- Create: `app/events/appeal_resolution.go`
- Create: `app/events/appeal_resolution_test.go`

- [ ] **Step 1: Write the failing test**

Create `app/events/appeal_resolution_test.go`:

```go
package events

import (
	"context"
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/events/mocks"
)

func TestAppealBotAdapter_NotifyAppealResult(t *testing.T) {
	var sent []tbapi.MessageConfig
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			sent = append(sent, c.(tbapi.MessageConfig))
			return tbapi.Message{}, nil
		},
	}
	adapter := NewAppealBotAdapter(&TelegramListener{TbAPI: mockAPI})

	require.NoError(t, adapter.NotifyAppealResult(context.Background(), 900, true))
	require.NoError(t, adapter.NotifyAppealResult(context.Background(), 900, false))

	require.Len(t, sent, 2)
	assert.Equal(t, int64(900), sent[0].ChatID)
	assert.Contains(t, sent[0].Text, "принята")
	assert.Contains(t, sent[1].Text, "отклонена")
}

func TestAppealBotAdapter_ClearUserWarningsNoCounter(t *testing.T) {
	adapter := NewAppealBotAdapter(&TelegramListener{})
	assert.NoError(t, adapter.ClearUserWarnings(context.Background(), 1), "no detected-spam counter -> no-op")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/events/ -run TestAppealBotAdapter -v`
Expected: compile failure — `NewAppealBotAdapter` does not exist.

- [ ] **Step 3: Create the adapter**

Create `app/events/appeal_resolution.go`:

```go
package events

import (
	"context"
	"fmt"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/audit"
)

// appealBotAdapter adapts the telegram listener to the audit.BotService
// interface so AppealService.Accept/Reject can unban users, clear their
// warning strikes and DM them the appeal outcome.
type appealBotAdapter struct {
	listener *TelegramListener
}

// NewAppealBotAdapter returns an audit.BotService backed by the listener.
func NewAppealBotAdapter(listener *TelegramListener) audit.BotService {
	return &appealBotAdapter{listener: listener}
}

// UnbanUser lifts a ban or restriction for the user in the primary chat.
func (b *appealBotAdapter) UnbanUser(_ context.Context, userID int64) error {
	if b.listener.adminHandler == nil {
		return fmt.Errorf("admin handler not initialized")
	}
	return b.listener.adminHandler.unban(userID)
}

// AddHamSample feeds the appealed message back as a ham sample.
func (b *appealBotAdapter) AddHamSample(_ context.Context, text string) error {
	if b.listener.Bot == nil || text == "" {
		return nil
	}
	return b.listener.Bot.UpdateHam(text)
}

// ClearUserWarnings removes every warning strike recorded for the user.
func (b *appealBotAdapter) ClearUserWarnings(ctx context.Context, userID int64) error {
	if b.listener.adminHandler == nil || b.listener.DetectedSpamCounter == nil {
		return nil
	}
	_, _, err := b.listener.adminHandler.deleteAllWarns(ctx, userID, "")
	return err
}

// NotifyAppealResult DMs the user the appeal outcome. Best-effort: a blocked
// bot or closed DM returns an error that the caller logs and ignores.
func (b *appealBotAdapter) NotifyAppealResult(_ context.Context, userID int64, accepted bool) error {
	text := "❌ Апелляция отклонена."
	if accepted {
		text = "✅ Апелляция принята — вы разбанены, предупреждения сняты."
	}
	if _, err := b.listener.TbAPI.Send(tbapi.NewMessage(userID, text)); err != nil {
		return fmt.Errorf("notify user %d: %w", userID, err)
	}
	return nil
}

var _ audit.BotService = (*appealBotAdapter)(nil)
```

- [ ] **Step 4: Run tests and build**

Run: `go test ./app/events/ -run TestAppealBotAdapter -v`
Expected: PASS.

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add app/events/appeal_resolution.go app/events/appeal_resolution_test.go
git commit -m "feat: add appeal bot service adapter"
```

---

## Task 7: Appeal handler — /start deep-link processing

**Files:**
- Create: `app/events/appeal_handler.go`
- Create: `app/events/appeal_handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `app/events/appeal_handler_test.go`:

```go
package events

import (
	"context"
	"errors"
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/audit"
	"github.com/umputun/tg-spam/app/events/mocks"
)

type fakeAppealFiler struct {
	incident    audit.Incident
	incidentErr error
	existing    audit.Appeal
	existingErr error
	submitted   audit.Appeal
	submitErr   error
	submitCalls []int64
}

func (f *fakeAppealFiler) GetIncident(_ context.Context, _ int64) (audit.Incident, error) {
	return f.incident, f.incidentErr
}

func (f *fakeAppealFiler) GetForIncident(_ context.Context, _ int64) (audit.Appeal, error) {
	return f.existing, f.existingErr
}

func (f *fakeAppealFiler) Submit(_ context.Context, incidentID, _ int64, _, _ string) (audit.Appeal, error) {
	f.submitCalls = append(f.submitCalls, incidentID)
	return f.submitted, f.submitErr
}

func newAppealTestMessage(userID int64, text string) *tbapi.Message {
	return &tbapi.Message{
		Chat: tbapi.Chat{ID: userID, Type: "private"},
		From: &tbapi.User{ID: userID, FirstName: "Spammer"},
		Text: text,
	}
}

func TestAppealHandler_Handle(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		filer     *fakeAppealFiler
		wantReply string
		wantSubmit bool
		wantAdmin  bool
	}{
		{
			name:      "garbage payload",
			payload:   "not-a-number",
			filer:     &fakeAppealFiler{},
			wantReply: "Неверная ссылка.",
		},
		{
			name:      "incident not found",
			payload:   "10",
			filer:     &fakeAppealFiler{incidentErr: errors.New("missing")},
			wantReply: "Инцидент не найден.",
		},
		{
			name:      "wrong user",
			payload:   "10",
			filer:     &fakeAppealFiler{incident: audit.Incident{ID: 10, SpamUserID: 999, Status: audit.IncidentStatusOpen}},
			wantReply: "Эта ссылка не для вас.",
		},
		{
			name:    "closed incident",
			payload: "10",
			filer: &fakeAppealFiler{
				incident: audit.Incident{ID: 10, SpamUserID: 555, Status: audit.IncidentStatusClosed},
			},
			wantReply: "Наказание уже неактивно.",
		},
		{
			name:    "appeal already filed",
			payload: "10",
			filer: &fakeAppealFiler{
				incident: audit.Incident{ID: 10, SpamUserID: 555, Status: audit.IncidentStatusAppealed},
				existing: audit.Appeal{ID: 1, Status: audit.AppealNew},
			},
			wantReply: "Апелляция уже подана, ожидайте решения модераторов.",
		},
		{
			name:    "success",
			payload: "10",
			filer: &fakeAppealFiler{
				incident:    audit.Incident{ID: 10, SpamUserID: 555, Status: audit.IncidentStatusOpen, ReasonText: "regex"},
				existingErr: errors.New("no appeal yet"),
				submitted:   audit.Appeal{ID: 88},
			},
			wantReply:  "✅ Апелляция подана, ожидайте решения модераторов.",
			wantSubmit: true,
			wantAdmin:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent []tbapi.MessageConfig
			mockAPI := &mocks.TbAPIMock{
				SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
					sent = append(sent, c.(tbapi.MessageConfig))
					return tbapi.Message{}, nil
				},
			}
			h := newAppealHandler(mockAPI, tt.filer, 12345)

			err := h.Handle(context.Background(), newAppealTestMessage(555, "/start "+tt.payload), tt.payload)
			require.NoError(t, err)

			require.NotEmpty(t, sent)
			assert.Equal(t, tt.wantReply, sent[0].Text)
			assert.Equal(t, tt.wantSubmit, len(tt.filer.submitCalls) == 1)
			if tt.wantAdmin {
				require.Len(t, sent, 2)
				assert.Equal(t, int64(12345), sent[1].ChatID)
				markup, ok := sent[1].ReplyMarkup.(tbapi.InlineKeyboardMarkup)
				require.True(t, ok)
				assert.Equal(t, "AA88", markup.InlineKeyboard[0][0].Data != nil && *markup.InlineKeyboard[0][0].Data == "AA88" ?
					"AA88" : *markup.InlineKeyboard[0][0].Data)
				assert.Equal(t, "AR88", *markup.InlineKeyboard[0][1].Data)
			}
		})
	}
}
```

Note: the ternary-looking line is not valid Go — replace that whole assertion with the two straightforward lines:

```go
				require.NotNil(t, markup.InlineKeyboard[0][0].Data)
				assert.Equal(t, "AA88", *markup.InlineKeyboard[0][0].Data)
				require.NotNil(t, markup.InlineKeyboard[0][1].Data)
				assert.Equal(t, "AR88", *markup.InlineKeyboard[0][1].Data)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/events/ -run TestAppealHandler_Handle -v`
Expected: compile failure — `newAppealHandler` / `appealHandler` do not exist.

- [ ] **Step 3: Create the appeal handler**

Create `app/events/appeal_handler.go`:

```go
package events

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tbapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/umputun/tg-spam/app/audit"
)

// callback prefixes for appeal accept/reject inline buttons; dispatched in
// admin_callbacks.go by InlineCallbackHandler.
const (
	appealAcceptPrefix = "AA"
	appealRejectPrefix = "AR"
)

// appealFiler files appeals and looks up incident/appeal state for the
// /start deep-link flow.
type appealFiler interface {
	Submit(ctx context.Context, incidentID, appellantUserID int64, appellantName, appealText string) (audit.Appeal, error)
	GetForIncident(ctx context.Context, incidentID int64) (audit.Appeal, error)
	GetIncident(ctx context.Context, incidentID int64) (audit.Incident, error)
}

// appealHandler processes "/start <incidentID>" deep links sent to the bot DM,
// files the appeal and notifies the admin chat.
type appealHandler struct {
	tbAPI       TbAPI
	appeals     appealFiler
	adminChatID int64
}

func newAppealHandler(tbAPI TbAPI, appeals appealFiler, adminChatID int64) *appealHandler {
	return &appealHandler{tbAPI: tbAPI, appeals: appeals, adminChatID: adminChatID}
}

// Handle validates the deep-link payload, files an appeal for the incident and
// posts an admin-chat notification with accept/reject buttons. Validation
// failures reply to the user and return nil; only unexpected errors propagate.
func (h *appealHandler) Handle(ctx context.Context, msg *tbapi.Message, payload string) error {
	if msg == nil || msg.From == nil {
		return nil
	}

	incidentID, err := strconv.ParseInt(strings.TrimSpace(payload), 10, 64)
	if err != nil || incidentID <= 0 {
		return h.reply(msg.Chat.ID, "Неверная ссылка.")
	}

	inc, err := h.appeals.GetIncident(ctx, incidentID)
	if err != nil {
		return h.reply(msg.Chat.ID, "Инцидент не найден.")
	}

	if msg.From.ID != inc.SpamUserID {
		return h.reply(msg.Chat.ID, "Эта ссылка не для вас.")
	}

	if inc.Status == audit.IncidentStatusClosed || inc.Status == audit.IncidentStatusResolved {
		return h.reply(msg.Chat.ID, "Наказание уже неактивно.")
	}

	if existing, gErr := h.appeals.GetForIncident(ctx, incidentID); gErr == nil && existing.ID > 0 {
		if existing.Status == audit.AppealAccepted || existing.Status == audit.AppealRejected {
			return h.reply(msg.Chat.ID, "Апелляция уже рассмотрена.")
		}
		return h.reply(msg.Chat.ID, "Апелляция уже подана, ожидайте решения модераторов.")
	}

	appellantName := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	appeal, err := h.appeals.Submit(ctx, incidentID, msg.From.ID, appellantName, "")
	if err != nil {
		_ = h.reply(msg.Chat.ID, "Не удалось подать апелляцию, попробуйте позже.")
		return fmt.Errorf("submit appeal for incident %d: %w", incidentID, err)
	}

	if err := h.reply(msg.Chat.ID, "✅ Апелляция подана, ожидайте решения модераторов."); err != nil {
		return err
	}

	h.notifyAdmin(appeal, inc, msg.From)
	return nil
}

func (h *appealHandler) reply(chatID int64, text string) error {
	if _, err := h.tbAPI.Send(tbapi.NewMessage(chatID, text)); err != nil {
		return fmt.Errorf("reply to appeal chat %d: %w", chatID, err)
	}
	return nil
}

// notifyAdmin posts the appeal to the admin chat with accept/reject buttons.
// A failure here is logged, not propagated: the appeal is already filed.
func (h *appealHandler) notifyAdmin(appeal audit.Appeal, inc audit.Incident, from *tbapi.User) {
	if h.adminChatID == 0 {
		return
	}
	reason := inc.ReasonText
	if reason == "" {
		reason = string(inc.ReasonCode)
	}
	snippet := strings.ReplaceAll(htmlEscape(truncateString(inc.MessageText, 200, "…")), "\n", " ")
	text := fmt.Sprintf("📩 <b>Апелляция</b> по инциденту #%d\n%s\n\n%s\n\n%s",
		inc.ID, htmlEscape(appealAppellantLabel(from)), htmlEscape(reason), snippet)

	msgConfig := tbapi.NewMessage(h.adminChatID, text)
	msgConfig.ParseMode = tbapi.ModeHTML
	msgConfig.LinkPreviewOptions = tbapi.LinkPreviewOptions{IsDisabled: true}
	msgConfig.ReplyMarkup = tbapi.NewInlineKeyboardMarkup(
		tbapi.NewInlineKeyboardRow(
			tbapi.NewInlineKeyboardButtonData("✅ Принять", fmt.Sprintf("%s%d", appealAcceptPrefix, appeal.ID)),
			tbapi.NewInlineKeyboardButtonData("❌ Отклонить", fmt.Sprintf("%s%d", appealRejectPrefix, appeal.ID)),
		),
	)
	if _, err := h.tbAPI.Send(msgConfig); err != nil {
		log.Printf("[WARN] failed to send appeal notification to admin chat: %v", err)
	}
}

// appealAppellantLabel renders a "<name> (<id>)" label for the appellant.
func appealAppellantLabel(u *tbapi.User) string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		name = u.UserName
	}
	if name == "" {
		return fmt.Sprintf("user %d", u.ID)
	}
	return fmt.Sprintf("%s (%d)", name, u.ID)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./app/events/ -run TestAppealHandler_Handle -v`
Expected: PASS for all sub-tests.

- [ ] **Step 5: Commit**

```bash
git add app/events/appeal_handler.go app/events/appeal_handler_test.go
git commit -m "feat: add appeal /start deep-link handler"
```

---

## Task 8: Route private-chat /start to the appeal handler

**Files:**
- Modify: `app/events/listener.go:86` area (`TelegramListener` struct — add `AppealService` field), `:95-109` (handler fields), `:239-257` (`initHandlers`), `:289-385` (`handleUpdate`)
- Test: `app/events/appeal_handler_test.go` (add a listener-routing test)

- [ ] **Step 1: Write the failing test**

Add to `app/events/appeal_handler_test.go`:

```go
func TestProcAppealStart(t *testing.T) {
	filer := &fakeAppealFiler{
		incident:    audit.Incident{ID: 10, SpamUserID: 555, Status: audit.IncidentStatusOpen},
		existingErr: errors.New("no appeal yet"),
		submitted:   audit.Appeal{ID: 88},
	}
	var sent []tbapi.MessageConfig
	mockAPI := &mocks.TbAPIMock{
		SendFunc: func(c tbapi.Chattable) (tbapi.Message, error) {
			sent = append(sent, c.(tbapi.MessageConfig))
			return tbapi.Message{}, nil
		},
	}
	l := &TelegramListener{appealHandler: newAppealHandler(mockAPI, filer, 12345)}

	update := tbapi.Update{Message: newAppealTestMessage(555, "/start 10")}
	assert.True(t, l.procAppealStart(context.Background(), update), "/start with payload is handled")
	assert.Equal(t, []int64{10}, filer.submitCalls)

	bare := tbapi.Update{Message: newAppealTestMessage(555, "/start")}
	assert.False(t, l.procAppealStart(context.Background(), bare), "bare /start is not handled here")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/events/ -run TestProcAppealStart -v`
Expected: compile failure — `TelegramListener` has no `appealHandler` field and no `procAppealStart` method.

- [ ] **Step 3: Add the `AppealService` and `appealHandler` fields**

In `app/events/listener.go`, add the `AppealService` field next to `IncidentCreator` (added in Task 4):

```go
	AuditWriter             AuditWriter
	IncidentCreator         IncidentCreator
	AppealService           *audit.AppealService
```

Add `audit` to the imports of `listener.go`:

```go
	"github.com/umputun/tg-spam/app/audit"
```

Add the `appealHandler` field to the unexported handler block (next to `adminHandler` / `reportsHandler`, around line 95):

```go
	adminHandler    *admin
	reportsHandler  *userReports
	appealHandler   *appealHandler
```

- [ ] **Step 4: Build the appeal handler in `initHandlers`**

In `app/events/listener.go`, at the end of `initHandlers` (after the `reportsHandler` assignment, before the closing brace at line 257) add:

```go
	if l.AppealService != nil {
		l.appealHandler = newAppealHandler(l.TbAPI, l.AppealService, l.adminChatID)
	}
```

- [ ] **Step 5: Add `procAppealStart`**

In `app/events/listener.go`, add this method (place it just below `handleCallback`):

```go
// procAppealStart routes a private-chat "/start <incidentID>" deep link to the
// appeal handler. It returns false for a bare /start or any non-/start text so
// the message continues through normal processing.
func (l *TelegramListener) procAppealStart(ctx context.Context, update tbapi.Update) (handled bool) {
	if l.appealHandler == nil || update.Message == nil {
		return false
	}
	const prefix = "/start "
	text := strings.TrimSpace(update.Message.Text)
	if !strings.HasPrefix(text, prefix) {
		return false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(text, prefix))
	if payload == "" {
		return false
	}
	if err := l.appealHandler.Handle(ctx, update.Message, payload); err != nil {
		log.Printf("[WARN] failed to handle appeal /start: %v", err)
	}
	return true
}
```

- [ ] **Step 6: Hook it into `handleUpdate`**

In `app/events/listener.go`, in `handleUpdate`, the block at lines 327-329 reads:

```go
	if update.Message == nil {
		return nil
	}
```

Insert the private-chat branch right after it:

```go
	if update.Message == nil {
		return nil
	}

	if update.Message.Chat.Type == "private" && l.procAppealStart(ctx, update) {
		return nil
	}
```

- [ ] **Step 7: Run tests and build**

Run: `go test ./app/events/ -run TestProcAppealStart -v`
Expected: PASS.

Run: `go test -race ./app/events/...`
Expected: PASS (existing tests unaffected — bare `/start` and group messages keep their current path).

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 8: Commit**

```bash
git add app/events/listener.go app/events/appeal_handler_test.go
git commit -m "feat: route private /start deep links to appeal handler"
```

---

## Task 9: Admin-chat appeal Accept/Reject callbacks

**Files:**
- Modify: `app/events/admin.go:19-38` (`admin` struct — add `appeals` field)
- Modify: `app/events/admin_callbacks.go:1-15` (imports), `:17-80` (`InlineCallbackHandler` dispatch), and add `callbackAppealResolve` + `appealResolver` interface
- Modify: `app/events/listener.go:239-257` (`initHandlers` — wire `admin.appeals`)
- Test: `app/events/admin_callbacks` test file (add to an existing `admin_part*_test.go` or create `app/events/appeal_callbacks_test.go`)

- [ ] **Step 1: Write the failing test**

Create `app/events/appeal_callbacks_test.go`:

```go
package events

import (
	"context"
	"errors"
	"testing"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/audit"
	"github.com/umputun/tg-spam/app/events/mocks"
)

type fakeAppealResolver struct {
	appeal      audit.Appeal
	appealErr   error
	acceptCalls []int64
	rejectCalls []int64
}

func (f *fakeAppealResolver) GetAppeal(_ context.Context, _ int64) (audit.Appeal, error) {
	return f.appeal, f.appealErr
}

func (f *fakeAppealResolver) Accept(_ context.Context, appealID int64, _, _ string) error {
	f.acceptCalls = append(f.acceptCalls, appealID)
	return nil
}

func (f *fakeAppealResolver) Reject(_ context.Context, appealID int64, _, _ string) error {
	f.rejectCalls = append(f.rejectCalls, appealID)
	return nil
}

func appealCallbackQuery(data string) *tbapi.CallbackQuery {
	return &tbapi.CallbackQuery{
		ID:      "cb1",
		From:    &tbapi.User{ID: 1, UserName: "admin"},
		Data:    data,
		Message: &tbapi.Message{MessageID: 50, Chat: tbapi.Chat{ID: 12345}, Text: "📩 Апелляция"},
	}
}

func TestAdmin_callbackAppealResolve_Accept(t *testing.T) {
	resolver := &fakeAppealResolver{appeal: audit.Appeal{ID: 88, Status: audit.AppealNew}}
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
		SendFunc:    func(tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
	}
	a := &admin{tbAPI: mockAPI, adminChatID: 12345, appeals: resolver}

	err := a.InlineCallbackHandler(context.Background(), appealCallbackQuery("AA88"))
	require.NoError(t, err)
	assert.Equal(t, []int64{88}, resolver.acceptCalls)
}

func TestAdmin_callbackAppealResolve_Reject(t *testing.T) {
	resolver := &fakeAppealResolver{appeal: audit.Appeal{ID: 88, Status: audit.AppealNew}}
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
		SendFunc:    func(tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
	}
	a := &admin{tbAPI: mockAPI, adminChatID: 12345, appeals: resolver}

	err := a.InlineCallbackHandler(context.Background(), appealCallbackQuery("AR88"))
	require.NoError(t, err)
	assert.Equal(t, []int64{88}, resolver.rejectCalls)
}

func TestAdmin_callbackAppealResolve_AlreadyResolved(t *testing.T) {
	resolver := &fakeAppealResolver{appeal: audit.Appeal{ID: 88, Status: audit.AppealAccepted}}
	var answered string
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(c tbapi.Chattable) (*tbapi.APIResponse, error) {
			if cb, ok := c.(tbapi.CallbackConfig); ok {
				answered = cb.Text
			}
			return &tbapi.APIResponse{Ok: true}, nil
		},
		SendFunc: func(tbapi.Chattable) (tbapi.Message, error) { return tbapi.Message{}, nil },
	}
	a := &admin{tbAPI: mockAPI, adminChatID: 12345, appeals: resolver}

	err := a.InlineCallbackHandler(context.Background(), appealCallbackQuery("AA88"))
	require.NoError(t, err)
	assert.Empty(t, resolver.acceptCalls, "already-resolved appeal is not re-accepted")
	assert.Equal(t, "Апелляция уже рассмотрена", answered)
}

func TestAdmin_callbackAppealResolve_BadID(t *testing.T) {
	resolver := &fakeAppealResolver{appealErr: errors.New("unused")}
	mockAPI := &mocks.TbAPIMock{
		RequestFunc: func(tbapi.Chattable) (*tbapi.APIResponse, error) { return &tbapi.APIResponse{Ok: true}, nil },
	}
	a := &admin{tbAPI: mockAPI, adminChatID: 12345, appeals: resolver}

	err := a.InlineCallbackHandler(context.Background(), appealCallbackQuery("AAxyz"))
	require.Error(t, err)
}
```

If `tbapi.CallbackConfig` is not the concrete type returned by `tbapi.NewCallback`, adjust the type assertion in `TestAdmin_callbackAppealResolve_AlreadyResolved` to the type `tbapi.NewCallback` actually returns (run `go doc github.com/OvyFlash/telegram-bot-api.NewCallback`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/events/ -run TestAdmin_callbackAppealResolve -v`
Expected: compile failure — `admin` has no `appeals` field.

- [ ] **Step 3: Add the `appeals` field to the `admin` struct**

In `app/events/admin.go`, add the field to the `admin` struct (after `aggressiveCleanupLimit int`, line 37):

```go
	aggressiveCleanup      bool
	aggressiveCleanupLimit int
	appeals                appealResolver
```

- [ ] **Step 4: Add the `appealResolver` interface and `callbackAppealResolve`**

In `app/events/admin_callbacks.go`, add `strconv` and `audit` to the import block:

```go
import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	tbapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/hashicorp/go-multierror"

	"github.com/umputun/tg-spam/app/audit"
	"github.com/umputun/tg-spam/app/bot"
)
```

Add the interface and handler at the end of `admin_callbacks.go`:

```go
// appealResolver resolves user appeals from the admin-chat inline buttons.
type appealResolver interface {
	GetAppeal(ctx context.Context, appealID int64) (audit.Appeal, error)
	Accept(ctx context.Context, appealID int64, resolverID, resolutionText string) error
	Reject(ctx context.Context, appealID int64, resolverID, resolutionText string) error
}

// callbackAppealResolve handles the "✅ Принять" / "❌ Отклонить" admin buttons.
// A second tap on an already-resolved appeal is answered with a notice and
// performs no action.
func (a *admin) callbackAppealResolve(ctx context.Context, query *tbapi.CallbackQuery, accept bool) error {
	if a.appeals == nil {
		return fmt.Errorf("appeal resolver not configured")
	}

	prefix := appealAcceptPrefix
	if !accept {
		prefix = appealRejectPrefix
	}
	appealID, err := strconv.ParseInt(strings.TrimPrefix(query.Data, prefix), 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse appeal id from %q: %w", query.Data, err)
	}

	ap, err := a.appeals.GetAppeal(ctx, appealID)
	if err != nil {
		return fmt.Errorf("failed to load appeal %d: %w", appealID, err)
	}
	if ap.Status == audit.AppealAccepted || ap.Status == audit.AppealRejected {
		if _, rErr := a.tbAPI.Request(tbapi.NewCallback(query.ID, "Апелляция уже рассмотрена")); rErr != nil {
			return fmt.Errorf("failed to answer callback: %w", rErr)
		}
		return nil
	}

	if _, err := a.tbAPI.Request(tbapi.NewCallback(query.ID, "принято")); err != nil {
		return fmt.Errorf("failed to answer callback: %w", err)
	}

	resolverID := query.From.UserName
	if resolverID == "" {
		resolverID = fmt.Sprintf("%d", query.From.ID)
	}

	outcome := "✅ апелляция принята"
	if accept {
		err = a.appeals.Accept(ctx, appealID, resolverID, "")
	} else {
		outcome = "❌ апелляция отклонена"
		err = a.appeals.Reject(ctx, appealID, resolverID, "")
	}
	if err != nil {
		return fmt.Errorf("failed to resolve appeal %d: %w", appealID, err)
	}

	updText := query.Message.Text + fmt.Sprintf("\n\n%s администратором %s за %v",
		outcome, markdownUserLink(query.From.UserName, query.From.ID), sinceQuery(query))
	editMsg := tbapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, updText)
	editMsg.ReplyMarkup = &tbapi.InlineKeyboardMarkup{InlineKeyboard: [][]tbapi.InlineKeyboardButton{}}
	if err := send(editMsg, a.tbAPI); err != nil {
		return fmt.Errorf("failed to edit appeal message, chatID:%d, msgID:%d, %w",
			query.Message.Chat.ID, query.Message.MessageID, err)
	}
	return nil
}
```

- [ ] **Step 5: Dispatch the new prefixes in `InlineCallbackHandler`**

In `app/events/admin_callbacks.go`, in `InlineCallbackHandler`, insert these two blocks after the `warnHamCancel` block (after line 71) and before the final `callbackUnbanConfirmed` fallthrough (line 73):

```go
	if strings.HasPrefix(callbackData, appealAcceptPrefix) {
		if err := a.callbackAppealResolve(ctx, query, true); err != nil {
			return fmt.Errorf("failed to accept appeal: %w", err)
		}
		log.Printf("[INFO] appeal accepted, chatID: %d, data: %s", chatID, callbackData)
		return nil
	}

	if strings.HasPrefix(callbackData, appealRejectPrefix) {
		if err := a.callbackAppealResolve(ctx, query, false); err != nil {
			return fmt.Errorf("failed to reject appeal: %w", err)
		}
		log.Printf("[INFO] appeal rejected, chatID: %d, data: %s", chatID, callbackData)
		return nil
	}
```

- [ ] **Step 6: Wire `admin.appeals` in `initHandlers`**

In `app/events/listener.go`, in `initHandlers`, after the `l.adminHandler = &admin{...}` literal and before the `reportsHandler` assignment, add:

```go
	if l.AppealService != nil {
		l.adminHandler.appeals = l.AppealService
	}
```

- [ ] **Step 7: Run tests and build**

Run: `go test ./app/events/ -run TestAdmin_callbackAppealResolve -v`
Expected: PASS for all four sub-tests.

Run: `go test -race ./app/events/...`
Expected: PASS.

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 8: Commit**

```bash
git add app/events/admin.go app/events/admin_callbacks.go app/events/listener.go app/events/appeal_callbacks_test.go
git commit -m "feat: handle admin appeal accept/reject callbacks"
```

---

## Task 10: Wire the appeal flow into runtime assembly

**Files:**
- Modify: `app/runtime_assembly.go:441-512` (`makeTelegramListener`)

- [ ] **Step 1: Wire `AppealService` onto the listener and register the bot adapter**

In `app/runtime_assembly.go`, in `makeTelegramListener`, after the existing `if a.AuditService != nil { ... }` block (the audit-writer block updated in Task 4) and before `if a.UsageMetering != nil {`, add:

```go
	if a.AppealService != nil {
		listener.AppealService = a.AppealService
	}
```

Then, just before `return listener` (after `a.Web.BotUsername = listener.BotUsername`, line 510), add:

```go
	if a.AppealService != nil {
		a.AppealService.SetBotService(events.NewAppealBotAdapter(listener))
	}
```

- [ ] **Step 2: Build and run the full assembly tests**

Run: `go build ./...`
Expected: builds clean.

Run: `go test -race ./app/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add app/runtime_assembly.go
git commit -m "feat: wire appeal flow into runtime assembly"
```

---

## Task 11: Documentation and full verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Document the appeal flow in README**

Open `README.md` and find the section that describes warnings / moderation escalation (search for "Warn" or "WarnDeleteDuration"). Add a subsection there:

```markdown
### Appeal flow

When the bot warns or bans a user it posts a group-chat message carrying an
**"Обжаловать"** (Appeal) inline button. A ban now posts its own group message
(previously only the admin chat was notified); both the warn and the ban
message auto-delete after `WarnDeleteDuration`, so the appeal button is
available for that window.

Tapping the button opens the bot DM and files a one-tap appeal (no reason
text). The appeal is sent to the admin chat with **Принять / Отклонить**
buttons. Accepting unbans the user, clears all of their warning strikes and
DMs them the outcome; rejecting closes the incident and DMs the user. Each
incident accepts a single appeal — a moderator decision is final.

The same accept/reject behavior backs the web `/appeals` admin UI, so an
appeal resolved on the website unbans and notifies the user identically.
```

If a `CHANGES`/changelog section exists, add a matching one-line entry.

- [ ] **Step 2: Normalize comments**

Run: `command -v unfuck-ai-comments >/dev/null || go install github.com/umputun/unfuck-ai-comments@latest; unfuck-ai-comments run --fmt --skip=mocks ./...`
Expected: completes; review and keep any changes it makes.

- [ ] **Step 3: Run the full test suite**

Run: `go test -race ./...`
Expected: PASS.

- [ ] **Step 4: Run the linter**

Run: `golangci-lint run`
Expected: no issues.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: document the appeal flow"
```

---

## Self-Review

**Spec coverage:**

- **C1 — group-chat warn/ban message + appeal button:** Task 2 (warn button), Task 3 (ban message executor), Task 4 (incident-first ordering, ban group message, button wiring). Incident is created before the message via `ensureIncident`. ✔
- **C2 — `/start` deep-link handler:** Task 7 (`appealHandler`, validation, Submit, replies), Task 8 (private-chat routing in `handleUpdate`). All six validation outcomes (garbage, missing incident, wrong user, closed incident, duplicate appeal, success) are covered and tested. ✔
- **C3 — admin-chat notification + decision:** Task 7 (`notifyAdmin` posts with `AA`/`AR` buttons), Task 9 (dispatch + `callbackAppealResolve` + double-tap guard). ✔
- **C4 — shared resolution behavior:** Task 5 (`BotService` extended with `ClearUserWarnings`/`NotifyAppealResult`; `Accept`/`Reject` call them; `SetBotService`), Task 6 (`appealBotAdapter`), Task 10 (wiring). Web `/appeals` shares the path because it uses the same `*audit.AppealService` instance. ✔
- **Ban message auto-deletes via `WarnDeleteDuration`:** Task 3 `scheduleDelete`, Task 4 `postBanGroupMessage` passes `l.ModerationConfig.WarnDeleteDuration`. ✔
- **Channel bans unchanged:** `postBanGroupMessage` returns early when `pc.resp.ChannelID != 0`. ✔
- **Notify user of result:** Task 5 `NotifyAppealResult` in both `Accept` and `Reject`; Task 6 implements the DM. ✔
- **One appeal per incident:** Task 7 step-5 existence check; Task 9 double-tap guard on appeal status. ✔
- **Incidents-subsystem-disabled degradation:** `ensureIncident` returns 0 when `IncidentCreator == nil`; `appealKeyboard` returns `false` for incident id 0, so the message posts with no button and the rest of warn/ban is unaffected. ✔
- **Deviations from spec, documented above:** manual `/warn` `/ban` appeal buttons are deferred; the `appeals.incident_id` unique index is omitted because updates are processed sequentially. Both are stated in the scope notes and must be raised in the execution handoff.

**Placeholder scan:** Two intentional, clearly-flagged spots remain — both are *test-adjustment* notes, not implementation placeholders: Task 4 step 8 (a pre-existing ban test may need its expected `Send`/`postBanCalls` count updated) and Task 9 step 1 / Task 7 step 1 (a type-assertion may need correcting against the actual `tbapi` API via `go doc`). These cannot be pinned down without compiling against the vendored `tbapi` version; each gives the exact command to resolve it.

**Type consistency:** `IncidentCreator.CreateIncident` returns `(int64, error)` everywhere (interface, `incidentAdapter`, `audit.Service`). `warnRequest.incidentID`/`botUsername`, `banMessageRequest`, and `pipelineContext.incidentID` are `int64`/`string` consistently. `appealAcceptPrefix`/`appealRejectPrefix` (`"AA"`/`"AR"`) are defined once in `appeal_handler.go` and reused in `admin_callbacks.go`. `appealFiler` (handler) and `appealResolver` (callbacks) are distinct consumer interfaces, both satisfied by `*audit.AppealService`. `BotService` has four methods, matched by `appealBotAdapter` and `mockBotService`.
