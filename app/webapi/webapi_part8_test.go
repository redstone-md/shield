package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/tg-spam/app/events"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/webapi/mocks"
	"github.com/umputun/tg-spam/lib/approved"
)

func TestServer_deleteDictionaryEntryHandler(t *testing.T) {
	t.Run("success json", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			DeleteFunc: func(ctx context.Context, id int64) error {
				return nil
			},
		}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return nil
			},
		}

		srv := NewServer(Config{DictionaryStore: mockDict, SpamFilter: mockSpamFilter})
		reqBody := `{"id": 123}`
		req := httptest.NewRequest("POST", "/dictionary/delete", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.deleteDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "123")
		assert.Len(t, mockDict.DeleteCalls(), 1)
		assert.Equal(t, int64(123), mockDict.DeleteCalls()[0].ID)
		assert.Len(t, mockSpamFilter.ReloadSamplesCalls(), 1)
	})

	t.Run("error deleting", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			DeleteFunc: func(ctx context.Context, id int64) error {
				return errors.New("not found")
			},
		}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return nil
			},
		}

		srv := NewServer(Config{DictionaryStore: mockDict, SpamFilter: mockSpamFilter})
		reqBody := `{"id": 999}`
		req := httptest.NewRequest("POST", "/dictionary/delete", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.deleteDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Contains(t, string(body), "can't delete entry")
		assert.Empty(t, mockSpamFilter.ReloadSamplesCalls())
	})

	t.Run("json decode error", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return nil
			},
		}

		srv := NewServer(Config{DictionaryStore: mockDict, SpamFilter: mockSpamFilter})
		reqBody := `{"id": malformed json}`
		req := httptest.NewRequest("POST", "/dictionary/delete", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.deleteDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, string(body), "can't decode request")
		assert.Empty(t, mockDict.DeleteCalls())
		assert.Empty(t, mockSpamFilter.ReloadSamplesCalls())
	})

	t.Run("success htmx", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			DeleteFunc: func(ctx context.Context, id int64) error {
				return nil
			},
			ReadWithIDsFunc: func(ctx context.Context, t storage.DictionaryType) ([]storage.DictionaryEntry, error) {
				return []storage.DictionaryEntry{{ID: 2, Data: "remaining phrase"}}, nil
			},
		}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return nil
			},
		}

		srv := NewServer(Config{DictionaryStore: mockDict, SpamFilter: mockSpamFilter})
		form := url.Values{}
		form.Set("id", "123")
		req := httptest.NewRequest("POST", "/dictionary/delete", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()

		srv.deleteDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "remaining phrase")
		assert.Len(t, mockDict.DeleteCalls(), 1)
		assert.Equal(t, int64(123), mockDict.DeleteCalls()[0].ID)
		assert.Len(t, mockSpamFilter.ReloadSamplesCalls(), 1)
	})

	t.Run("invalid id htmx", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{}
		srv := NewServer(Config{DictionaryStore: mockDict})
		form := url.Values{}
		form.Set("id", "not-a-number")
		req := httptest.NewRequest("POST", "/dictionary/delete", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()

		srv.deleteDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "Invalid ID")
		assert.Equal(t, "#error-message", resp.Header.Get("HX-Retarget"))
		assert.Empty(t, mockDict.DeleteCalls())
	})

	t.Run("reload error json", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			DeleteFunc: func(ctx context.Context, id int64) error {
				return nil
			},
		}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return errors.New("reload failed")
			},
		}

		srv := NewServer(Config{DictionaryStore: mockDict, SpamFilter: mockSpamFilter})
		reqBody := `{"id": 123}`
		req := httptest.NewRequest("POST", "/dictionary/delete", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.deleteDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Contains(t, string(body), "entry deleted but reload failed")
		assert.Len(t, mockDict.DeleteCalls(), 1)
		assert.Len(t, mockSpamFilter.ReloadSamplesCalls(), 1)
	})

	t.Run("reload error htmx", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			DeleteFunc: func(ctx context.Context, id int64) error {
				return nil
			},
			ReadWithIDsFunc: func(ctx context.Context, t storage.DictionaryType) ([]storage.DictionaryEntry, error) {
				return []storage.DictionaryEntry{{ID: 2, Data: "remaining phrase"}}, nil
			},
		}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return errors.New("reload failed")
			},
		}

		srv := NewServer(Config{DictionaryStore: mockDict, SpamFilter: mockSpamFilter})
		form := url.Values{}
		form.Set("id", "123")
		req := httptest.NewRequest("POST", "/dictionary/delete", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()

		srv.deleteDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "remaining phrase")
		assert.Len(t, mockDict.DeleteCalls(), 1)
		assert.Len(t, mockSpamFilter.ReloadSamplesCalls(), 1)
	})
}

func TestServer_ErrorResponseContentType(t *testing.T) {

	t.Run("check handler bad request", func(t *testing.T) {
		server := NewServer(Config{})
		req := httptest.NewRequest("POST", "/check", bytes.NewBuffer([]byte("invalid json")))
		rr := httptest.NewRecorder()

		server.checkMsgHandler(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	})

	t.Run("check id handler bad request", func(t *testing.T) {
		server := NewServer(Config{})
		req := httptest.NewRequest("GET", "/check/invalid", http.NoBody)
		req.SetPathValue("user_id", "invalid")
		rr := httptest.NewRecorder()

		server.checkIDHandler(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	})

	t.Run("check id handler internal error", func(t *testing.T) {
		mockDetectedSpam := &mocks.DetectedSpamMock{
			FindByUserIDFunc: func(_ context.Context, _ int64) (*storage.DetectedSpamInfo, error) {
				return nil, assert.AnError
			},
		}
		server := NewServer(Config{DetectedSpamStore: mockDetectedSpam})
		req := httptest.NewRequest("GET", "/check/123", http.NoBody)
		req.SetPathValue("user_id", "123")
		rr := httptest.NewRecorder()

		server.checkIDHandler(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	})

	t.Run("update sample handler bad request", func(t *testing.T) {
		mockSpamFilter := &mocks.SpamFilterMock{
			UpdateSpamFunc: func(_ string) error { return nil },
		}
		server := NewServer(Config{SpamFilter: mockSpamFilter})
		req := httptest.NewRequest("POST", "/update/spam", bytes.NewBuffer([]byte("invalid json")))
		rr := httptest.NewRecorder()

		handler := server.updateSampleHandler(mockSpamFilter.UpdateSpam)
		handler(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	})

	t.Run("update sample handler internal error", func(t *testing.T) {
		mockSpamFilter := &mocks.SpamFilterMock{
			UpdateSpamFunc: func(_ string) error { return assert.AnError },
		}
		server := NewServer(Config{SpamFilter: mockSpamFilter})
		reqBody, _ := json.Marshal(map[string]string{"msg": "test"})
		req := httptest.NewRequest("POST", "/update/spam", bytes.NewBuffer(reqBody))
		rr := httptest.NewRecorder()

		handler := server.updateSampleHandler(mockSpamFilter.UpdateSpam)
		handler(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	})

	t.Run("add dictionary entry bad request empty data", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{}
		server := NewServer(Config{DictionaryStore: mockDict})
		reqBody, _ := json.Marshal(map[string]string{"type": "stop_phrase", "data": ""})
		req := httptest.NewRequest("POST", "/dictionary/add", bytes.NewBuffer(reqBody))
		rr := httptest.NewRecorder()

		server.addDictionaryEntryHandler(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	})

	t.Run("add approved user bad request no id", func(t *testing.T) {
		mockDetector := &mocks.DetectorMock{}
		mockLocator := &mocks.LocatorMock{
			UserIDByNameFunc: func(_ context.Context, _ string) int64 { return 0 },
		}
		server := NewServer(Config{Detector: mockDetector, Locator: mockLocator})
		reqBody, _ := json.Marshal(map[string]string{"user_name": ""})
		req := httptest.NewRequest("POST", "/users/add", bytes.NewBuffer(reqBody))
		rr := httptest.NewRecorder()

		handler := server.updateApprovedUsersHandler(func(_ context.Context, ui approved.UserInfo) error {
			return mockDetector.AddApprovedUser(ui)
		})
		handler(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	})
}

func TestDMUsers_getDMUsersHandlerJSON(t *testing.T) {
	ts := time.Date(2026, 3, 31, 10, 30, 0, 0, time.UTC)
	mockProvider := &mocks.DMUsersProviderMock{
		GetDMUsersFunc: func() []events.DMUser {
			return []events.DMUser{
				{UserID: 12345678, UserName: "dkrm", DisplayName: "Dmitry K.", Timestamp: ts},
				{UserID: 87654321, UserName: "alice", DisplayName: "Alice", Timestamp: ts.Add(-15 * time.Minute)},
			}
		},
	}

	server := NewServer(Config{DMUsersProvider: mockProvider})

	t.Run("json response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/dm-users", http.NoBody)
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.getDMUsersHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Header().Get("Content-Type"), "application/json")

		var result []struct {
			UserID      int64     `json:"user_id"`
			UserName    string    `json:"user_name"`
			DisplayName string    `json:"display_name"`
			Timestamp   time.Time `json:"timestamp"`
		}
		err := json.Unmarshal(rr.Body.Bytes(), &result)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, int64(12345678), result[0].UserID)
		assert.Equal(t, "dkrm", result[0].UserName)
		assert.Equal(t, "Dmitry K.", result[0].DisplayName)
		assert.Equal(t, ts, result[0].Timestamp)
		assert.Equal(t, int64(87654321), result[1].UserID)
		assert.Len(t, mockProvider.GetDMUsersCalls(), 1)
	})

	t.Run("empty list", func(t *testing.T) {
		emptyProvider := &mocks.DMUsersProviderMock{
			GetDMUsersFunc: func() []events.DMUser { return nil },
		}
		srv := NewServer(Config{DMUsersProvider: emptyProvider})
		req := httptest.NewRequest("GET", "/dm-users", http.NoBody)
		rr := httptest.NewRecorder()
		http.HandlerFunc(srv.getDMUsersHandler).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "[]")
	})

	t.Run("nil provider returns 503", func(t *testing.T) {
		srv := NewServer(Config{})
		req := httptest.NewRequest("GET", "/dm-users", http.NoBody)
		rr := httptest.NewRecorder()
		http.HandlerFunc(srv.getDMUsersHandler).ServeHTTP(rr, req)

		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	})
}

func TestDMUsers_getDMUsersHandlerHTMX(t *testing.T) {
	mockProvider := &mocks.DMUsersProviderMock{
		GetDMUsersFunc: func() []events.DMUser {
			return []events.DMUser{
				{UserID: 12345678, UserName: "dkrm", DisplayName: "Dmitry K.", Timestamp: time.Now().Add(-2 * time.Minute)},
				{UserID: 87654321, UserName: "", DisplayName: "Alice", Timestamp: time.Now().Add(-1 * time.Hour)},
			}
		},
	}

	server := NewServer(Config{DMUsersProvider: mockProvider})
	req := httptest.NewRequest("GET", "/dm-users", http.NoBody)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	http.HandlerFunc(server.getDMUsersHandler).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "12345678")
	assert.Contains(t, body, "Dmitry K.")
	assert.Contains(t, body, "@dkrm")
	assert.Contains(t, body, "87654321")
	assert.Contains(t, body, "Alice")
	assert.Contains(t, body, "2m ago")
	assert.Contains(t, body, "1h ago")
	assert.Contains(t, body, "copyUserID")
	assert.Len(t, mockProvider.GetDMUsersCalls(), 1)
}

func TestDMUsers_getDMUsersHandlerHTMX_Empty(t *testing.T) {
	mockProvider := &mocks.DMUsersProviderMock{
		GetDMUsersFunc: func() []events.DMUser { return nil },
	}

	server := NewServer(Config{DMUsersProvider: mockProvider})
	req := httptest.NewRequest("GET", "/dm-users", http.NoBody)
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	http.HandlerFunc(server.getDMUsersHandler).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "No recent DM users")
}

func TestDMUsers_relativeTime(t *testing.T) {
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"just now", 30 * time.Second, "just now"},
		{"1 minute", 1 * time.Minute, "1m ago"},
		{"5 minutes", 5 * time.Minute, "5m ago"},
		{"1 hour", 1 * time.Hour, "1h ago"},
		{"3 hours", 3 * time.Hour, "3h ago"},
		{"1 day", 25 * time.Hour, "1d ago"},
		{"5 days", 5 * 24 * time.Hour, "5d ago"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := now.Add(-tc.d)
			assert.Equal(t, tc.want, relativeTime(ts, now))
		})
	}
}
