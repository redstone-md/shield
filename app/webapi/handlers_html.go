package webapi

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-pkgz/rest"

	"github.com/redstone-md/shield/app/observability"
	"github.com/redstone-md/shield/app/storage"
	"github.com/redstone-md/shield/app/storage/engine"
	"github.com/redstone-md/shield/lib/approved"
)

// htmlSpamCheckHandler handles GET / request.
// It returns rendered spam_check.html template with all the components.
func (s *Server) htmlSpamCheckHandler(w http.ResponseWriter, r *http.Request) {
	tmplData := struct {
		Version string
	}{
		Version: s.Version,
	}

	if err := tmpl.ExecuteTemplate(w, "spam_check.html", tmplData); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute template: %v", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
		return
	}
}

// htmlManageSamplesHandler handles GET /manage_samples request.
// It returns rendered manage_samples.html template with all the components.
func (s *Server) htmlManageSamplesHandler(w http.ResponseWriter, r *http.Request) {
	s.renderSamples(w, r, "manage_samples.html")
}

func (s *Server) htmlManageUsersHandler(w http.ResponseWriter, r *http.Request) {
	users := s.Detector.ApprovedUsers()
	tmplData := struct {
		ApprovedUsers      []approved.UserInfo
		TotalApprovedUsers int
	}{
		ApprovedUsers:      users,
		TotalApprovedUsers: len(users),
	}
	tmplData.TotalApprovedUsers = len(tmplData.ApprovedUsers)

	if err := tmpl.ExecuteTemplate(w, "manage_users.html", tmplData); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute template: %v", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
		return
	}
}

func (s *Server) htmlManageDictionaryHandler(w http.ResponseWriter, r *http.Request) {
	s.renderDictionary(r.Context(), w, "manage_dictionary.html")
}

func (s *Server) htmlDetectedSpamHandler(w http.ResponseWriter, r *http.Request) {
	ds, err := s.detectedSpam().Read(r.Context(), s.Settings.TenantID)
	if err != nil {
		observability.Logf(r.Context(), "[ERROR] Failed to fetch detected spam: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// clean up detected spam entries
	for i, d := range ds {
		d.Text = strings.ReplaceAll(d.Text, "'", " ")
		d.Text = strings.ReplaceAll(d.Text, "\n", " ")
		d.Text = strings.ReplaceAll(d.Text, "\r", " ")
		d.Text = strings.ReplaceAll(d.Text, "\t", " ")
		d.Text = strings.ReplaceAll(d.Text, "\"", " ")
		d.Text = strings.ReplaceAll(d.Text, "\\", " ")
		ds[i] = d
	}

	// get filter from query param, default to "all"
	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "all"
	}

	// apply filtering
	var filteredDS []storage.DetectedSpamInfo
	switch filter {
	case "non-classified":
		for _, entry := range ds {
			hasClassifierHam := false
			for _, check := range entry.Checks {
				if check.Name == "classifier" && !check.Spam {
					hasClassifierHam = true
					break
				}
			}
			if hasClassifierHam {
				filteredDS = append(filteredDS, entry)
			}
		}
	case "openai":
		for _, entry := range ds {
			hasOpenAI := false
			for _, check := range entry.Checks {
				if check.Name == "openai" {
					hasOpenAI = true
					break
				}
			}
			if hasOpenAI {
				filteredDS = append(filteredDS, entry)
			}
		}
	case "gemini":
		for _, entry := range ds {
			hasGemini := false
			for _, check := range entry.Checks {
				if check.Name == "gemini" {
					hasGemini = true
					break
				}
			}
			if hasGemini {
				filteredDS = append(filteredDS, entry)
			}
		}
	default: // "all" or any other value
		filteredDS = ds
	}

	tmplData := struct {
		DetectedSpamEntries []storage.DetectedSpamInfo
		TotalDetectedSpam   int
		FilteredCount       int
		Filter              string
		OpenAIEnabled       bool
		GeminiEnabled       bool
	}{
		DetectedSpamEntries: filteredDS,
		TotalDetectedSpam:   len(ds),
		FilteredCount:       len(filteredDS),
		Filter:              filter,
		OpenAIEnabled:       s.Settings.OpenAIEnabled,
		GeminiEnabled:       s.Settings.GeminiEnabled,
	}

	// if it's an HTMX request, render both content and count display for OOB swap
	if r.Header.Get("HX-Request") == "true" {
		var buf bytes.Buffer

		// first render the content template
		if err := tmpl.ExecuteTemplate(&buf, "detected_spam_content", tmplData); err != nil {
			observability.Logf(r.Context(), "[WARN] can't execute content template: %v", err)
			http.Error(w, "Error executing template", http.StatusInternalServerError)
			return
		}

		// then append OOB swap for the count display
		countHTML := ""
		if filter != "all" {
			countHTML = fmt.Sprintf("(%d/%d)", len(filteredDS), len(ds))
		} else {
			countHTML = fmt.Sprintf("(%d)", len(ds))
		}

		buf.WriteString(`<span id="count-display" hx-swap-oob="true">` + countHTML + `</span>`)

		// write the combined response
		if _, err := buf.WriteTo(w); err != nil {
			observability.Logf(r.Context(), "[WARN] failed to write response: %v", err)
		}
		return
	}

	// full page render for normal requests
	if err := tmpl.ExecuteTemplate(w, "detected_spam.html", tmplData); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute template: %v", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
		return
	}
}

func (s *Server) htmlAddDetectedSpamHandler(w http.ResponseWriter, r *http.Request) {
	reportErr := func(err error, _ int) {
		w.Header().Set("HX-Retarget", "#error-message")
		fmt.Fprintf(w, "<div class='alert alert-danger'>%s</div>", err)
	}
	msg := r.FormValue("msg")

	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil || msg == "" {
		observability.Logf(r.Context(), "[WARN] bad request: %v", err)
		reportErr(fmt.Errorf("bad request: %v", err), http.StatusBadRequest)
		return
	}

	if err := s.SpamFilter.UpdateSpam(msg); err != nil {
		observability.Logf(r.Context(), "[WARN] failed to update spam samples: %v", err)
		reportErr(fmt.Errorf("can't update spam samples: %v", err), http.StatusInternalServerError)
		return

	}
	if err := s.detectedSpam().SetAddedToSamplesFlag(r.Context(), s.Settings.TenantID, id); err != nil {
		observability.Logf(r.Context(), "[WARN] failed to update detected spam: %v", err)
		reportErr(fmt.Errorf("can't update detected spam: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) htmlSettingsHandler(w http.ResponseWriter, r *http.Request) {
	// get database information if StorageEngine is available
	var dbInfo struct {
		DatabaseType   string `json:"database_type"`
		TenantID       string `json:"tenant_id"`
		DatabaseStatus string `json:"database_status"`
	}

	if s.StorageEngine != nil {
		// try to cast to SQL engine to get type information
		if sqlEngine, ok := s.StorageEngine.(*engine.SQL); ok {
			dbInfo.DatabaseType = string(sqlEngine.Type())
			dbInfo.TenantID = sqlEngine.TenantID()
			dbInfo.DatabaseStatus = "Connected"
		} else {
			dbInfo.DatabaseType = "Unknown"
			dbInfo.DatabaseStatus = "Connected (unknown type)"
		}
	} else {
		dbInfo.DatabaseStatus = "Not connected"
	}

	// get backup information
	backupURL := "/download/backup"
	backupFilename := fmt.Sprintf("tg-spam-backup-%s-%s.sql.gz", dbInfo.DatabaseType, time.Now().Format("20060102-150405"))

	// get system info - uptime since server start
	uptime := time.Since(startTime)

	// get the list of available Lua plugins
	s.Settings.LuaAvailablePlugins = s.Detector.GetLuaPluginNames()

	data := struct {
		Settings
		Version  string
		Database struct {
			Type     string
			TenantID string
			Status   string
		}
		Backup struct {
			URL      string
			Filename string
		}
		System struct {
			Uptime string
		}
	}{
		Settings: s.Settings,
		Version:  s.Version,
		Database: struct {
			Type     string
			TenantID string
			Status   string
		}{
			Type:     dbInfo.DatabaseType,
			TenantID: dbInfo.TenantID,
			Status:   dbInfo.DatabaseStatus,
		},
		Backup: struct {
			URL      string
			Filename string
		}{
			URL:      backupURL,
			Filename: backupFilename,
		},
		System: struct {
			Uptime string
		}{
			Uptime: formatDuration(uptime),
		},
	}

	if err := tmpl.ExecuteTemplate(w, "settings.html", data); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute template: %v", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
		return
	}
}

// getDMUsersHandler handles GET /dm-users. For HTMX requests it renders the dm_users.html partial,
// for API requests it returns JSON with the list of recent DM users.
func (s *Server) getDMUsersHandler(w http.ResponseWriter, r *http.Request) {
	if s.DMUsersProvider == nil {
		http.Error(w, "DM users provider not configured", http.StatusServiceUnavailable)
		return
	}

	users := s.DMUsersProvider.GetDMUsers()

	if r.Header.Get("HX-Request") != "true" {
		// api response — return raw timestamps, no relative time
		type dmUserJSON struct {
			UserID      int64     `json:"user_id"`
			UserName    string    `json:"user_name"`
			DisplayName string    `json:"display_name"`
			Timestamp   time.Time `json:"timestamp"`
		}
		result := make([]dmUserJSON, len(users))
		for i, u := range users {
			result[i] = dmUserJSON{
				UserID:      u.UserID,
				UserName:    u.UserName,
				DisplayName: u.DisplayName,
				Timestamp:   u.Timestamp,
			}
		}
		rest.RenderJSON(w, result)
		return
	}

	// htmx response — render partial template with relative timestamps
	type dmUserView struct {
		UserID      int64
		UserName    string
		DisplayName string
		When        string
	}
	viewUsers := make([]dmUserView, len(users))
	for i, u := range users {
		viewUsers[i] = dmUserView{
			UserID:      u.UserID,
			UserName:    u.UserName,
			DisplayName: u.DisplayName,
			When:        relativeTime(u.Timestamp),
		}
	}

	data := struct {
		Users []dmUserView
	}{Users: viewUsers}

	if err := tmpl.ExecuteTemplate(w, "dm_users.html", data); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute dm_users template: %v", err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

// relativeTime formats a timestamp as a human-readable relative time string.
// accepts an optional reference time; if omitted, uses time.Now().
func relativeTime(t time.Time, now ...time.Time) string {
	ref := time.Now()
	if len(now) > 0 {
		ref = now[0]
	}
	d := ref.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// formatDuration formats a duration in a human-readable way
func (s *Server) htmlIncidentsHandler(w http.ResponseWriter, r *http.Request) {
	if s.AuditService == nil {
		http.Error(w, "incidents not configured", http.StatusNotImplemented)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "incidents.html", nil); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute template: %v", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
	}
}

func (s *Server) htmlIncidentDetailHandler(w http.ResponseWriter, r *http.Request) {
	if s.AuditService == nil {
		http.Error(w, "incidents not configured", http.StatusNotImplemented)
		return
	}
	tmplData := struct {
		IncidentID string
	}{
		IncidentID: r.PathValue("id"),
	}
	if err := tmpl.ExecuteTemplate(w, "incident_detail.html", tmplData); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute template: %v", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
	}
}

func (s *Server) htmlAppealsHandler(w http.ResponseWriter, r *http.Request) {
	if s.AppealService == nil {
		http.Error(w, "appeals not configured", http.StatusNotImplemented)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "appeals.html", nil); err != nil {
		observability.Logf(r.Context(), "[WARN] can't execute template: %v", err)
		http.Error(w, "Error executing template", http.StatusInternalServerError)
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}

	return fmt.Sprintf("%dm", minutes)
}
