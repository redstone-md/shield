package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-pkgz/rest"

	"github.com/umputun/tg-spam/app/audit"
	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

type replayResponse struct {
	IncidentID    int64                    `json:"incident_id"`
	OriginalText  string                   `json:"original_text"`
	DetectionSpam bool                     `json:"detection_spam"`
	Checks        []spamcheck.Response     `json:"checks"`
	ReplayAt      time.Time                `json:"replay_at"`
	ReplayResult  *audit.ReplayResult      `json:"replay_result,omitempty"`
}

func (s *Server) replayIncidentHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "incident id required"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid incident id", "details": err.Error()})
		return
	}

	incident, err := s.AuditService.GetIncident(r.Context(), id)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusNotFound, rest.JSON{"error": "incident not found", "details": err.Error()})
		return
	}

	if incident.MessageText == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "incident has no stored message text"})
		return
	}

	spam, checks := s.Detector.Check(spamcheck.Request{
		Msg:      incident.MessageText,
		UserID:   fmt.Sprintf("%d", incident.SpamUserID),
		UserName: incident.SpamUserName,
	})

	replayResult := audit.ReplayResult{
		DetectionSpam:   spam,
		PolicyAction:    "replay",
		PolicyReason:    "dry-run replay of stored message",
		ReplayTimestamp: time.Now().UTC().Format(time.RFC3339),
	}

	resp := replayResponse{
		IncidentID:    incident.ID,
		OriginalText:  incident.MessageText,
		DetectionSpam: spam,
		Checks:        checks,
		ReplayAt:      time.Now().UTC(),
		ReplayResult:  &replayResult,
	}

	if s.AppealService != nil {
		if storeErr := s.AppealService.StoreReplayResult(r.Context(), incident.ID, replayResult); storeErr != nil {
			observability.Logf(r.Context(), "[WARN] failed to store replay result for incident %d: %v", incident.ID, storeErr)
		}
	}

	payload, _ := json.Marshal(resp)
	_, _ = s.AuditService.AddRawComment(r.Context(), audit.IncidentComment{
		IncidentID: incident.ID,
		AuthorType: "system",
		AuthorID:   "replay",
		Action:     "replay",
		Payload:    string(payload),
	})

	rest.RenderJSON(w, resp)
}

func (s *Server) getIncidentHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "incident id required"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid incident id", "details": err.Error()})
		return
	}

	incident, err := s.AuditService.GetIncident(r.Context(), id)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusNotFound, rest.JSON{"error": "incident not found", "details": err.Error()})
		return
	}

	comments, _ := s.AuditService.ListComments(r.Context(), id)

	type incidentDetail struct {
		audit.Incident
		Comments []audit.IncidentComment `json:"comments"`
	}

	rest.RenderJSON(w, incidentDetail{Incident: incident, Comments: comments})
}

func (s *Server) listIncidentsHandler(w http.ResponseWriter, r *http.Request) {
	filter := audit.IncidentFilter{
		TenantID: s.Settings.TenantID,
		Limit:    50,
	}

	if v := r.URL.Query().Get("status"); v != "" {
		filter.Status = audit.IncidentStatus(v)
	}
	if v := r.URL.Query().Get("source"); v != "" {
		filter.Source = audit.IncidentSource(v)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			filter.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	incidents, err := s.AuditService.ListIncidents(r.Context(), filter)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "list incidents failed", "details": err.Error()})
		return
	}
	if incidents == nil {
		incidents = []audit.Incident{}
	}
	rest.RenderJSON(w, incidents)
}

func (s *Server) updateIncidentStatusHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "incident id required"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid incident id", "details": err.Error()})
		return
	}

	var body struct {
		Status     string `json:"status"`
		ResolvedBy string `json:"resolved_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid body", "details": err.Error()})
		return
	}

	if err := s.AuditService.UpdateIncidentStatus(r.Context(), id, audit.IncidentStatus(body.Status), body.ResolvedBy); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "update failed", "details": err.Error()})
		return
	}

	rest.RenderJSON(w, rest.JSON{"status": "updated"})
}

func (s *Server) addIncidentCommentHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	if idStr == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "incident id required"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid incident id", "details": err.Error()})
		return
	}

	var body struct {
		AuthorType string `json:"author_type"`
		AuthorID   string `json:"author_id"`
		Action     string `json:"action"`
		Payload    string `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid body", "details": err.Error()})
		return
	}

	comment, err := s.AuditService.AddComment(r.Context(), audit.IncidentComment{
		IncidentID: id,
		AuthorType: body.AuthorType,
		AuthorID:   body.AuthorID,
		Action:     body.Action,
		Payload:    body.Payload,
	})
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "add comment failed", "details": err.Error()})
		return
	}

	rest.RenderJSON(w, comment)
}

func (s *Server) listAppealsHandler(w http.ResponseWriter, r *http.Request) {
	if s.AppealService == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "appeals not configured"})
		return
	}

	status := audit.AppealNew
	if v := r.URL.Query().Get("status"); v != "" {
		status = audit.AppealStatus(v)
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	appeals, err := s.AppealService.ListByStatus(r.Context(), status, limit, 0)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "list appeals failed", "details": err.Error()})
		return
	}
	if appeals == nil {
		appeals = []audit.Appeal{}
	}
	rest.RenderJSON(w, appeals)
}

func (s *Server) resolveAppealHandler(w http.ResponseWriter, r *http.Request) {
	if s.AppealService == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "appeals not configured"})
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "appeal id required"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid appeal id", "details": err.Error()})
		return
	}

	var body struct {
		Action         string `json:"action"`
		ResolverID     string `json:"resolver_id"`
		ResolutionText string `json:"resolution_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid body", "details": err.Error()})
		return
	}

	var resolveErr error
	switch body.Action {
	case "accept":
		resolveErr = s.AppealService.Accept(r.Context(), id, body.ResolverID, body.ResolutionText)
	case "reject":
		resolveErr = s.AppealService.Reject(r.Context(), id, body.ResolverID, body.ResolutionText)
	case "escalate":
		resolveErr = s.AppealService.Escalate(r.Context(), id)
	case "triage":
		resolveErr = s.AppealService.Triage(r.Context(), id, body.ResolverID)
	default:
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid action", "details": "must be accept, reject, escalate, or triage"})
		return
	}

	if resolveErr != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "resolve failed", "details": resolveErr.Error()})
		return
	}

	rest.RenderJSON(w, rest.JSON{"status": "resolved", "action": body.Action})
}

func (s *Server) getAppealHandler(w http.ResponseWriter, r *http.Request) {
	if s.AppealService == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "appeals not configured"})
		return
	}
	idStr := r.PathValue("id")
	if idStr == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "appeal id required"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid appeal id", "details": err.Error()})
		return
	}

	ap, err := s.AppealService.GetForIncident(r.Context(), id)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusNotFound, rest.JSON{"error": "appeal not found", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, ap)
}

func (s *Server) createAppealHandler(w http.ResponseWriter, r *http.Request) {
	if s.AppealService == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "appeals not configured"})
		return
	}
	var body struct {
		IncidentID     int64  `json:"incident_id"`
		AppellantID    int64  `json:"appellant_user_id"`
		AppellantName  string `json:"appellant_name"`
		AppealText     string `json:"appeal_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid body", "details": err.Error()})
		return
	}

	ap, err := s.AppealService.Submit(r.Context(), body.IncidentID, body.AppellantID, body.AppellantName, body.AppealText)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "submit appeal failed", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, ap)
}

func (s *Server) escalateAppealHandler(w http.ResponseWriter, r *http.Request) {
	if s.AppealService == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "appeals not configured"})
		return
	}
	idStr := r.PathValue("id")
	if idStr == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "appeal id required"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid appeal id", "details": err.Error()})
		return
	}

	if err := s.AppealService.Escalate(r.Context(), id); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "escalate failed", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, rest.JSON{"status": "escalated"})
}
