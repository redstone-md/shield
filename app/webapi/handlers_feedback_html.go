package webapi

import (
	"net/http"

	"github.com/umputun/tg-spam/app/observability"
)

func (s *Server) htmlFeedbackHandler(w http.ResponseWriter, r *http.Request) {
	if s.FeedbackService == nil {
		http.Error(w, "feedback not configured", http.StatusNotImplemented)
		return
	}

	tmplData := struct {
		Labels     any
		Candidates any
		Snapshots  any
	}{}

	if s.KnowledgeService != nil {
		snaps, err := s.KnowledgeService.ListSnapshots(r.Context(), 20, 0)
		if err != nil {
			observability.Logf(r.Context(), "[WARN] can't list knowledge snapshots: %v", err)
		} else {
			tmplData.Snapshots = snaps
		}
	}

	if err := tmpl.ExecuteTemplate(w, "feedback.html", tmplData); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute template: %v", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
	}
}
