package webapi

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/app/rules"
)

// llmModeOptions and llmConsensusOptions are the allowed values for the enum selects.
var (
	llmModeOptions      = []string{"", "missed", "flagged", "always"}
	llmConsensusOptions = []string{"any", "all"}
)

// htmlSettingsEditHandler renders the editable settings form, pre-filled from the current ruleset.
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
		observability.Logf(r.Context(), "[WARN] can't execute template: %v", err)
		http.Error(w, "failed to render: "+err.Error(), http.StatusInternalServerError)
	}
}

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
