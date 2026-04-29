package webapi

import (
	"net/http"

	"github.com/go-pkgz/rest"
)

func (s *Server) restoreTenantHandler(w http.ResponseWriter, r *http.Request) {
	if s.RestoreProvider == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "restore not configured"})
		return
	}

	id := r.PathValue("id")
	if id == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "tenant id required"})
		return
	}

	if err := s.RestoreProvider.RestoreTenant(r.Context(), id, r.Body); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, rest.JSON{"status": "restored"})
}
