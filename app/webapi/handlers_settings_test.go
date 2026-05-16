package webapi

import (
	"context"
	"net/http"
	"net/http/httptest"
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
