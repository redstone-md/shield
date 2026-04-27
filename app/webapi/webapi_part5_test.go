package webapi

import (
	"encoding/json"
	"errors"
	"github.com/go-pkgz/routegroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/webapi/mocks"
	"github.com/umputun/tg-spam/lib/approved"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServer_StaticFiles(t *testing.T) {

	mockDetector := &mocks.DetectorMock{
		CheckFunc: func(req spamcheck.Request) (bool, []spamcheck.Response) {
			return false, []spamcheck.Response{{Details: "not spam"}}
		},
		ApprovedUsersFunc: func() []approved.UserInfo {
			return []approved.UserInfo{}
		},
	}
	mockSpamFilter := &mocks.SpamFilterMock{}
	detectedSpamMock := &mocks.DetectedSpamMock{}

	server := NewServer(Config{
		Version:      "1.0",
		Detector:     mockDetector,
		SpamFilter:   mockSpamFilter,
		DetectedSpamStore: detectedSpamMock,
	})
	ts := httptest.NewServer(server.routes(routegroup.New(http.NewServeMux())))
	defer ts.Close()

	tests := []struct {
		name        string
		path        string
		contentType string
		contains    string // for text files like CSS
	}{
		{
			name:        "styles.css",
			path:        "/styles.css",
			contentType: "text/css; charset=utf-8",
			contains:    "body",
		},
		{
			name:        "logo.png",
			path:        "/logo.png",
			contentType: "image/png",
		},
		{
			name:        "spinner.svg",
			path:        "/spinner.svg",
			contentType: "image/svg+xml",
		},
		{
			name:        "non-existent file",
			path:        "/non-existent.txt",
			contentType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(ts.URL + tt.path)
			require.NoError(t, err)
			defer resp.Body.Close()

			if tt.contentType == "" {
				assert.Equal(t, http.StatusNotFound, resp.StatusCode, "should return 404 for non-existent files")
				return
			}

			assert.Equal(t, http.StatusOK, resp.StatusCode, "should return OK")
			assert.Equal(t, tt.contentType, resp.Header.Get("Content-Type"), "should return correct content type")

			if tt.contains != "" {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Contains(t, string(body), tt.contains, "response should contain expected content")
			}
		})
	}

	t.Run("disallow access to other files", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/assets/some.html")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "should not allow access to other files")
	})
}

func TestServer_getDynamicSamplesHandler(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		mockSpamFilter := &mocks.SpamFilterMock{
			DynamicSamplesFunc: func() ([]string, []string, error) {
				return []string{"spam1", "spam2"}, []string{"ham1", "ham2"}, nil
			},
		}

		server := NewServer(Config{SpamFilter: mockSpamFilter})
		req, err := http.NewRequest("GET", "/samples", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.getDynamicSamplesHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))

		var response struct {
			Spam []string `json:"spam"`
			Ham  []string `json:"ham"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, []string{"spam1", "spam2"}, response.Spam)
		assert.Equal(t, []string{"ham1", "ham2"}, response.Ham)
	})

	t.Run("error response", func(t *testing.T) {
		mockSpamFilter := &mocks.SpamFilterMock{
			DynamicSamplesFunc: func() ([]string, []string, error) {
				return nil, nil, errors.New("test error")
			},
		}

		server := NewServer(Config{SpamFilter: mockSpamFilter})
		req, err := http.NewRequest("GET", "/samples", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.getDynamicSamplesHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))

		var response struct {
			Error   string `json:"error"`
			Details string `json:"details"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "can't get dynamic samples", response.Error)
		assert.Equal(t, "test error", response.Details)
	})
}

func Test_downloadSampleHandler(t *testing.T) {
	mockSpamFilter := &mocks.SpamFilterMock{
		DynamicSamplesFunc: func() ([]string, []string, error) {
			return []string{"spam1", "spam2"}, []string{"ham1", "ham2"}, nil
		},
	}

	server := NewServer(Config{
		SpamFilter: mockSpamFilter,
	})

	t.Run("successful spam response", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/download/spam", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := server.downloadSampleHandler(func(spam, ham []string) ([]string, string) {
			return spam, "spam.txt"
		})

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
		assert.Contains(t, rr.Header().Get("Content-Disposition"), "attachment; filename=\"spam.txt\"")
	})

	t.Run("successful ham response", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/download/ham", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := server.downloadSampleHandler(func(spam, ham []string) ([]string, string) {
			return spam, "ham.txt"
		})

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "text/plain; charset=utf-8", rr.Header().Get("Content-Type"))
		assert.Contains(t, rr.Header().Get("Content-Disposition"), "attachment; filename=\"ham.txt\"")
	})

	t.Run("error handling", func(t *testing.T) {
		mockSpamFilter.DynamicSamplesFunc = func() ([]string, []string, error) {
			return nil, nil, errors.New("test error")
		}

		req, err := http.NewRequest("GET", "/download/ham", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := server.downloadSampleHandler(func(spam, ham []string) ([]string, string) {
			return spam, "ham.txt"
		})

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)

		var response struct {
			Error   string `json:"error"`
			Details string `json:"details"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "can't get dynamic samples", response.Error)
		assert.Equal(t, "test error", response.Details)
	})
}

func TestServer_reloadDynamicSamplesHandler(t *testing.T) {
	mockSpamFilter := &mocks.SpamFilterMock{
		ReloadSamplesFunc: func() error {
			return nil
		},
	}

	server := NewServer(Config{
		SpamFilter: mockSpamFilter,
	})

	t.Run("successful reload", func(t *testing.T) {
		req, err := http.NewRequest("PUT", "/samples", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.reloadDynamicSamplesHandler)

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var response struct {
			Reloaded bool `json:"reloaded"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response.Reloaded)
	})

	t.Run("error during reload", func(t *testing.T) {
		mockSpamFilter.ReloadSamplesFunc = func() error {
			return errors.New("test error")
		}

		req, err := http.NewRequest("PUT", "/samples", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.reloadDynamicSamplesHandler)

		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusInternalServerError, rr.Code)

		var response struct {
			Error   string `json:"error"`
			Details string `json:"details"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "can't reload samples", response.Error)
		assert.Equal(t, "test error", response.Details)
	})
}

// TestServer_formatDuration tests the formatDuration function in webapi.go
func TestServer_formatDuration(t *testing.T) {
	tests := []struct {
		name string
		dur  time.Duration
		want string
	}{
		{"Minutes only", 5 * time.Minute, "5m"},
		{"Hours and minutes", 2*time.Hour + 30*time.Minute, "2h 30m"},
		{"Days, hours, minutes", 4*24*time.Hour + 2*time.Hour + 5*time.Minute, "4d 2h 5m"},
		{"Zero", 0, "0m"},
		{"Just seconds", 30 * time.Second, "0m"},
		{"Large duration", 100*24*time.Hour + 12*time.Hour + 45*time.Minute, "100d 12h 45m"},
		{"Exactly one day", 24 * time.Hour, "1d 0h 0m"},
		{"Exactly one hour", 1 * time.Hour, "1h 0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := formatDuration(tt.dur)
			assert.Equal(t, tt.want, s)
		})
	}
}

func TestServer_reverseSamples(t *testing.T) {
	tests := []struct {
		name    string
		spam    []string
		ham     []string
		revSpam []string
		revHam  []string
	}{
		{
			name:    "Empty slices",
			spam:    []string{},
			ham:     []string{},
			revSpam: []string{},
			revHam:  []string{},
		},
		{
			name:    "Single element slices",
			spam:    []string{"a"},
			ham:     []string{"1"},
			revSpam: []string{"a"},
			revHam:  []string{"1"},
		},
		{
			name:    "Multiple elements",
			spam:    []string{"a", "b", "c"},
			ham:     []string{"1", "2", "3"},
			revSpam: []string{"c", "b", "a"},
			revHam:  []string{"3", "2", "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			gotSpam, gotHam := s.reverseSamples(tt.spam, tt.ham)
			assert.Equal(t, tt.revSpam, gotSpam)
			assert.Equal(t, tt.revHam, gotHam)
		})
	}
}

func TestServer_renderSamples(t *testing.T) {
	t.Run("successful rendering", func(t *testing.T) {
		mockSpamFilter := &mocks.SpamFilterMock{
			DynamicSamplesFunc: func() ([]string, []string, error) {
				return []string{"spam1", "spam2"}, []string{"ham1", "ham2"}, nil
			},
		}

		server := NewServer(Config{SpamFilter: mockSpamFilter})
		w := httptest.NewRecorder()
		server.renderSamples(w, "samples_list")
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		t.Log(w.Body.String())
		assert.Contains(t, w.Body.String(), "Spam Samples (2)")
		assert.Contains(t, w.Body.String(), "spam1")
		assert.Contains(t, w.Body.String(), "spam2")
		assert.Contains(t, w.Body.String(), "Ham Samples (2)")
		assert.Contains(t, w.Body.String(), "ham1")
		assert.Contains(t, w.Body.String(), "ham2")
	})

	t.Run("empty samples", func(t *testing.T) {
		mockSpamFilter := &mocks.SpamFilterMock{
			DynamicSamplesFunc: func() ([]string, []string, error) {
				return []string{}, []string{}, nil
			},
		}

		server := NewServer(Config{SpamFilter: mockSpamFilter})
		w := httptest.NewRecorder()
		server.renderSamples(w, "samples_list")
		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "Spam Samples (0)")
		assert.Contains(t, body, "Ham Samples (0)")
	})

	t.Run("DynamicSamples error", func(t *testing.T) {
		mockSpamFilter := &mocks.SpamFilterMock{
			DynamicSamplesFunc: func() ([]string, []string, error) {
				return nil, nil, errors.New("sample fetch error")
			},
		}

		server := NewServer(Config{SpamFilter: mockSpamFilter})
		w := httptest.NewRecorder()
		server.renderSamples(w, "samples_list")
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

		var response map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "can't fetch samples", response["error"])
	})

	t.Run("template execution error", func(t *testing.T) {

		origTmpl := tmpl
		defer func() { tmpl = origTmpl }()

		badTemplate := template.New("bad")
		badTemplate, err := badTemplate.Parse(`{{.InvalidField}}`)
		require.NoError(t, err)
		tmpl = badTemplate

		mockSpamFilter := &mocks.SpamFilterMock{
			DynamicSamplesFunc: func() ([]string, []string, error) {
				return []string{"spam1"}, []string{"ham1"}, nil
			},
		}

		server := NewServer(Config{SpamFilter: mockSpamFilter})
		w := httptest.NewRecorder()
		server.renderSamples(w, "samples_list")
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

		var response map[string]any
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "can't execute template", response["error"])
	})
}
