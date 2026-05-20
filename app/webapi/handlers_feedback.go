package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-pkgz/rest"

	"github.com/redstone-md/shield/app/feedback"
)

func (s *Server) createLabelHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DetectedSpamID int64  `json:"detected_spam_id"`
		IncidentID     int64  `json:"incident_id"`
		Label          string `json:"label"`
		LabeledBy      string `json:"labeled_by"`
		Comment        string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": err.Error()})
		return
	}
	if req.Label == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "label is required"})
		return
	}

	entry := feedback.LabelEntry{
		DetectedSpamID: req.DetectedSpamID,
		IncidentID:     req.IncidentID,
		Label:          feedback.Label(req.Label),
		LabeledBy:      req.LabeledBy,
		Comment:        req.Comment,
	}

	created, err := s.FeedbackService.Label(r.Context(), entry)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusCreated, created)
}

func (s *Server) listLabelsHandler(w http.ResponseWriter, r *http.Request) {
	filter := feedback.LabelFilter{Limit: 100}
	if v := r.URL.Query().Get("label"); v != "" {
		filter.Label = feedback.Label(v)
	}
	if v := r.URL.Query().Get("labeled_by"); v != "" {
		filter.LabeledBy = v
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}

	labels, err := s.FeedbackService.List(r.Context(), filter)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, labels)
}

func (s *Server) labelStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := s.FeedbackService.Stats(r.Context())
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, stats)
}

func (s *Server) listCandidatesHandler(w http.ResponseWriter, r *http.Request) {
	filter := feedback.CandidateFilter{Limit: 100}
	if v := r.URL.Query().Get("status"); v != "" {
		filter.Status = feedback.CandidateStatus(v)
	}
	if v := r.URL.Query().Get("type"); v != "" {
		filter.Type = feedback.CandidateType(v)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}

	candidates, err := s.ReviewService.ListAll(r.Context(), filter)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, candidates)
}

func (s *Server) approveCandidateHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid id"})
		return
	}
	var req struct {
		Reviewer string `json:"reviewer"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := s.ReviewService.Approve(r.Context(), id, req.Reviewer); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, rest.JSON{"status": "approved"})
}

func (s *Server) rejectCandidateHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid id"})
		return
	}
	var req struct {
		Reviewer string `json:"reviewer"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := s.ReviewService.Reject(r.Context(), id, req.Reviewer); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, rest.JSON{"status": "rejected"})
}

func (s *Server) generateCandidatesHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IncidentID  int64  `json:"incident_id"`
		MessageText string `json:"message_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": err.Error()})
		return
	}
	if req.MessageText == "" {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "message_text is required"})
		return
	}

	candidates, err := s.ReviewService.GenerateFromIncident(r.Context(), req.IncidentID, req.MessageText)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, candidates)
}

func (s *Server) createKnowledgeSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	_, err := s.KnowledgeService.Snapshot(r.Context(), "tg-spam")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}

	snaps, err := s.KnowledgeService.ListSnapshots(r.Context(), 20, 0)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.ExecuteTemplate(w, "knowledge_list", snaps)
}

func (s *Server) listKnowledgeSnapshotsHandler(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	snaps, err := s.KnowledgeService.ListSnapshots(r.Context(), limit, 0)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, snaps)
}

func (s *Server) getKnowledgeSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": fmt.Sprintf("invalid snapshot id: %s", idStr)})
		return
	}

	snap, err := s.KnowledgeService.GetSnapshot(r.Context(), id)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusNotFound, rest.JSON{"error": err.Error()})
		return
	}
	_ = rest.EncodeJSON(w, http.StatusOK, snap)
}

func (s *Server) rollbackKnowledgeHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": fmt.Sprintf("invalid snapshot id: %s", idStr)})
		return
	}

	_, err = s.KnowledgeService.Rollback(r.Context(), id, "tg-spam")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}

	snaps, err := s.KnowledgeService.ListSnapshots(r.Context(), 20, 0)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.ExecuteTemplate(w, "knowledge_list", snaps)
}
