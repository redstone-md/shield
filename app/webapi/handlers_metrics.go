package webapi

import (
	"net/http"

	"github.com/go-pkgz/rest"
)

func (s *Server) metricsHandler(w http.ResponseWriter, _ *http.Request) {
	if s.MetricsCollector == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "metrics not configured"})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, s.MetricsCollector.Snapshot())
}
