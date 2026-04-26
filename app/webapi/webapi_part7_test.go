package webapi

import (
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/tg-spam/app/storage"
	"github.com/umputun/tg-spam/app/webapi/mocks"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestServer_addDictionaryEntryHandler(t *testing.T) {
	t.Run("success json", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			AddFunc: func(ctx context.Context, t storage.DictionaryType, data string) error {
				return nil
			},
		}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return nil
			},
		}

		srv := NewServer(Config{Dictionary: mockDict, SpamFilter: mockSpamFilter})
		reqBody := `{"type": "stop_phrase", "data": "test phrase"}`
		req := httptest.NewRequest("POST", "/dictionary/add", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.addDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "test phrase")
		require.Len(t, mockDict.AddCalls(), 1)
		assert.Equal(t, storage.DictionaryTypeStopPhrase, mockDict.AddCalls()[0].T)
		assert.Equal(t, "test phrase", mockDict.AddCalls()[0].Data)
		assert.Len(t, mockSpamFilter.ReloadSamplesCalls(), 1)
	})

	t.Run("empty data", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{}
		srv := NewServer(Config{Dictionary: mockDict})
		reqBody := `{"type": "stop_phrase", "data": ""}`
		req := httptest.NewRequest("POST", "/dictionary/add", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.addDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, string(body), "data cannot be empty")
		assert.Empty(t, mockDict.AddCalls())
	})

	t.Run("invalid type", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{}
		srv := NewServer(Config{Dictionary: mockDict})
		reqBody := `{"type": "invalid_type", "data": "test"}`
		req := httptest.NewRequest("POST", "/dictionary/add", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.addDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, string(body), "invalid type")
		assert.Empty(t, mockDict.AddCalls())
	})

	t.Run("json decode error", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return nil
			},
		}

		srv := NewServer(Config{Dictionary: mockDict, SpamFilter: mockSpamFilter})
		reqBody := `{"type": "stop_phrase", "data": malformed json}`
		req := httptest.NewRequest("POST", "/dictionary/add", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.addDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, string(body), "can't decode request")
		assert.Empty(t, mockDict.AddCalls())
		assert.Empty(t, mockSpamFilter.ReloadSamplesCalls())
	})

	t.Run("error adding entry", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			AddFunc: func(ctx context.Context, t storage.DictionaryType, data string) error {
				return errors.New("database error")
			},
		}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return nil
			},
		}

		srv := NewServer(Config{Dictionary: mockDict, SpamFilter: mockSpamFilter})
		reqBody := `{"type": "stop_phrase", "data": "test phrase"}`
		req := httptest.NewRequest("POST", "/dictionary/add", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.addDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Contains(t, string(body), "can't add entry")
		assert.Len(t, mockDict.AddCalls(), 1)
		assert.Empty(t, mockSpamFilter.ReloadSamplesCalls())
	})

	t.Run("success htmx", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			AddFunc: func(ctx context.Context, t storage.DictionaryType, data string) error {
				return nil
			},
			ReadWithIDsFunc: func(ctx context.Context, t storage.DictionaryType) ([]storage.DictionaryEntry, error) {
				return []storage.DictionaryEntry{{ID: 1, Data: "test phrase"}}, nil
			},
		}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return nil
			},
		}

		srv := NewServer(Config{Dictionary: mockDict, SpamFilter: mockSpamFilter})
		form := url.Values{}
		form.Set("type", "stop_phrase")
		form.Set("data", "test phrase")
		req := httptest.NewRequest("POST", "/dictionary/add", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()

		srv.addDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "test phrase")
		require.Len(t, mockDict.AddCalls(), 1)
		assert.Equal(t, storage.DictionaryTypeStopPhrase, mockDict.AddCalls()[0].T)
		assert.Equal(t, "test phrase", mockDict.AddCalls()[0].Data)
		assert.Len(t, mockSpamFilter.ReloadSamplesCalls(), 1)
	})

	t.Run("empty data htmx", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{}
		srv := NewServer(Config{Dictionary: mockDict})
		form := url.Values{}
		form.Set("type", "stop_phrase")
		form.Set("data", "")
		req := httptest.NewRequest("POST", "/dictionary/add", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()

		srv.addDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "Data cannot be empty")
		assert.Equal(t, "#error-message", resp.Header.Get("HX-Retarget"))
		assert.Empty(t, mockDict.AddCalls())
	})

	t.Run("invalid type htmx", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{}
		srv := NewServer(Config{Dictionary: mockDict})
		form := url.Values{}
		form.Set("type", "invalid_type")
		form.Set("data", "test")
		req := httptest.NewRequest("POST", "/dictionary/add", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()

		srv.addDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "Invalid type")
		assert.Equal(t, "#error-message", resp.Header.Get("HX-Retarget"))
		assert.Empty(t, mockDict.AddCalls())
	})

	t.Run("reload error json", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			AddFunc: func(ctx context.Context, t storage.DictionaryType, data string) error {
				return nil
			},
		}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return errors.New("reload failed")
			},
		}

		srv := NewServer(Config{Dictionary: mockDict, SpamFilter: mockSpamFilter})
		reqBody := `{"type": "stop_phrase", "data": "test phrase"}`
		req := httptest.NewRequest("POST", "/dictionary/add", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.addDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.Contains(t, string(body), "entry added but reload failed")
		assert.Len(t, mockDict.AddCalls(), 1)
		assert.Len(t, mockSpamFilter.ReloadSamplesCalls(), 1)
	})

	t.Run("reload error htmx", func(t *testing.T) {
		mockDict := &mocks.DictionaryMock{
			AddFunc: func(ctx context.Context, t storage.DictionaryType, data string) error {
				return nil
			},
			ReadWithIDsFunc: func(ctx context.Context, t storage.DictionaryType) ([]storage.DictionaryEntry, error) {
				return []storage.DictionaryEntry{{ID: 1, Data: "test phrase"}}, nil
			},
		}
		mockSpamFilter := &mocks.SpamFilterMock{
			ReloadSamplesFunc: func() error {
				return errors.New("reload failed")
			},
		}

		srv := NewServer(Config{Dictionary: mockDict, SpamFilter: mockSpamFilter})
		form := url.Values{}
		form.Set("type", "stop_phrase")
		form.Set("data", "test phrase")
		req := httptest.NewRequest("POST", "/dictionary/add", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		w := httptest.NewRecorder()

		srv.addDictionaryEntryHandler(w, req)

		resp := w.Result()
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "test phrase")
		assert.Len(t, mockDict.AddCalls(), 1)
		assert.Len(t, mockSpamFilter.ReloadSamplesCalls(), 1)
	})
}
