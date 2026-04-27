package webapi

import (
	"context"
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

func (s *Server) checkMsgHandler(w http.ResponseWriter, r *http.Request) {
	type CheckResultDisplay struct {
		Spam   bool
		Checks []spamcheck.Response
	}

	isHtmxRequest := r.Header.Get("HX-Request") == "true"

	req := spamcheck.Request{CheckOnly: true}
	if !isHtmxRequest {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			_ = rest.EncodeJSON(w, http.StatusBadRequest, rest.JSON{"error": "can't decode request", "details": err.Error()})
			observability.Logf(r.Context(), "[WARN] can't decode request: %v", err)
			return
		}
	} else {
		req.UserID = r.FormValue("user_id")
		req.UserName = r.FormValue("user_name")
		req.Msg = r.FormValue("msg")
	}

	spam, cr := s.Detector.Check(req)
	if !isHtmxRequest {
		rest.RenderJSON(w, rest.JSON{"spam": spam, "checks": cr})
		return
	}

	if req.Msg == "" {
		w.Header().Set("HX-Retarget", "#error-message")
		fmt.Fprintln(w, "<div class='alert alert-danger'>Valid message required.</div>")
		return
	}

	resultDisplay := CheckResultDisplay{Spam: spam, Checks: cr}
	if err := tmpl.ExecuteTemplate(w, "check_results", resultDisplay); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute result template: %v", err)
		http.Error(w, "Error rendering result", http.StatusInternalServerError)
	}
}

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
	}{Status: "ham"}

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

func (s *Server) getDynamicSamplesHandler(w http.ResponseWriter, _ *http.Request) {
	spam, ham, err := s.SpamFilter.DynamicSamples()
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't get dynamic samples", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, rest.JSON{"spam": spam, "ham": ham})
}

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
		if err := updFn(req.Msg); err != nil {
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

func (s *Server) reloadDynamicSamplesHandler(w http.ResponseWriter, _ *http.Request) {
	if err := s.SpamFilter.ReloadSamples(); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't reload samples", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, rest.JSON{"reloaded": true})
}

func (s *Server) updateApprovedUsersHandler(updFn func(ctx context.Context, ui approved.UserInfo) error) func(w http.ResponseWriter, r *http.Request) {
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

		if err := updFn(r.Context(), req); err != nil {
			_ = rest.EncodeJSON(w, http.StatusInternalServerError,
				rest.JSON{"error": "can't update approved users", "details": err.Error()})
			return
		}

		if isHtmxRequest {
			users, _ := s.approvedUsers().List(r.Context())
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

func (s *Server) removeApprovedUserAdapter(ctx context.Context, req approved.UserInfo) error {
	if err := s.approvedUsers().Remove(ctx, req.UserID); err != nil {
		return fmt.Errorf("failed to remove approved user %s: %w", req.UserID, err)
	}
	return nil
}

func (s *Server) getApprovedUsersHandler(w http.ResponseWriter, r *http.Request) {
	users, _ := s.approvedUsers().List(r.Context())
	rest.RenderJSON(w, rest.JSON{"user_ids": users})
}

func (s *Server) approvedUsers() ApprovedUsersProvider {
	if s.ApprovedUsersProvider != nil {
		return s.ApprovedUsersProvider
	}
	return detectorApprovedUsersAdapter{detector: s.Detector}
}

type detectorApprovedUsersAdapter struct {
	detector Detector
}

func (a detectorApprovedUsersAdapter) List(_ context.Context) ([]approved.UserInfo, error) {
	return a.detector.ApprovedUsers(), nil
}

func (a detectorApprovedUsersAdapter) Add(_ context.Context, user approved.UserInfo) error {
	return a.detector.AddApprovedUser(user)
}

func (a detectorApprovedUsersAdapter) Remove(_ context.Context, id string) error {
	return a.detector.RemoveApprovedUser(id)
}

func (s *Server) dictionary() DictionaryProvider {
	if s.DictionaryProvider != nil {
		return s.DictionaryProvider
	}
	return dictionaryStoreAdapter{
		store: s.DictionaryStore,
		spamFilter: s.SpamFilter,
	}
}

type dictionaryStoreAdapter struct {
	store      Dictionary
	spamFilter SpamFilter
}

func (a dictionaryStoreAdapter) Read(ctx context.Context, t storage.DictionaryType) ([]string, error) {
	return a.store.Read(ctx, t)
}

func (a dictionaryStoreAdapter) ReadWithIDs(ctx context.Context, t storage.DictionaryType) ([]storage.DictionaryEntry, error) {
	return a.store.ReadWithIDs(ctx, t)
}

func (a dictionaryStoreAdapter) Add(ctx context.Context, t storage.DictionaryType, data string) error {
	return a.store.Add(ctx, t, data)
}

func (a dictionaryStoreAdapter) Delete(ctx context.Context, id int64) error {
	return a.store.Delete(ctx, id)
}

func (a dictionaryStoreAdapter) Stats(ctx context.Context) (*storage.DictionaryStats, error) {
	return a.store.Stats(ctx)
}

func (s *Server) getSettingsHandler(w http.ResponseWriter, _ *http.Request) {
	s.Settings.LuaAvailablePlugins = s.Detector.GetLuaPluginNames()
	rest.RenderJSON(w, s.Settings)
}

func (s *Server) getRuleSetHandler(w http.ResponseWriter, r *http.Request) {
	if s.RuleSetProvider == nil {
		_ = rest.EncodeJSON(w, http.StatusNotImplemented, rest.JSON{"error": "rule set service not available"})
		return
	}
	rs, err := s.RuleSetProvider.Get(r.Context(), s.Settings.InstanceID)
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
	updated, err := s.RuleSetProvider.Update(r.Context(), s.Settings.InstanceID, "api", rs)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't update rule set", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, updated)
}

func (s *Server) getDictionaryEntriesHandler(w http.ResponseWriter, r *http.Request) {
	dict := s.dictionary()
	stopPhrases, err := dict.Read(r.Context(), storage.DictionaryTypeStopPhrase)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't get stop phrases", "details": err.Error()})
		return
	}
	ignoredWords, err := dict.Read(r.Context(), storage.DictionaryTypeIgnoredWord)
	if err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't get ignored words", "details": err.Error()})
		return
	}
	rest.RenderJSON(w, rest.JSON{"stop_phrases": stopPhrases, "ignored_words": ignoredWords})
}

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

	dict := s.dictionary()
	if err := dict.Add(r.Context(), dictType, req.Data); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't add entry", "details": err.Error()})
		return
	}

	if err := s.SpamFilter.ReloadSamples(); err != nil {
		observability.Logf(r.Context(), "[WARN] failed to reload samples after dictionary add: %v", err)
		if !isHtmxRequest {
			_ = rest.EncodeJSON(w, http.StatusInternalServerError,
				rest.JSON{"error": "entry added but reload failed", "details": err.Error()})
			return
		}
	}

	if isHtmxRequest {
		s.renderDictionary(r.Context(), w, "dictionary_list")
	} else {
		rest.RenderJSON(w, rest.JSON{"added": true, "type": req.Type, "data": req.Data})
	}
}

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

	dict := s.dictionary()
	if err := dict.Delete(r.Context(), req.ID); err != nil {
		_ = rest.EncodeJSON(w, http.StatusInternalServerError, rest.JSON{"error": "can't delete entry", "details": err.Error()})
		return
	}

	if s.DictionaryProvider == nil {
		if err := s.SpamFilter.ReloadSamples(); err != nil {
			observability.Logf(r.Context(), "[WARN] failed to reload samples after dictionary delete: %v", err)
			if !isHtmxRequest {
				_ = rest.EncodeJSON(w, http.StatusInternalServerError,
					rest.JSON{"error": "entry deleted but reload failed", "details": err.Error()})
				return
			}
		}
	}

	if isHtmxRequest {
		s.renderDictionary(r.Context(), w, "dictionary_list")
	} else {
		rest.RenderJSON(w, rest.JSON{"deleted": true, "id": req.ID})
	}
}
