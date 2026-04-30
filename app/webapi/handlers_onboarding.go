package webapi

import (
	"encoding/json"
	"net/http"
	"time"

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

	start := time.Now()
	res, err := s.OnboardingProvider.Onboard(r.Context(), OnboardRequest{
		TenantID: body.TenantID,
		Name:     body.Name,
		OwnerID:  body.OwnerID,
		GID:      body.GID,
	})
	if err != nil {
		s.MetricsCollector.Inc("onboard_errors")
		_ = rest.EncodeJSON(w, http.StatusConflict, rest.JSON{"error": err.Error()})
		return
	}
	s.MetricsCollector.Inc("onboard_success")
	s.MetricsCollector.Observe("onboard_duration", time.Since(start))
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

	start := time.Now()
	if err := s.OnboardingProvider.Offboard(r.Context(), id); err != nil {
		s.MetricsCollector.Inc("offboard_errors")
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": err.Error()})
		return
	}
	s.MetricsCollector.Inc("offboard_success")
	s.MetricsCollector.Observe("offboard_duration", time.Since(start))
	_ = rest.EncodeJSON(w, http.StatusOK, rest.JSON{"status": "offboarded"})
}
