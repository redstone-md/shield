package webapi

import (
	"net/http"

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
