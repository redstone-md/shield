package webapi

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/webapi/mocks"
	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestServer_checkHandler_HTMX(t *testing.T) {
	mockDetector := &mocks.DetectorMock{
		CheckFunc: func(req spamcheck.Request) (bool, []spamcheck.Response) {
			return req.Msg == "spam example", []spamcheck.Response{{Spam: req.Msg == "spam example", Name: "test", Details: "result details"}}
		},
		RemoveApprovedUserFunc: func(id string) error {
			return nil
		},
	}

	server := NewServer(Config{
		Detector: mockDetector,
		Version:  "1.0",
	})

	t.Run("HTMX request", func(t *testing.T) {
		form := url.Values{}
		form.Set("msg", "spam example")
		form.Set("user_id", "user123")
		req, err := http.NewRequest("POST", "/check", strings.NewReader(form.Encode()))
		require.NoError(t, err)
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("HX-Request", "true")

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.checkMsgHandler)

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "handler returned wrong status code")

		assert.Contains(t, rr.Body.String(), "strong>Result:</strong> Spam detected", "response should contain spam result")
		assert.Contains(t, rr.Body.String(), "result details")

		assert.Len(t, mockDetector.CheckCalls(), 1)
		assert.Equal(t, "spam example", mockDetector.CheckCalls()[0].Req.Msg)
		assert.Equal(t, "user123", mockDetector.CheckCalls()[0].Req.UserID)
	})
}

func TestServer_htmlSpamCheckHandler(t *testing.T) {
	t.Run("successful template render", func(t *testing.T) {
		server := NewServer(Config{Version: "1.0"})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.htmlSpamCheckHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler should return status OK")
		body := rr.Body.String()
		assert.Contains(t, body, "<title>Checker - TG-Spam</title>", "template should contain the correct title")
		assert.Contains(t, body, "Version: 1.0", "template should contain the correct version")
		assert.Contains(t, body, "<form", "template should contain a form")
	})

	t.Run("template execution error", func(t *testing.T) {

		origTmpl := tmpl
		defer func() { tmpl = origTmpl }()

		badTemplate := template.New("bad")
		badTemplate, err := badTemplate.Parse(`{{.InvalidField}}`)
		require.NoError(t, err)
		tmpl = badTemplate

		server := NewServer(Config{Version: "1.0"})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.htmlSpamCheckHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code, "should return internal server error")
		assert.Contains(t, rr.Body.String(), "Error executing template")
	})

	t.Run("with full config options", func(t *testing.T) {
		server := NewServer(Config{
			Version: "2.0-test",
			Settings: Settings{
				PrimaryGroup:        "test-group",
				AdminGroup:          "admin-group",
				SimilarityThreshold: 0.75,
				MinMsgLen:           100,
				ParanoidMode:        true,
			},
		})

		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.htmlSpamCheckHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler should return status OK")
		body := rr.Body.String()
		assert.Contains(t, body, "Version: 2.0-test", "should contain correct version")
	})
}

func TestServer_htmlManageSamplesHandler(t *testing.T) {
	spamFilterMock := &mocks.SpamFilterMock{
		DynamicSamplesFunc: func(ctx context.Context) ([]string, []string, error) {
			return []string{"spam1", "spam2"}, []string{"ham1", "ham2"}, nil
		},
	}

	server := NewServer(Config{Version: "1.0", SpamFilter: spamFilterMock})
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/manage_samples", http.NoBody)
	require.NoError(t, err)

	handler := http.HandlerFunc(server.htmlManageSamplesHandler)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "handler should return status OK")
	body := rr.Body.String()
	assert.Contains(t, body, "<title>Manage Samples - TG-Spam</title>", "template should contain the correct title")
	assert.Contains(t, body, `<div class="row" id="samples-list">`, "template should contain a samples list")
}

func TestServer_htmlManageUsersHandler(t *testing.T) {
	t.Run("successful rendering", func(t *testing.T) {
		detectorMock := &mocks.DetectorMock{
			ApprovedUsersFunc: func() []approved.UserInfo {
				return []approved.UserInfo{
					{UserID: "user1", UserName: "User One", Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
					{UserID: "user2", UserName: "User Two", Timestamp: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
				}
			},
		}

		server := NewServer(Config{Version: "1.0", Detector: detectorMock})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/manage_users", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.htmlManageUsersHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler should return status OK")
		body := rr.Body.String()
		assert.Contains(t, body, "<title>Manage Users - TG-Spam</title>", "template should contain the correct title")
		assert.Contains(t, body, "<h4>Approved Users (2)</h4>", "template should contain users list with correct count")
		assert.Contains(t, body, "User One", "should contain first user's name")
		assert.Contains(t, body, "User Two", "should contain second user's name")
		assert.Contains(t, body, "user1", "should contain first user's ID")
		assert.Contains(t, body, "user2", "should contain second user's ID")
	})

	t.Run("empty approved users list", func(t *testing.T) {
		detectorMock := &mocks.DetectorMock{
			ApprovedUsersFunc: func() []approved.UserInfo {
				return []approved.UserInfo{}
			},
		}

		server := NewServer(Config{Version: "1.0", Detector: detectorMock})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/manage_users", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.htmlManageUsersHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		body := rr.Body.String()
		assert.Contains(t, body, "<h4>Approved Users (0)</h4>", "should show zero users")
	})

	t.Run("template execution error", func(t *testing.T) {

		origTmpl := tmpl
		defer func() { tmpl = origTmpl }()

		badTemplate := template.New("bad")
		badTemplate, err := badTemplate.Parse(`{{.InvalidField}}`)
		require.NoError(t, err)
		tmpl = badTemplate

		detectorMock := &mocks.DetectorMock{
			ApprovedUsersFunc: func() []approved.UserInfo {
				return []approved.UserInfo{{UserID: "123"}}
			},
		}

		server := NewServer(Config{Version: "1.0", Detector: detectorMock})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/manage_users", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.htmlManageUsersHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Contains(t, rr.Body.String(), "Error executing template")
	})
}

func TestServer_getSettingsHandler(t *testing.T) {
	t.Run("with lua plugins", func(t *testing.T) {
		detectorMock := &mocks.DetectorMock{
			GetLuaPluginNamesFunc: func() []string {
				return []string{"plugin1", "plugin2", "plugin3"}
			},
		}

		settings := Settings{
			TenantID:        "test",
			LuaPluginsEnabled: true,
			LuaPluginsDir:     "/path/to/plugins",
			LuaEnabledPlugins: []string{"plugin1", "plugin2"},
		}

		server := NewServer(Config{Version: "1.0", Detector: detectorMock, Settings: settings})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/settings", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.getSettingsHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))

		var respSettings Settings
		err = json.Unmarshal(rr.Body.Bytes(), &respSettings)
		require.NoError(t, err)
		assert.Equal(t, settings.TenantID, respSettings.TenantID)
		assert.Equal(t, settings.LuaPluginsEnabled, respSettings.LuaPluginsEnabled)
		assert.Equal(t, settings.LuaPluginsDir, respSettings.LuaPluginsDir)
		assert.Equal(t, settings.LuaEnabledPlugins, respSettings.LuaEnabledPlugins)
		assert.Equal(t, []string{"plugin1", "plugin2", "plugin3"}, respSettings.LuaAvailablePlugins)
		assert.Len(t, detectorMock.GetLuaPluginNamesCalls(), 1)
	})

	t.Run("with lua plugins disabled", func(t *testing.T) {
		detectorMock := &mocks.DetectorMock{
			GetLuaPluginNamesFunc: func() []string {
				return []string{}
			},
		}

		settings := Settings{
			TenantID:        "test",
			LuaPluginsEnabled: false,
		}

		server := NewServer(Config{Version: "1.0", Detector: detectorMock, Settings: settings})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/settings", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.getSettingsHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var respSettings Settings
		err = json.Unmarshal(rr.Body.Bytes(), &respSettings)
		require.NoError(t, err)
		assert.Equal(t, settings.TenantID, respSettings.TenantID)
		assert.Equal(t, settings.LuaPluginsEnabled, respSettings.LuaPluginsEnabled)
		assert.Empty(t, respSettings.LuaAvailablePlugins)
		assert.Len(t, detectorMock.GetLuaPluginNamesCalls(), 1)
	})
}

func TestServer_htmlSettingsHandler(t *testing.T) {

	t.Run("without storage engine", func(t *testing.T) {
		detectorMock := &mocks.DetectorMock{
			GetLuaPluginNamesFunc: func() []string {
				return []string{"plugin1", "plugin2", "plugin3"}
			},
		}

		server := NewServer(Config{
			Version:  "1.0",
			Detector: detectorMock,
			Settings: Settings{SuperUsers: []string{"user1", "user2"}, MinMsgLen: 150},
		})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/settings", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.htmlSettingsHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler should return status OK")
		body := rr.Body.String()
		assert.Contains(t, body, "<title>Settings - TG-Spam</title>", "template should contain the correct title")
		assert.Contains(t, body, "Database")
		assert.Contains(t, body, "Not connected", "Should show database is not connected")
		assert.Contains(t, body, "Backup")
		assert.Contains(t, body, "System Status")
		assert.Contains(t, body, "Spam Detection")
	})

	t.Run("with SQL storage engine", func(t *testing.T) {
		sqlEngine := &mocks.StorageEngineMock{}
		detectorMock := &mocks.DetectorMock{
			GetLuaPluginNamesFunc: func() []string {
				return []string{"plugin1", "plugin2", "plugin3"}
			},
		}

		server := NewServer(Config{
			Version:       "1.0",
			StorageEngine: sqlEngine,
			Detector:      detectorMock,
			Settings:      Settings{SuperUsers: []string{"user1", "user2"}, MinMsgLen: 150},
		})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/settings", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.htmlSettingsHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler should return status OK")
		body := rr.Body.String()
		assert.Contains(t, body, "<title>Settings - TG-Spam</title>", "template should contain the correct title")
		assert.Contains(t, body, "Connected", "Should show database is connected")
		assert.Len(t, detectorMock.GetLuaPluginNamesCalls(), 1, "GetLuaPluginNames should be called")
	})

	t.Run("with non-SQL storage engine", func(t *testing.T) {
		mockEngine := &mocks.StorageEngineMock{}
		detectorMock := &mocks.DetectorMock{
			GetLuaPluginNamesFunc: func() []string {
				return []string{"plugin1", "plugin2", "plugin3"}
			},
		}

		server := NewServer(Config{
			Version:       "1.0",
			StorageEngine: mockEngine,
			Detector:      detectorMock,
			Settings:      Settings{SuperUsers: []string{"user1", "user2"}, MinMsgLen: 150},
		})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/settings", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.htmlSettingsHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "handler should return status OK")
		body := rr.Body.String()
		assert.Contains(t, body, "Connected (unknown type)", "Should show connected with unknown type")
		assert.Contains(t, body, "Unknown", "Should show unknown database type")
		assert.Len(t, detectorMock.GetLuaPluginNamesCalls(), 1, "GetLuaPluginNames should be called")
	})

	t.Run("template execution error", func(t *testing.T) {

		origTmpl := tmpl
		defer func() { tmpl = origTmpl }()

		badTemplate := template.New("bad")
		badTemplate, err := badTemplate.Parse(`{{.InvalidField}}`)
		require.NoError(t, err)
		tmpl = badTemplate

		detectorMock := &mocks.DetectorMock{
			GetLuaPluginNamesFunc: func() []string {
				return []string{"plugin1", "plugin2", "plugin3"}
			},
		}

		server := NewServer(Config{
			Version:  "1.0",
			Detector: detectorMock,
		})
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/settings", http.NoBody)
		require.NoError(t, err)

		handler := http.HandlerFunc(server.htmlSettingsHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code, "should return internal server error")
		assert.Len(t, detectorMock.GetLuaPluginNamesCalls(), 1, "GetLuaPluginNames should be called")
	})
}
