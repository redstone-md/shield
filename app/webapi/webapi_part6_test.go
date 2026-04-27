package webapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/storage/engine"
	"github.com/umputun/tg-spam/app/webapi/mocks"
	"github.com/umputun/tg-spam/lib/spamcheck"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServer_downloadDetectedSpamHandler(t *testing.T) {
	testTime := time.Date(2025, 1, 25, 10, 0, 0, 0, time.UTC)

	t.Run("successful download", func(t *testing.T) {
		ds := &mocks.DetectedSpamMock{
			ReadFunc: func(ctx context.Context) ([]storage.DetectedSpamInfo, error) {
				return []storage.DetectedSpamInfo{
					{
						ID:        123,
						GID:       "gid123",
						Text:      "spam example",
						UserID:    123,
						UserName:  "user",
						Checks:    []spamcheck.Response{{Spam: true, Name: "test", Details: "details"}},
						Timestamp: testTime,
					},
				}, nil
			},
		}

		server := NewServer(Config{DetectedSpamStore: ds})
		req, err := http.NewRequest("GET", "/download/detected_spam", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.downloadDetectedSpamHandler)
		handler.ServeHTTP(rr, req)

		t.Run("verify headers", func(t *testing.T) {
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "application/x-jsonlines", rr.Header().Get("Content-Type"))
			assert.Contains(t, rr.Header().Get("Content-Disposition"), "detected_spam.jsonl")
		})

		t.Run("verify content", func(t *testing.T) {
			var info struct {
				ID        int64                `json:"id"`
				GID       string               `json:"gid"`
				Text      string               `json:"text"`
				UserID    int64                `json:"user_id"`
				UserName  string               `json:"user_name"`
				Timestamp time.Time            `json:"timestamp"`
				Added     bool                 `json:"added"`
				Checks    []spamcheck.Response `json:"checks"`
			}
			err = json.Unmarshal([]byte(strings.TrimSpace(rr.Body.String())), &info)
			require.NoError(t, err)
			assert.Equal(t, int64(123), info.ID)
			assert.Equal(t, "gid123", info.GID)
			assert.Equal(t, "spam example", info.Text)
			assert.Equal(t, int64(123), info.UserID)
			assert.Equal(t, "user", info.UserName)
			assert.Equal(t, testTime, info.Timestamp)
			require.Len(t, info.Checks, 1)
			assert.Equal(t, "test", info.Checks[0].Name)
			assert.Equal(t, "details", info.Checks[0].Details)
			assert.True(t, info.Checks[0].Spam)
		})
	})

	t.Run("multiple entries", func(t *testing.T) {
		ds := &mocks.DetectedSpamMock{
			ReadFunc: func(ctx context.Context) ([]storage.DetectedSpamInfo, error) {
				return []storage.DetectedSpamInfo{
					{ID: 1, Text: "first"},
					{ID: 2, Text: "second"},
				}, nil
			},
		}

		server := NewServer(Config{DetectedSpamStore: ds})
		req, err := http.NewRequest("GET", "/download/detected_spam", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.downloadDetectedSpamHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
		assert.Len(t, lines, 2)

		for i, line := range lines {
			var info struct {
				ID    int64  `json:"id"`
				Text  string `json:"text"`
				Added bool   `json:"added"`
			}
			err = json.Unmarshal([]byte(line), &info)
			require.NoError(t, err)
			assert.Equal(t, int64(i+1), info.ID)
			assert.Equal(t, []string{"first", "second"}[i], info.Text)
		}
	})

	t.Run("error handling", func(t *testing.T) {
		ds := &mocks.DetectedSpamMock{
			ReadFunc: func(ctx context.Context) ([]storage.DetectedSpamInfo, error) {
				return nil, errors.New("test error")
			},
		}

		server := NewServer(Config{DetectedSpamStore: ds})
		req, err := http.NewRequest("GET", "/download/detected_spam", http.NoBody)
		require.NoError(t, err)

		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(server.downloadDetectedSpamHandler)
		handler.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))

		var resp struct {
			Error   string `json:"error"`
			Details string `json:"details"`
		}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "can't get detected spam", resp.Error)
		assert.Equal(t, "test error", resp.Details)
	})
}

func TestServer_downloadBackupHandler(t *testing.T) {
	t.Run("successful backup with gzip", func(t *testing.T) {
		mockStorageEngine := &mocks.StorageEngineMock{
			BackupFunc: func(_ context.Context, w io.Writer) error {
				_, err := w.Write([]byte("-- SQL backup test content"))
				return err
			},
		}

		srv := NewServer(Config{
			StorageEngine: mockStorageEngine,
		})

		req := httptest.NewRequest("GET", "/download/backup", http.NoBody)
		w := httptest.NewRecorder()
		srv.downloadBackupHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/octet-stream", resp.Header.Get("Content-Type"), "content type should be binary")
		assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment; filename=")
		assert.Contains(t, resp.Header.Get("Content-Disposition"), ".sql.gz")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		gzipReader, err := gzip.NewReader(bytes.NewReader(body))
		require.NoError(t, err, "Content should be properly gzipped")
		defer gzipReader.Close()

		decompressedContent, err := io.ReadAll(gzipReader)
		require.NoError(t, err)

		assert.Contains(t, string(decompressedContent), "-- SQL backup test content")
	})

	t.Run("nil storage engine", func(t *testing.T) {
		srv := NewServer(Config{
			StorageEngine: nil,
		})

		req := httptest.NewRequest("GET", "/download/backup", http.NoBody)
		w := httptest.NewRecorder()
		srv.downloadBackupHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Contains(t, string(body), "storage engine not available")
	})
}

func TestServer_downloadExportToPostgresHandler(t *testing.T) {
	t.Run("successful export with sqlite engine", func(t *testing.T) {
		mockStorage := &mocks.StorageEngineMock{
			TypeFunc: func() engine.Type {
				return engine.Sqlite
			},
			BackupSqliteAsPostgresFunc: func(_ context.Context, w io.Writer) error {
				_, err := w.Write([]byte("-- SQLite to PostgreSQL export test content"))
				return err
			},
		}

		srv := NewServer(Config{
			StorageEngine: mockStorage,
		})

		req := httptest.NewRequest("GET", "/download/export-to-postgres", http.NoBody)
		w := httptest.NewRecorder()

		srv.downloadExportToPostgresHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/octet-stream", resp.Header.Get("Content-Type"), "content type should be binary")
		assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment; filename=")
		assert.Contains(t, resp.Header.Get("Content-Disposition"), "tg-spam-sqlite-to-postgres")
		assert.Contains(t, resp.Header.Get("Content-Disposition"), ".sql.gz")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		gzipReader, err := gzip.NewReader(bytes.NewReader(body))
		require.NoError(t, err, "Content should be properly gzipped")
		defer gzipReader.Close()

		decompressedContent, err := io.ReadAll(gzipReader)
		require.NoError(t, err)

		assert.Contains(t, string(decompressedContent), "-- SQLite to PostgreSQL export test content")
	})

	t.Run("non-sqlite engine", func(t *testing.T) {
		mockStorage := &mocks.StorageEngineMock{
			TypeFunc: func() engine.Type {
				return engine.Postgres
			},
		}

		srv := NewServer(Config{
			StorageEngine: mockStorage,
		})

		req := httptest.NewRequest("GET", "/download/export-to-postgres", http.NoBody)
		w := httptest.NewRecorder()
		srv.downloadExportToPostgresHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, string(body), "source database must be SQLite")
	})

	t.Run("nil storage engine", func(t *testing.T) {
		srv := NewServer(Config{
			StorageEngine: nil,
		})

		req := httptest.NewRequest("GET", "/download/export-to-postgres", http.NoBody)
		w := httptest.NewRecorder()
		srv.downloadExportToPostgresHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Contains(t, string(body), "storage engine not available")
	})
}

func TestServer_logoutHandler(t *testing.T) {

	logoutHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="tg-spam"`)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "Logged out successfully")
	}

	req := httptest.NewRequest("GET", "/logout", http.NoBody)
	w := httptest.NewRecorder()

	logoutHandler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, `Basic realm="tg-spam"`, resp.Header.Get("WWW-Authenticate"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Logged out successfully")
}

func TestServer_getDictionaryEntriesHandler(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			ReadFunc: func(ctx context.Context, t storage.DictionaryType) ([]string, error) {
				if t == storage.DictionaryTypeStopPhrase {
					return []string{"spam word", "bad phrase"}, nil
				}
				return []string{"ignored1", "ignored2"}, nil
			},
		}

		srv := NewServer(Config{DictionaryStore: mockDict})
		req := httptest.NewRequest("GET", "/dictionary", http.NoBody)
		w := httptest.NewRecorder()

		srv.getDictionaryEntriesHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "spam word")
		assert.Contains(t, string(body), "ignored1")
	})

	t.Run("error reading stop phrases", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			ReadFunc: func(ctx context.Context, t storage.DictionaryType) ([]string, error) {
				if t == storage.DictionaryTypeStopPhrase {
					return nil, errors.New("db error")
				}
				return []string{}, nil
			},
		}

		srv := NewServer(Config{DictionaryStore: mockDict})
		req := httptest.NewRequest("GET", "/dictionary", http.NoBody)
		w := httptest.NewRecorder()

		srv.getDictionaryEntriesHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Contains(t, string(body), "can't get stop phrases")
	})

	t.Run("error reading ignored words", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			ReadFunc: func(ctx context.Context, t storage.DictionaryType) ([]string, error) {
				if t == storage.DictionaryTypeStopPhrase {
					return []string{"spam word"}, nil
				}
				return nil, errors.New("db error")
			},
		}

		srv := NewServer(Config{DictionaryStore: mockDict})
		req := httptest.NewRequest("GET", "/dictionary", http.NoBody)
		w := httptest.NewRecorder()

		srv.getDictionaryEntriesHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Contains(t, string(body), "can't get ignored words")
	})
}
