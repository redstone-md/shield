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
	if err := tmpl.ExecuteTemplate(w, "feedback.html", nil); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute template: %v", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
	}
}
