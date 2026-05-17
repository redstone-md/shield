# Web-based Configuration — Design

- **Date:** 2026-05-16
- **Status:** Approved (pending spec review)
- **Scope:** Project A — editable web UI for detection settings. Project B (app-wide UI restyle) is a separate effort, not covered here.

## Problem

Detection and moderation settings are configured through a large `.env` / CLI flag
sheet (`options` struct in `app/main.go`, ~125 flags). Re-tuning a threshold means
SSH to the server, editing `.env`, and restarting. There is a read-only settings
page (`GET /list_settings`) but no way to change values from the UI.

## Goals

- Edit detection/moderation tuning settings from the web UI, applied live without a restart.
- Keep `.env` working for backwards compatibility.
- Cover the settings actually re-tuned in practice: thresholds, per-check toggles,
  LLM mode/veto/consensus, LLM models and prompts.

## Non-goals

- App-wide UI restyle / design system (separate project B).
- Editing secrets (API tokens) or infrastructure values via the UI.
- Editing listener-behaviour flags outside detection tuning.

## Existing infrastructure (reused, already built)

| Component | Location | Role |
|---|---|---|
| `storage.RuleSets` | `app/storage/rule_sets.go` | DB store, versioned (`rule_sets` + `rule_set_versions`, JSON payload), history, bootstrap |
| `controlplane.RuleSetService` | `app/controlplane/ruleset_service.go` | `Get` (cached), `Update` (persist + notify), `OnChange` subscribers |
| `wireLiveReload` | `app/runtime_assembly.go:484` | ruleset change → `Detector.UpdateConfig()`, `Listener.ApplyRuleSet()`, `Bot.ApplyRuleSet()`, meta-checks rebuild |
| REST API | `app/webapi/routes.go` | `GET /api/rules/`, `PUT /api/rules/` (JSON) |
| `buildDetectorConfig` | `app/assembly.go:326` | `rules.RuleSet` → `tgspam.Config` mapping |
| `slowpath.FilePromptRegistry` | `app/slowpath/prompt_registry.go` | file-backed, versioned, per-provider vision prompt store (`Active`/`Set`) |

The hot-reload pipeline already exists. The single missing piece is an editable
HTML/HTMX page. Two backend gaps remain (Parts 2a, 2b below).

## Work breakdown

### Part 2a — extend `rules.RuleSet`

Detection knobs currently read straight from `opts` (env) in `buildDetectorConfig`
are moved into `RuleSet` so they become editable and hot-reloadable.

Two new nested structs on `rules.RuleSet`:

```go
Detection DetectionRules `json:"detection"`
LLM       LLMCommonRules `json:"llm"`

type DetectionRules struct {
    MaxEmoji            int     `json:"max_emoji"`
    MinMsgLen           int     `json:"min_msg_len"`
    SimilarityThreshold float64 `json:"similarity_threshold"`
    MinSpamProbability  float64 `json:"min_spam_probability"` // classifier
    MultiLangWords      int     `json:"multi_lang_words"`
    CasEnabled          bool    `json:"cas_enabled"`
    HistorySize         int     `json:"history_size"`
    FirstMessagesCount  int     `json:"first_messages_count"`
    ParanoidMode        bool    `json:"paranoid_mode"`
}

type LLMCommonRules struct {
    Mode      string `json:"mode"`      // "" | missed | flagged | always
    Consensus string `json:"consensus"` // any | all
}
```

`LLMRules` (used for both OpenAI and Gemini) gains:

```go
Prompt      string `json:"prompt"`       // main system prompt; empty = builtin default
VisionModel string `json:"vision_model"` // empty = use Model
```

`buildDetectorConfig` is updated to read these fields from `ruleSet` instead of `opts`.

**Edge decisions:**
- CAS API URL / UserAgent / Timeout stay in `.env` (infrastructure endpoint).
  `DetectionRules.CasEnabled` is a toggle: `false` skips CAS even when the URL is set.
- `ScoringThreshold` is not currently wired as an `opts` flag (not set in
  `buildDetectorConfig`, defaults to 0 → boolean-OR). It is **not** added to `RuleSet`
  in this project. See Open items.

### Part 2b — precedence (env backwards-compat + UI warnings)

The existing model is kept: `applyExplicitRuleSetOverrides` (`app/settings_precedence.go`)
lets explicitly-set env/CLI values override the DB ruleset at startup and on each
live reload. This stays — existing deployments do not break.

Changes:
- **Empty env value = not set.** `configured(flag, env)` currently treats a present
  but empty env var (`FOO=`) as configured. It must treat an empty value as not set,
  so the DB value is used instead of being overwritten with empty.
- **Env-pinned field reporting.** A function exposes the set of field keys currently
  pinned by env/CLI (reusing the `configured()` logic). The UI uses this to render a
  warning badge on those fields.
- New `RuleSet` fields from Part 2a get matching `configured()` entries so the same
  override + warning behaviour applies to them.

**Behaviour:** a field present in `.env` overrides the DB ruleset on restart. The UI
still saves edits to such a field (staged in DB) and warns the user. Removing the
field from `.env` and restarting makes the DB value (including the staged UI edit)
take effect. Workflow: "empty the tuning section of `.env`, manage from the UI."

### Part 2c — prompts (three surfaces)

| Prompt | Current source | Plan |
|---|---|---|
| OpenAI/Gemini text system prompt | `opts.OpenAI.Prompt` / `opts.Gemini.Prompt` (env, builtin fallback) | → `LLMRules.Prompt` (Part 2a) |
| OpenAI/Gemini custom prompts | `LLMRules.CustomPrompts` | already in `RuleSet` |
| Vision prompt | hardcoded `defaultVisionPrompt` const in `checkVision` | → `RuleSet.LLM.VisionPrompt`; const stays as empty-value fallback |

Investigation finding (revises the earlier assumption): the slowpath `checkVision`
hardcodes `defaultVisionPrompt` and ignores the `PromptRegistry` entirely. The
`PromptRegistry` only carries the slowpath *text* system prompt, and it is an
`InMemoryPromptRegistry` (ephemeral). It is not a viable persistence layer.

Decision: consolidate all prompts into the DB `RuleSet` and remove `PromptRegistry`.
- Slowpath text system prompt → sourced per-provider from `RuleSet.OpenAI.Prompt` /
  `RuleSet.Gemini.Prompt`, pushed onto the engine via `Engine.SetSystemPrompt`.
- Vision prompt → one shared `RuleSet.LLM.VisionPrompt` (`LLMCommonRules`), pushed via
  `Engine.SetVisionPrompt`; empty value falls back to the `defaultVisionPrompt` const.
- `PromptRegistry`, `FilePromptRegistry`, `InMemoryPromptRegistry`, `PromptEntry`, and
  `configureSlowPathPrompts` are deleted.

This is implemented by Plan 2 (`docs/superpowers/plans/2026-05-16-web-config-llm-prompts.md`).
Plan 2 also adds prompt hot-reload: `wireLiveReload` rebuilds the OpenAI/Gemini text
checkers and re-pushes the slowpath prompts on every ruleset change.

### Part 1 — editable web UI

The read-only settings page becomes editable.

- Settings grouped into sections (detection, meta, OpenAI, Gemini, LLM, vision, etc.).
- Field inputs by type: number, checkbox, text, textarea (prompts).
- Env-pinned fields show a warning badge ("set in env — will be overwritten on
  restart; remove from env to manage here"). The field stays editable.
- Per-section "Save" button. Save → server validates → persists → hot-reload applies.
- Detection/meta/LLM/etc. sections save through `RuleSetService.Update`
  (each save is a new ruleset version; history is preserved).
- The vision-prompt section saves through `PromptRegistry.Set`.
- HTMX form posts; no new JS framework. Native HTML5 input constraints
  (`type=number`, `min`, `max`, `step`, `required`, `pattern`) give instant
  client-side feedback.
- The new settings page is styled cleanly within the existing Bootstrap/HTMX stack
  (grouped cards, consistent spacing). A full app-wide restyle is out of scope.

### Validation

- **Server-side (authoritative, Go):** per-field validators (type, min/max, allowed
  values) driven by a field registry — one source of truth shared by validation,
  form rendering, and HTML5 attribute generation.
- **Client-side (UX only):** native HTML5 constraints, no JS.
- On invalid input the server returns the form fragment with inline errors and
  persists nothing.

### Error handling

- All-or-nothing: validate the whole submitted section, then persist, then apply.
  No partial application.
- Persist failure → not applied, error surfaced to the UI.
- Every change written to the audit log (who, what, when) via the existing audit package.
- Concurrent edits: last-write-wins; each save bumps the ruleset version so all
  versions remain in history. No locking.

### Hot-reload

Reuses `wireLiveReload`. `RuleSetService.Update` notifies subscribers →
`Detector.UpdateConfig`, `Listener.ApplyRuleSet`, `Bot.ApplyRuleSet`, meta-checks
rebuild. New `DetectionRules` / `LLMCommonRules` fields are applied through the
existing `buildDetectorConfig` path. No restart.

## Testing

- `buildDetectorConfig` maps the new `DetectionRules` / `LLMCommonRules` / `LLMRules`
  fields correctly.
- `bootstrapRuleSet` seeds the new fields from env on first boot.
- `configured()` treats an empty env value as not-set.
- Env-pinned field reporting returns the correct key set.
- UI handler: form parsing, validation errors render inline, valid save creates a new
  ruleset version and triggers hot-reload.
- Vision-prompt save persists through `PromptRegistry`.

## Migration / risks

- Deployments with a DB ruleset already bootstrapped by the old logic have a
  version-1 ruleset without the new `Detection` / `LLM` fields. On load these
  decode as Go zero values. The bootstrap path must backfill missing new fields
  from env/defaults on the first boot after this change, so a ruleset is never
  partially populated.
- Vision-prompt editing depends on the `PromptRegistry` being wired into the
  runtime (see Part 2c).

## Open items

- `ScoringThreshold` is not an `opts` flag today; excluded from this project. If it
  becomes a real knob it can be added to `DetectionRules` later.
- Confirm whether the slowpath `PromptRegistry` is wired in the runtime; wire it if not.

## Out of scope (separate projects)

- Project B: app-wide UI restyle / design system.
- Editing secrets, infrastructure, or listener-behaviour flags via the UI.
