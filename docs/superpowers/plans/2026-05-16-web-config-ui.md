# Web Config — Editable Settings UI (Plan 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** An editable HTMX settings page that lets an admin change the detection/LLM tuning in the versioned `RuleSet` from the browser, applied live without a restart.

**Architecture:** A new `GET /settings/edit` page renders a Bootstrap/HTMX form grouped by section, pre-filled from the current ruleset (`RuleSetProvider.Get`). `POST /settings/save` parses the form, builds an updated `rules.RuleSet`, validates it, and calls `RuleSetProvider.Update` — which persists a new version and triggers the existing hot-reload. Settings explicitly pinned by env vars are shown with a warning badge, using a pinned-keys map computed at startup.

**Tech Stack:** Go 1.24, `go-pkgz/routegroup`, HTMX v2, Bootstrap, `stretchr/testify`.

**Depends on:** Plan 1 + Plan 2. Branch `feat/web-config-ui` is stacked on `feat/web-config-llm-prompts`.

**Scope:** Editable UI only. Project B (app-wide restyle) is out of scope; this page follows the existing Bootstrap/HTMX patterns (see `app/webapi/assets/manage_dictionary.html`).

---

## File Structure

- `app/webapi/webapi.go` — add `EnvPinnedKeys map[string]bool` to `Config`.
- `app/webapi/settings_form.go` — new: `ruleSetFromForm` (form → `rules.RuleSet` with validation) and the field-validation helpers.
- `app/webapi/settings_form_test.go` — new: tests for the form decoder/validator.
- `app/webapi/handlers_settings.go` — new: `htmlSettingsEditHandler` (GET) and `saveSettingsHandler` (POST).
- `app/webapi/handlers_settings_test.go` — new: handler tests.
- `app/webapi/assets/settings_edit.html` — new: the editable form template.
- `app/webapi/routes.go` — register the two new routes.
- `app/webapi/assets/settings.html` — add a link to the editor.
- `app/runtime_assembly.go` — populate `webapi.Config.EnvPinnedKeys` with `envPinnedKeys()`.

---

## Task 1: Add `EnvPinnedKeys` to webapi config and wire it

**Files:**
- Modify: `app/webapi/webapi.go` (`Config` struct)
- Modify: `app/runtime_assembly.go` (webapi config construction)
- Test: `app/webapi/webapi_test.go` (add a small test; or `app/webapi/settings_form_test.go` if created first)

- [ ] **Step 1: Write the failing test**

Add to `app/webapi/webapi_test.go` (package `webapi`):

```go
func TestConfig_EnvPinnedKeysField(t *testing.T) {
	cfg := Config{EnvPinnedKeys: map[string]bool{"detection.max_emoji": true}}
	assert.True(t, cfg.EnvPinnedKeys["detection.max_emoji"])
	assert.False(t, cfg.EnvPinnedKeys["detection.min_msg_len"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/webapi/ -run TestConfig_EnvPinnedKeysField -v`
Expected: FAIL — `EnvPinnedKeys` undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/webapi/webapi.go`, add a field to the `Config` struct (place it near `Settings`):

```go
	// EnvPinnedKeys lists RuleSet JSON paths whose value is pinned by an env var
	// and will override the stored ruleset on the next restart.
	EnvPinnedKeys map[string]bool
```

In `app/runtime_assembly.go`, find where `webapi.Config` (or `webapi.Server`) is constructed. Add `EnvPinnedKeys: envPinnedKeys(),` to that config literal. `envPinnedKeys()` is the existing function in `app/settings_precedence.go` (package `main`). Read the construction site to confirm the exact `webapi.Config{...}` literal and add the one field; if the webapi server is built in a different file (`app/assembly.go`), add it there instead — search for `webapi.Config{` or `webapi.Server{` to locate it.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/webapi/ -run TestConfig_EnvPinnedKeysField -v`
Expected: PASS.
Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add app/webapi/webapi.go app/runtime_assembly.go app/webapi/webapi_test.go
git commit -m "feat(webapi): add EnvPinnedKeys config for the settings editor"
```

---

## Task 2: Form decoder — `ruleSetFromForm`

**Files:**
- Create: `app/webapi/settings_form.go`
- Test: `app/webapi/settings_form_test.go`

This task builds the pure function that turns submitted form values into an updated `rules.RuleSet`, with validation. It starts from a `base` ruleset (the current one) so unedited/identity fields are preserved.

- [ ] **Step 1: Write the failing test**

Create `app/webapi/settings_form_test.go`:

```go
package webapi

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/rules"
)

func TestRuleSetFromForm_AppliesValues(t *testing.T) {
	base := rules.RuleSet{WorkspaceID: "tg-spam", Version: 7}
	form := url.Values{
		"detection.max_emoji":            {"5"},
		"detection.similarity_threshold": {"0.6"},
		"detection.cas_enabled":          {"on"},
		"llm.mode":                       {"flagged"},
		"llm.consensus":                  {"any"},
		"llm.vision_prompt":              {"scan image"},
		"openai.veto":                    {"on"},
		"openai.model":                   {"gpt-4o-mini"},
		"openai.prompt":                  {"be strict"},
		"slow_path_enabled":              {"on"},
	}

	rs, errs := ruleSetFromForm(base, form)

	require.Empty(t, errs)
	assert.Equal(t, "tg-spam", rs.WorkspaceID, "identity preserved")
	assert.Equal(t, 7, rs.Version, "version preserved")
	assert.Equal(t, 5, rs.Detection.MaxEmoji)
	assert.InEpsilon(t, 0.6, rs.Detection.SimilarityThreshold, 0.0001)
	assert.True(t, rs.Detection.CasEnabled)
	assert.Equal(t, "flagged", rs.LLM.Mode)
	assert.Equal(t, "scan image", rs.LLM.VisionPrompt)
	assert.True(t, rs.OpenAI.Veto)
	assert.Equal(t, "be strict", rs.OpenAI.Prompt)
	assert.True(t, rs.SlowPathEnabled)
}

func TestRuleSetFromForm_UncheckedCheckboxIsFalse(t *testing.T) {
	base := rules.RuleSet{Detection: rules.DetectionRules{CasEnabled: true}}
	// checkbox absent from the form submission => false
	rs, errs := ruleSetFromForm(base, url.Values{})
	require.Empty(t, errs)
	assert.False(t, rs.Detection.CasEnabled)
}

func TestRuleSetFromForm_InvalidIntRejected(t *testing.T) {
	_, errs := ruleSetFromForm(rules.RuleSet{}, url.Values{"detection.max_emoji": {"abc"}})
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "detection.max_emoji")
}

func TestRuleSetFromForm_NegativeThresholdRejected(t *testing.T) {
	_, errs := ruleSetFromForm(rules.RuleSet{}, url.Values{"detection.similarity_threshold": {"-1"}})
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "similarity_threshold")
}

func TestRuleSetFromForm_InvalidLLMModeRejected(t *testing.T) {
	_, errs := ruleSetFromForm(rules.RuleSet{}, url.Values{"llm.mode": {"bogus"}})
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "llm.mode")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/webapi/ -run TestRuleSetFromForm -v`
Expected: FAIL — `ruleSetFromForm` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `app/webapi/settings_form.go`:

```go
package webapi

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/umputun/tg-spam/app/rules"
)

// ruleSetFromForm applies submitted form values onto a copy of base and returns the
// updated ruleset plus a list of human-readable validation errors. Identity and
// versioning fields on base are preserved. When errs is non-empty the ruleset must
// not be persisted.
func ruleSetFromForm(base rules.RuleSet, form url.Values) (rules.RuleSet, []string) {
	rs := base
	var errs []string

	intField := func(key string, min, max int, target *int) {
		raw := form.Get(key)
		if raw == "" {
			return
		}
		v, err := strconv.Atoi(raw)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: not a whole number", key))
			return
		}
		if v < min || v > max {
			errs = append(errs, fmt.Sprintf("%s: must be between %d and %d", key, min, max))
			return
		}
		*target = v
	}
	floatField := func(key string, min, max float64, target *float64) {
		raw := form.Get(key)
		if raw == "" {
			return
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: not a number", key))
			return
		}
		if v < min || v > max {
			errs = append(errs, fmt.Sprintf("%s: must be between %.2f and %.2f", key, min, max))
			return
		}
		*target = v
	}
	boolField := func(key string) bool { return form.Get(key) == "on" }
	enumField := func(key string, allowed []string, target *string) {
		raw := form.Get(key)
		for _, a := range allowed {
			if raw == a {
				*target = raw
				return
			}
		}
		errs = append(errs, fmt.Sprintf("%s: must be one of %v", key, allowed))
	}

	// detection
	intField("detection.max_emoji", -1, 1000, &rs.Detection.MaxEmoji)
	intField("detection.min_msg_len", 0, 100000, &rs.Detection.MinMsgLen)
	floatField("detection.similarity_threshold", 0, 1, &rs.Detection.SimilarityThreshold)
	floatField("detection.min_spam_probability", 0, 100, &rs.Detection.MinSpamProbability)
	intField("detection.multi_lang_words", 0, 1000, &rs.Detection.MultiLangWords)
	intField("detection.history_size", 0, 1000000, &rs.Detection.HistorySize)
	intField("detection.first_messages_count", 0, 10000, &rs.Detection.FirstMessagesCount)
	rs.Detection.CasEnabled = boolField("detection.cas_enabled")
	rs.Detection.ParanoidMode = boolField("detection.paranoid_mode")

	// llm
	enumField("llm.mode", []string{"", "missed", "flagged", "always"}, &rs.LLM.Mode)
	enumField("llm.consensus", []string{"any", "all"}, &rs.LLM.Consensus)
	rs.LLM.VisionPrompt = form.Get("llm.vision_prompt")

	// openai
	rs.OpenAI.Veto = boolField("openai.veto")
	rs.OpenAI.CheckShortMessages = boolField("openai.check_short_messages")
	rs.OpenAI.Model = form.Get("openai.model")
	rs.OpenAI.Prompt = form.Get("openai.prompt")
	intField("openai.history_size", 0, 1000000, &rs.OpenAI.HistorySize)

	// gemini
	rs.Gemini.Veto = boolField("gemini.veto")
	rs.Gemini.CheckShortMessages = boolField("gemini.check_short_messages")
	rs.Gemini.Model = form.Get("gemini.model")
	rs.Gemini.Prompt = form.Get("gemini.prompt")
	intField("gemini.history_size", 0, 1000000, &rs.Gemini.HistorySize)

	rs.SlowPathEnabled = boolField("slow_path_enabled")

	return rs, errs
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/webapi/ -run TestRuleSetFromForm -v`
Expected: PASS (all five tests).

- [ ] **Step 5: Commit**

```bash
git add app/webapi/settings_form.go app/webapi/settings_form_test.go
git commit -m "feat(webapi): add form decoder for the editable ruleset"
```

---

## Task 3: The editable settings template

**Files:**
- Create: `app/webapi/assets/settings_edit.html`

`embed.FS` already globs `assets/*.html` so a new file is picked up automatically by `tmpl`.

- [ ] **Step 1: Create the template**

Create `app/webapi/assets/settings_edit.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <title>Edit Settings - TG-Spam</title>
    {{template "heads.html"}}
</head>
<body>
{{template "navbar.html"}}

<div class="container mt-4">
    <h2 class="text-center mb-4">Edit Settings</h2>

    <div id="settings-error"></div>
    <div id="settings-status"></div>

    <form hx-post="/settings/save" hx-target="#settings-status" hx-swap="innerHTML">
        {{$pinned := .EnvPinned}}

        <div class="card mb-3">
            <div class="card-header">Detection</div>
            <div class="card-body">
                {{template "settings_int" dict "Key" "detection.max_emoji" "Label" "Max emoji" "Val" .RuleSet.Detection.MaxEmoji "Pinned" $pinned}}
                {{template "settings_int" dict "Key" "detection.min_msg_len" "Label" "Min message length" "Val" .RuleSet.Detection.MinMsgLen "Pinned" $pinned}}
                {{template "settings_float" dict "Key" "detection.similarity_threshold" "Label" "Similarity threshold" "Val" .RuleSet.Detection.SimilarityThreshold "Pinned" $pinned}}
                {{template "settings_float" dict "Key" "detection.min_spam_probability" "Label" "Min spam probability %" "Val" .RuleSet.Detection.MinSpamProbability "Pinned" $pinned}}
                {{template "settings_int" dict "Key" "detection.multi_lang_words" "Label" "Multi-lang words" "Val" .RuleSet.Detection.MultiLangWords "Pinned" $pinned}}
                {{template "settings_int" dict "Key" "detection.history_size" "Label" "History size" "Val" .RuleSet.Detection.HistorySize "Pinned" $pinned}}
                {{template "settings_int" dict "Key" "detection.first_messages_count" "Label" "First messages count" "Val" .RuleSet.Detection.FirstMessagesCount "Pinned" $pinned}}
                {{template "settings_bool" dict "Key" "detection.cas_enabled" "Label" "CAS enabled" "Val" .RuleSet.Detection.CasEnabled "Pinned" $pinned}}
                {{template "settings_bool" dict "Key" "detection.paranoid_mode" "Label" "Paranoid mode" "Val" .RuleSet.Detection.ParanoidMode "Pinned" $pinned}}
            </div>
        </div>

        <div class="card mb-3">
            <div class="card-header">LLM</div>
            <div class="card-body">
                {{template "settings_enum" dict "Key" "llm.mode" "Label" "LLM mode" "Val" .RuleSet.LLM.Mode "Options" .LLMModes "Pinned" $pinned}}
                {{template "settings_enum" dict "Key" "llm.consensus" "Label" "LLM consensus" "Val" .RuleSet.LLM.Consensus "Options" .LLMConsensus "Pinned" $pinned}}
                {{template "settings_text" dict "Key" "llm.vision_prompt" "Label" "Vision prompt" "Val" .RuleSet.LLM.VisionPrompt "Pinned" $pinned}}
            </div>
        </div>

        <div class="card mb-3">
            <div class="card-header">OpenAI</div>
            <div class="card-body">
                {{template "settings_bool" dict "Key" "openai.veto" "Label" "Veto" "Val" .RuleSet.OpenAI.Veto "Pinned" $pinned}}
                {{template "settings_bool" dict "Key" "openai.check_short_messages" "Label" "Check short messages" "Val" .RuleSet.OpenAI.CheckShortMessages "Pinned" $pinned}}
                {{template "settings_str" dict "Key" "openai.model" "Label" "Model" "Val" .RuleSet.OpenAI.Model "Pinned" $pinned}}
                {{template "settings_int" dict "Key" "openai.history_size" "Label" "History size" "Val" .RuleSet.OpenAI.HistorySize "Pinned" $pinned}}
                {{template "settings_text" dict "Key" "openai.prompt" "Label" "System prompt" "Val" .RuleSet.OpenAI.Prompt "Pinned" $pinned}}
            </div>
        </div>

        <div class="card mb-3">
            <div class="card-header">Gemini</div>
            <div class="card-body">
                {{template "settings_bool" dict "Key" "gemini.veto" "Label" "Veto" "Val" .RuleSet.Gemini.Veto "Pinned" $pinned}}
                {{template "settings_bool" dict "Key" "gemini.check_short_messages" "Label" "Check short messages" "Val" .RuleSet.Gemini.CheckShortMessages "Pinned" $pinned}}
                {{template "settings_str" dict "Key" "gemini.model" "Label" "Model" "Val" .RuleSet.Gemini.Model "Pinned" $pinned}}
                {{template "settings_int" dict "Key" "gemini.history_size" "Label" "History size" "Val" .RuleSet.Gemini.HistorySize "Pinned" $pinned}}
                {{template "settings_text" dict "Key" "gemini.prompt" "Label" "System prompt" "Val" .RuleSet.Gemini.Prompt "Pinned" $pinned}}
            </div>
        </div>

        <div class="card mb-3">
            <div class="card-body">
                {{template "settings_bool" dict "Key" "slow_path_enabled" "Label" "Slow-path enabled" "Val" .RuleSet.SlowPathEnabled "Pinned" $pinned}}
            </div>
        </div>

        <button type="submit" class="btn btn-primary">Save</button>
    </form>
</div>

</body>
</html>

{{define "settings_pinned_badge"}}
{{if index .Pinned .Key}}<span class="badge bg-warning text-dark ms-2" title="Set by env var; this will be overwritten on restart. Remove it from env to manage it here.">env-pinned</span>{{end}}
{{end}}

{{define "settings_int"}}
<div class="mb-3 row">
    <label class="col-sm-4 col-form-label">{{.Label}}{{template "settings_pinned_badge" .}}</label>
    <div class="col-sm-8">
        <input type="number" step="1" class="form-control" name="{{.Key}}" value="{{.Val}}">
    </div>
</div>
{{end}}

{{define "settings_float"}}
<div class="mb-3 row">
    <label class="col-sm-4 col-form-label">{{.Label}}{{template "settings_pinned_badge" .}}</label>
    <div class="col-sm-8">
        <input type="number" step="any" class="form-control" name="{{.Key}}" value="{{.Val}}">
    </div>
</div>
{{end}}

{{define "settings_str"}}
<div class="mb-3 row">
    <label class="col-sm-4 col-form-label">{{.Label}}{{template "settings_pinned_badge" .}}</label>
    <div class="col-sm-8">
        <input type="text" class="form-control" name="{{.Key}}" value="{{.Val}}">
    </div>
</div>
{{end}}

{{define "settings_text"}}
<div class="mb-3 row">
    <label class="col-sm-4 col-form-label">{{.Label}}{{template "settings_pinned_badge" .}}</label>
    <div class="col-sm-8">
        <textarea class="form-control" name="{{.Key}}" rows="3">{{.Val}}</textarea>
    </div>
</div>
{{end}}

{{define "settings_bool"}}
<div class="mb-3 row">
    <label class="col-sm-4 col-form-label">{{.Label}}{{template "settings_pinned_badge" .}}</label>
    <div class="col-sm-8">
        <div class="form-check">
            <input type="checkbox" class="form-check-input" name="{{.Key}}" {{if .Val}}checked{{end}}>
        </div>
    </div>
</div>
{{end}}

{{define "settings_enum"}}
<div class="mb-3 row">
    <label class="col-sm-4 col-form-label">{{.Label}}{{template "settings_pinned_badge" .}}</label>
    <div class="col-sm-8">
        <select class="form-select" name="{{.Key}}">
            {{$cur := .Val}}
            {{range .Options}}<option value="{{.}}" {{if eq . $cur}}selected{{end}}>{{if eq . ""}}(default){{else}}{{.}}{{end}}</option>{{end}}
        </select>
    </div>
</div>
{{end}}
```

This template uses a `dict` helper (Go templates have no built-in map constructor) — Task 4 registers it.

- [ ] **Step 2: Verify the file is syntactically embeddable**

Run: `go build ./app/webapi/...`
Expected: builds clean (the template is not parsed until runtime, but the file must be valid for `embed`).

- [ ] **Step 3: Commit**

```bash
git add app/webapi/assets/settings_edit.html
git commit -m "feat(webapi): add editable settings page template"
```

---

## Task 4: Register the `dict` template helper

**Files:**
- Modify: `app/webapi/webapi.go` (template construction, ~line 40-42)
- Test: `app/webapi/webapi_test.go`

The `settings_edit.html` template calls `dict "Key" "x" ...`. Go's `html/template` has no map literal; register a `dict` function.

- [ ] **Step 1: Write the failing test**

Add to `app/webapi/webapi_test.go`:

```go
func TestTemplateDictHelper(t *testing.T) {
	m, err := templateDict("A", 1, "B", "two")
	require.NoError(t, err)
	assert.Equal(t, 1, m["A"])
	assert.Equal(t, "two", m["B"])

	_, err = templateDict("oddNumberOfArgs")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/webapi/ -run TestTemplateDictHelper -v`
Expected: FAIL — `templateDict` undefined.

- [ ] **Step 3: Write minimal implementation**

In `app/webapi/webapi.go`, add the helper function:

```go
// templateDict builds a map from alternating key/value pairs, for use as a
// html/template FuncMap function so templates can pass named arguments to sub-templates.
func templateDict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict requires an even number of arguments, got %d", len(pairs))
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key %d is not a string", i)
		}
		m[key] = pairs[i+1]
	}
	return m, nil
}
```

Change the `tmpl` package variable to register the function. The current line is:

```go
var tmpl = template.Must(template.ParseFS(templateFS, "assets/*.html", "assets/components/*.html"))
```

Replace it with:

```go
var tmpl = template.Must(template.New("").Funcs(template.FuncMap{"dict": templateDict}).
	ParseFS(templateFS, "assets/*.html", "assets/components/*.html"))
```

Confirm `fmt` is imported in `webapi.go` (it almost certainly is; add it if not).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/webapi/ -run TestTemplateDictHelper -v`
Expected: PASS.
Run: `go test ./app/webapi/ 2>&1 | tail -5`
Expected: the `webapi` package tests still PASS (template parsing of all files, including `settings_edit.html`, happens at init — a parse error would fail every test).

- [ ] **Step 5: Commit**

```bash
git add app/webapi/webapi.go app/webapi/webapi_test.go
git commit -m "feat(webapi): register dict template helper"
```

---

## Task 5: GET handler — render the editor

**Files:**
- Create: `app/webapi/handlers_settings.go`
- Test: `app/webapi/handlers_settings_test.go`

- [ ] **Step 1: Write the failing test**

Create `app/webapi/handlers_settings_test.go`:

```go
package webapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/rules"
)

type ruleSetProviderStub struct {
	get     rules.RuleSet
	updated rules.RuleSet
	source  string
	err     error
}

func (s *ruleSetProviderStub) Get(_ context.Context, _ string) (rules.RuleSet, error) {
	return s.get, s.err
}

func (s *ruleSetProviderStub) Update(_ context.Context, _, source string, rs rules.RuleSet) (rules.RuleSet, error) {
	s.source = source
	s.updated = rs
	return rs, s.err
}

func TestHTMLSettingsEditHandler_RendersForm(t *testing.T) {
	prov := &ruleSetProviderStub{get: rules.RuleSet{
		Detection: rules.DetectionRules{MaxEmoji: 5},
		OpenAI:    rules.LLMRules{Model: "gpt-4o-mini"},
	}}
	srv := &Server{Config: Config{
		RuleSetProvider: prov,
		Settings:        Settings{TenantID: "tg-spam"},
		EnvPinnedKeys:   map[string]bool{"detection.max_emoji": true},
	}}

	rr := httptest.NewRecorder()
	srv.htmlSettingsEditHandler(rr, httptest.NewRequest(http.MethodGet, "/settings/edit", http.NoBody))

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, `name="detection.max_emoji"`)
	assert.Contains(t, body, `value="5"`)
	assert.Contains(t, body, "env-pinned", "pinned badge shown for detection.max_emoji")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/webapi/ -run TestHTMLSettingsEditHandler -v`
Expected: FAIL — `htmlSettingsEditHandler` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `app/webapi/handlers_settings.go`:

```go
package webapi

import (
	"net/http"

	"github.com/umputun/tg-spam/app/rules"
)

// llmModeOptions and llmConsensusOptions are the allowed values for the enum selects.
var (
	llmModeOptions      = []string{"", "missed", "flagged", "always"}
	llmConsensusOptions = []string{"any", "all"}
)

// htmlSettingsEditHandler renders the editable settings form, pre-filled from the
// current ruleset.
func (s *Server) htmlSettingsEditHandler(w http.ResponseWriter, r *http.Request) {
	if s.RuleSetProvider == nil {
		http.Error(w, "rule set provider not configured", http.StatusNotImplemented)
		return
	}
	rs, err := s.RuleSetProvider.Get(r.Context(), s.Settings.TenantID)
	if err != nil {
		http.Error(w, "failed to load rule set: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		RuleSet      rules.RuleSet
		EnvPinned    map[string]bool
		LLMModes     []string
		LLMConsensus []string
	}{
		RuleSet:      rs,
		EnvPinned:    s.EnvPinnedKeys,
		LLMModes:     llmModeOptions,
		LLMConsensus: llmConsensusOptions,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "settings_edit.html", data); err != nil {
		http.Error(w, "failed to render: "+err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/webapi/ -run TestHTMLSettingsEditHandler -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/webapi/handlers_settings.go app/webapi/handlers_settings_test.go
git commit -m "feat(webapi): add GET handler for the settings editor"
```

---

## Task 6: POST handler — save the settings

**Files:**
- Modify: `app/webapi/handlers_settings.go`
- Test: `app/webapi/handlers_settings_test.go`

- [ ] **Step 1: Write the failing test**

Add to `app/webapi/handlers_settings_test.go`:

```go
func TestSaveSettingsHandler_ValidUpdate(t *testing.T) {
	prov := &ruleSetProviderStub{get: rules.RuleSet{WorkspaceID: "tg-spam", Version: 3}}
	srv := &Server{Config: Config{RuleSetProvider: prov, Settings: Settings{TenantID: "tg-spam"}}}

	form := url.Values{"detection.max_emoji": {"9"}, "llm.mode": {"flagged"}, "llm.consensus": {"any"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.saveSettingsHandler(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 9, prov.updated.Detection.MaxEmoji)
	assert.Equal(t, "flagged", prov.updated.LLM.Mode)
	assert.Equal(t, "web", prov.source, "update source must be 'web'")
	assert.Contains(t, rr.Body.String(), "Saved")
}

func TestSaveSettingsHandler_ValidationError(t *testing.T) {
	prov := &ruleSetProviderStub{get: rules.RuleSet{WorkspaceID: "tg-spam"}}
	srv := &Server{Config: Config{RuleSetProvider: prov, Settings: Settings{TenantID: "tg-spam"}}}

	form := url.Values{"detection.max_emoji": {"abc"}, "llm.mode": {"flagged"}, "llm.consensus": {"any"}}
	req := httptest.NewRequest(http.MethodPost, "/settings/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.saveSettingsHandler(rr, req)

	assert.Equal(t, "#settings-error", rr.Header().Get("HX-Retarget"))
	assert.Contains(t, rr.Body.String(), "detection.max_emoji")
	assert.Equal(t, rules.RuleSet{}, prov.updated, "nothing persisted on validation error")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/webapi/ -run TestSaveSettingsHandler -v`
Expected: FAIL — `saveSettingsHandler` undefined.

- [ ] **Step 3: Write minimal implementation**

Append to `app/webapi/handlers_settings.go`:

```go
// saveSettingsHandler parses the submitted settings form, validates it, and persists
// the updated ruleset through the rule set provider. On a validation error it returns
// an HTMX error fragment retargeted to #settings-error and persists nothing.
func (s *Server) saveSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if s.RuleSetProvider == nil {
		http.Error(w, "rule set provider not configured", http.StatusNotImplemented)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.settingsError(w, []string{"malformed form: " + err.Error()})
		return
	}

	base, err := s.RuleSetProvider.Get(r.Context(), s.Settings.TenantID)
	if err != nil {
		s.settingsError(w, []string{"failed to load current rule set: " + err.Error()})
		return
	}

	updated, errs := ruleSetFromForm(base, r.Form)
	if len(errs) > 0 {
		s.settingsError(w, errs)
		return
	}

	if _, err := s.RuleSetProvider.Update(r.Context(), s.Settings.TenantID, "web", updated); err != nil {
		s.settingsError(w, []string{"failed to save: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<div class="alert alert-success">Saved. Changes applied live.</div>`))
}

// settingsError writes an HTMX error fragment retargeted to the #settings-error slot.
func (s *Server) settingsError(w http.ResponseWriter, errs []string) {
	w.Header().Set("HX-Retarget", "#settings-error")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	b.WriteString(`<div class="alert alert-danger"><strong>Could not save:</strong><ul>`)
	for _, e := range errs {
		b.WriteString("<li>" + template.HTMLEscapeString(e) + "</li>")
	}
	b.WriteString("</ul></div>")
	_, _ = w.Write([]byte(b.String()))
}
```

Add the imports `"strings"` and `"html/template"` to `handlers_settings.go` (the import block currently has only `"net/http"` and the `rules` package).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/webapi/ -run TestSaveSettingsHandler -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add app/webapi/handlers_settings.go app/webapi/handlers_settings_test.go
git commit -m "feat(webapi): add POST handler to save edited settings"
```

---

## Task 7: Register routes and link the page

**Files:**
- Modify: `app/webapi/routes.go`
- Modify: `app/webapi/assets/settings.html`
- Test: `app/webapi/routes_test.go` or `app/webapi/handlers_settings_test.go`

- [ ] **Step 1: Write the failing test**

Add to `app/webapi/handlers_settings_test.go`:

```go
func TestSettingsEditRoute_Registered(t *testing.T) {
	prov := &ruleSetProviderStub{get: rules.RuleSet{}}
	srv := &Server{Config: Config{
		RuleSetProvider: prov,
		Settings:        Settings{TenantID: "tg-spam"},
		Version:         "test",
	}}
	router := srv.routes()

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/settings/edit", http.NoBody))
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "GET /settings/edit must be routed")
}
```

Note: `routes()` is the method that builds the router (confirm its exact name by reading `app/webapi/routes.go` — adjust the call if it differs, e.g. `srv.router()`). If routing requires auth, the test may get 401 — that is still "not 404", which is what the assertion checks. If the handler needs more `Config` fields to avoid a panic, add them to the literal.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./app/webapi/ -run TestSettingsEditRoute_Registered -v`
Expected: FAIL — route returns 404.

- [ ] **Step 3: Write minimal implementation**

In `app/webapi/routes.go`, find the `webUI` route group (where `GET /list_settings` is registered with `htmlSettingsHandler`). Add two routes to the same `webUI` group so they get the same `BasicAuthWithPrompt` auth:

```go
		webUI.HandleFunc("GET /settings/edit", s.htmlSettingsEditHandler)
		webUI.HandleFunc("POST /settings/save", s.saveSettingsHandler)
```

In `app/webapi/assets/settings.html`, add a link to the editor near the top of the page body (just after the `<h2>` heading). Read the file to find the heading, then add:

```html
    <div class="text-center mb-3">
        <a href="/settings/edit" class="btn btn-primary btn-sm">Edit Settings</a>
    </div>
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./app/webapi/ -run TestSettingsEditRoute_Registered -v`
Expected: PASS.
Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add app/webapi/routes.go app/webapi/assets/settings.html app/webapi/handlers_settings_test.go
git commit -m "feat(webapi): register settings editor routes and link"
```

---

## Task 8: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Build**

Run: `go build ./...`
Expected: builds clean.

- [ ] **Step 2: Tests**

Run: `go test ./app/webapi/ ./app/ ./app/rules/ 2>&1 | tail -10`
Expected: all three packages PASS.

- [ ] **Step 3: Vet**

Run: `go vet ./app/webapi/`
Expected: clean.

- [ ] **Step 4: Lint**

Run: `golangci-lint run ./app/webapi/...`
Expected: no findings (ignore Windows CRLF gofmt noise if it appears).

- [ ] **Step 5: README**

Add a short paragraph to `README.md` under the web UI / settings section describing the new `Edit Settings` page (`/settings/edit`): it edits the detection/LLM ruleset live, and env-pinned settings show a warning badge. Keep it to 3-4 sentences. Commit:

```bash
git add README.md
git commit -m "docs: document the editable settings page"
```

---

## Self-Review notes

- **Spec coverage:** Part 1 (editable UI) — Tasks 2-7. Validation (server-side `ruleSetFromForm` + HTML5 input attributes in the template) — Tasks 2-3. Env-pinned badges — Tasks 1, 3, 5. Audit — automatic: the existing `AdminAuditLogger` middleware already covers `/settings/save` once it is a mutating route under the web group (verify in Task 8; if the audit path matcher does not include `/settings`, that is a follow-up, not part of this plan).
- **Save model:** one form, one `POST /settings/save`, source `"web"`. Per-section save was considered and dropped for simplicity.
- **Out of scope:** Project B (app-wide restyle); per-section save; live preview.
- **Type consistency:** `ruleSetFromForm`, `templateDict`, `htmlSettingsEditHandler`, `saveSettingsHandler`, `settingsError`, `EnvPinnedKeys`, `llmModeOptions`, `llmConsensusOptions` used consistently across tasks.
- **Risk:** `routes()` method name in Task 7 is assumed — the implementer must confirm it against `app/webapi/routes.go` and adjust the test call. Flagged in the task.
