package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-pkgz/rest"

	"github.com/umputun/tg-spam/app/observability"
	"github.com/umputun/tg-spam/app/rules"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/spamcheck"
)

// checkMsgHandler handles POST /check request.
// it gets message text and user id from request body and returns spam status and check results.
func (s *Server) checkMsgHandler(w http.ResponseWriter, r *http.Request) {
	type CheckResultDisplay struct {
		Spam   bool
		Checks []spamcheck.Response
	}

	isHtmxRequest := r.Header.Get("HX-Request") == "true"

	req := spamcheck.Request{CheckOnly: true}
	if !isHtmxRequest {
		// API request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "can't decode request", "details": err.Error()})
			observability.Logf(r.Context(), "[WARN] can't decode request: %v", err)
			return
		}
	} else {
		// for hx-request (HTMX) we need to get the values from the form
		req.UserID = r.FormValue("user_id")
		req.UserName = r.FormValue("user_name")
		req.Msg = r.FormValue("msg")
	}

	spam, cr := s.Detector.Check(req)
	if !isHtmxRequest {
		// for API request return JSON
		rest.RenderJSON(w, rest.JSON{"spam": spam, "checks": cr})
		return
	}

	if req.Msg == "" {
		w.Header().Set("HX-Retarget", "#error-message")
		fmt.Fprintln(w, "<div class='alert alert-danger'>Valid message required.</div>")
		return
	}

	// render result for HTMX request
	resultDisplay := CheckResultDisplay{
		Spam:   spam,
		Checks: cr,
	}

	if err := tmpl.ExecuteTemplate(w, "check_results", resultDisplay); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute result template: %v", err)
		http.Error(w, "Error rendering result", http.StatusInternalServerError)
		return
	}
}

// checkIDHandler handles GET /check/{user_id} request.
// it returns JSON with the status "spam" or "ham" for a given user id.
// if user is spammer, it also returns check results.
func (s *Server) checkIDHandler(w http.ResponseWriter, r *http.Request) {
	type info struct {
		UserName  string              `json:"user_name,omitempty"`
		Message   string              `json:"message,omitempty"`
		Timestamp time.Time            `json:"timestamp,omitzero"`
		Checks    []spamcheck.Response `json:"checks,omitempty"`
	}
	resp := struct {
		Status string `json:"status"`
		Info   *info  `json:"info,omitempty"`
	}{
		Status: "ham",
	}

	userID, err := strconv.ParseInt(r.PathValue("user_id"), 10, 64)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "can't parse user id", "details": err.Error()})
		return
	}

	si, err := s.DetectedSpam.FindByUserID(r.Context(), userID)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't get user info", "details": err.Error()})
		return
	}
	if si != nil {
		resp.Status = "spam"
		resp.Info = &info{
			UserName:  si.UserName,
			Message:   si.Text,
			Timestamp: si.Timestamp,
			Checks:    si.Checks,
		}
	}
	rest.RenderJSON(w, resp)
}

// getDynamicSamplesHandler handles GET /samples request. It returns dynamic samples both for spam and ham.
func (s *Server) getDynamicSamplesHandler(w http.ResponseWriter, _ *http.Request) {
	spam, ham, err := s.SpamFilter.DynamicSamples()
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't get dynamic samples", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, rest.JSON{"spam": spam, "ham": ham})
}

// downloadSampleHandler handles GET /download/spam|ham request.
// It returns dynamic samples both for spam and ham.
func (s *Server) downloadSampleHandler(pickFn func(spam, ham []string) ([]string, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		spam, ham, err := s.SpamFilter.DynamicSamples()
		if err != nil {
			_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't get dynamic samples", "details": err.Error()})
			return
		}
		samples, name := pickFn(spam, ham)
		body := strings.Join(samples, "\n")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
}

// updateSampleHandler handles POST /update/spam|ham request. It updates dynamic samples both for spam and ham.
func (s *Server) updateSampleHandler(updFn func(msg string) error) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Msg string `json:"msg"`
		}

		isHtmxRequest := r.Header.Get("HX-Request") == "true"

		if isHtmxRequest {
			req.Msg = r.FormValue("msg")
		} else {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "can't decode request", "details": err.Error()})
				return
			}
		}

		err := updFn(req.Msg)
		if err != nil {
			_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't update samples", "details": err.Error()})
			return
		}

		if isHtmxRequest {
			s.renderSamples(w, "samples_list")
		} else {
			rest.RenderJSON(w, rest.JSON{"updated": true, "msg": req.Msg})
		}
	}
}

// deleteSampleHandler handles DELETE /samples request. It deletes dynamic samples both for spam and ham.
func (s *Server) deleteSampleHandler(delFn func(msg string) error) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Msg string `json:"msg"`
		}
		isHtmxRequest := r.Header.Get("HX-Request") == "true"
		if isHtmxRequest {
			req.Msg = r.FormValue("msg")
		} else {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "can't decode request", "details": err.Error()})
				return
			}
		}

		if err := delFn(req.Msg); err != nil {
			_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't delete sample", "details": err.Error()})
			return
		}

		if isHtmxRequest {
			s.renderSamples(w, "samples_list")
		} else {
			rest.RenderJSON(w, rest.JSON{"deleted": true, "msg": req.Msg, "count": 1})
		}
	}
}

// reloadDynamicSamplesHandler handles PUT /samples request. It reloads dynamic samples from db storage.
func (s *Server) reloadDynamicSamplesHandler(w http.ResponseWriter, _ *http.Request) {
	if err := s.SpamFilter.ReloadSamples(); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't reload samples", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, rest.JSON{"reloaded": true})
}

// updateApprovedUsersHandler handles POST /users/add and /users/delete requests, it adds or removes users from approved list.
func (s *Server) updateApprovedUsersHandler(updFn func(ui approved.UserInfo) error) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		req := approved.UserInfo{}
		isHtmxRequest := r.Header.Get("HX-Request") == "true"
		if isHtmxRequest {
			req.UserID = r.FormValue("user_id")
			req.UserName = r.FormValue("user_name")
		} else {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "can't decode request", "details": err.Error()})
				return
			}
		}

		// try to get userID from request and fallback to userName lookup if it's empty
		if req.UserID == "" {
			req.UserID = strconv.FormatInt(s.Locator.UserIDByName(r.Context(), req.UserName), 10)
		}

		if req.UserID == "" || req.UserID == "0" {
			if isHtmxRequest {
				w.Header().Set("HX-Retarget", "#error-message")
				fmt.Fprintln(w, "<div class='alert alert-danger'>Either userid or valid username required.</div>")
				return
			}
			_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "user ID is required"})
			return
		}

		// add or remove user from the approved list of detector
		if err := updFn(req); err != nil {
			_ = rest.EncodeJSON(w, http.StatusInternalServerError,
				rest.JSON{"error": "can't update approved users", "details": err.Error()})
			return
		}

		if isHtmxRequest {
			users := s.Detector.ApprovedUsers()
			tmplData := struct {
				ApprovedUsers      []approved.UserInfo
				TotalApprovedUsers int
			}{
				ApprovedUsers:      users,
				TotalApprovedUsers: len(users),
			}

			if err := tmpl.ExecuteTemplate(w, "users_list", tmplData); err != nil {
				http.Error(w, "Error executing template", http.StatusInternalServerError)
				return
			}

		} else {
			rest.RenderJSON(w, rest.JSON{"updated": true, "user_id": req.UserID, "user_name": req.UserName})
		}
	}
}

// removeApprovedUser is adopter for updateApprovedUsersHandler updFn
func (s *Server) removeApprovedUser(req approved.UserInfo) error {
	if err := s.Detector.RemoveApprovedUser(req.UserID); err != nil {
		return fmt.Errorf("failed to remove approved user %s: %w", req.UserID, err)
	}
	return nil
}

// getApprovedUsersHandler handles GET /users request. It returns list of approved users.
func (s *Server) getApprovedUsersHandler(w http.ResponseWriter, _ *http.Request) {
	rest.RenderJSON(w, rest.JSON{"user_ids": s.Detector.ApprovedUsers()})
}

// getSettingsHandler returns application settings, including the list of available Lua plugins
func (s *Server) getSettingsHandler(w http.ResponseWriter, _ *http.Request) {
	s.Settings.LuaAvailablePlugins = s.Detector.GetLuaPluginNames()
	rest.RenderJSON(w, s.Settings)
}

func (s *Server) getRuleSetHandler(w http.ResponseWriter, r *http.Request) {
	if s.RuleSetProvider == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "rule set service not available"})
		return
	}
	workspaceID := s.Settings.InstanceID
	rs, err := s.RuleSetProvider.Get(r.Context(), workspaceID)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't get rule set", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, rs)
}

func (s *Server) updateRuleSetHandler(w http.ResponseWriter, r *http.Request) {
	if s.RuleSetProvider == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "rule set service not available"})
		return
	}

	var rs rules.RuleSet
	if err := json.NewDecoder(r.Body).Decode(&rs); err != nil {
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "can't decode request", "details": err.Error()})
		return
	}

	workspaceID := s.Settings.InstanceID
	updated, err := s.RuleSetProvider.Update(r.Context(), workspaceID, "api", rs)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't update rule set", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, updated)
}

// getDictionaryEntriesHandler handles GET /dictionary request. It returns stop phrases and ignored words.
func (s *Server) getDictionaryEntriesHandler(w http.ResponseWriter, r *http.Request) {
	stopPhrases, err := s.Dictionary.Read(r.Context(), storage.DictionaryTypeStopPhrase)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't get stop phrases", "details": err.Error()})
		return
	}

	ignoredWords, err := s.Dictionary.Read(r.Context(), storage.DictionaryTypeIgnoredWord)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't get ignored words", "details": err.Error()})
		return
	}

	rest.RenderJSON(w, rest.JSON{"stop_phrases": stopPhrases, "ignored_words": ignoredWords})
}

// addDictionaryEntryHandler handles POST /dictionary/add request. It adds a stop phrase or ignored word.
func (s *Server) addDictionaryEntryHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}

	isHtmxRequest := r.Header.Get("HX-Request") == "true"

	if isHtmxRequest {
		req.Type = r.FormValue("type")
		req.Data = r.FormValue("data")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "can't decode request", "details": err.Error()})
			return
		}
	}

	if req.Data == "" {
		if isHtmxRequest {
			w.Header().Set("HX-Retarget", "#error-message")
			fmt.Fprintln(w, "<div class='alert alert-danger'>Data cannot be empty.</div>")
			return
		}
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "data cannot be empty"})
		return
	}

	dictType := storage.DictionaryType(req.Type)
	if err := dictType.Validate(); err != nil {
		if isHtmxRequest {
			w.Header().Set("HX-Retarget", "#error-message")
			fmt.Fprintf(w, "<div class='alert alert-danger'>Invalid type: %v</div>", err)
			return
		}
		_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "invalid type", "details": err.Error()})
		return
	}

	if err := s.Dictionary.Add(r.Context(), dictType, req.Data); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't add entry", "details": err.Error()})
		return
	}

	// reload samples to apply dictionary changes immediately
	if err := s.SpamFilter.ReloadSamples(); err != nil {
		observability.Logf(r.Context(), "[WARN] failed to reload samples after dictionary add: %v", err)
		if !isHtmxRequest {
			_ = rest.EncodeJSON(w, http.StatusInternalServerError,
				rest.JSON{"error": "entry added but reload failed", "details": err.Error()})
			return
		}
		// for HTMX, log but continue rendering (entry was added successfully)
	}

	if isHtmxRequest {
		s.renderDictionary(r.Context(), w, "dictionary_list")
	} else {
		rest.RenderJSON(w, rest.JSON{"added": true, "type": req.Type, "data": req.Data})
	}
}

// deleteDictionaryEntryHandler handles POST /dictionary/delete request. It deletes an entry by data.
func (s *Server) deleteDictionaryEntryHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int64 `json:"id"`
	}

	isHtmxRequest := r.Header.Get("HX-Request") == "true"

	if isHtmxRequest {
		idStr := r.FormValue("id")
		var err error
		req.ID, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			w.Header().Set("HX-Retarget", "#error-message")
			fmt.Fprintf(w, "<div class='alert alert-danger'>Invalid ID: %v</div>", err)
			return
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "can't decode request", "details": err.Error()})
			return
		}
	}

	if err := s.Dictionary.Delete(r.Context(), req.ID); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't delete entry", "details": err.Error()})
		return
	}

	// reload samples to apply dictionary changes immediately
	if err := s.SpamFilter.ReloadSamples(); err != nil {
		observability.Logf(r.Context(), "[WARN] failed to reload samples after dictionary delete: %v", err)
		if !isHtmxRequest {
			_ = rest.EncodeJSON(w, http.StatusInternalServerError,
				rest.JSON{"error": "entry deleted but reload failed", "details": err.Error()})
			return
		}
		// for HTMX, log but continue rendering (entry was deleted successfully)
	}

	if isHtmxRequest {
		s.renderDictionary(r.Context(), w, "dictionary_list")
	} else {
		rest.RenderJSON(w, rest.JSON{"deleted": true, "id": req.ID})
	}
}
