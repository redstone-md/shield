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

type settingsRuleSetStub struct {
	get     rules.RuleSet
	updated rules.RuleSet
	source  string
	err     error
}

func (s *settingsRuleSetStub) Get(_ context.Context, _ string) (rules.RuleSet, error) {
	return s.get, s.err
}

func (s *settingsRuleSetStub) Update(_ context.Context, _, source string, rs rules.RuleSet) (rules.RuleSet, error) {
	s.source = source
	s.updated = rs
	return rs, s.err
}

func TestHTMLSettingsEditHandler_RendersForm(t *testing.T) {
	prov := &settingsRuleSetStub{get: rules.RuleSet{
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

func TestSaveSettingsHandler_ValidUpdate(t *testing.T) {
	prov := &settingsRuleSetStub{get: rules.RuleSet{WorkspaceID: "tg-spam", Version: 3}}
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
	prov := &settingsRuleSetStub{get: rules.RuleSet{WorkspaceID: "tg-spam"}}
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
