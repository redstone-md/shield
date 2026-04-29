package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-pkgz/rest"
)

func (s *Server) onboardTenantHandler(w http.ResponseWriter, r *http.Request) {
	if s.OnboardingProvider == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "onboarding not configured"})
		return
	}

	var body struct {
		TenantID string `json:"tenant_id"`
		Name     string `json:"name"`
		OwnerID  string `json:"owner_id"`
		GID      string `json:"gid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid body", "details": err.Error()})
		return
	}

	res, err := s.OnboardingProvider.Onboard(r.Context(), OnboardRequest{
		TenantID: body.TenantID,
		Name:     body.Name,
		OwnerID:  body.OwnerID,
		GID:      body.GID,
	})
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusConflict, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusCreated, res)
}

func (s *Server) offboardTenantHandler(w http.ResponseWriter, r *http.Request) {
	if s.OnboardingProvider == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "onboarding not configured"})
		return
	}

	id := r.PathValue("id")
	if id == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "tenant id required"})
		return
	}

	if err := s.OnboardingProvider.Offboard(r.Context(), id); err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, rest.JSON{"status": "offboarded"})
}
