package webapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockTenantStatusProvider struct {
	status string
	err    error
}

func (m *mockTenantStatusProvider) Status(_ context.Context, _ string) (string, error) {
	return m.status, m.err
}

func TestTenantStatusMiddleware_Active(t *testing.T) {
	mw := &TenantStatusMiddleware{
		Checker:  &mockTenantStatusProvider{status: "active"},
		TenantID: "t1",
	}

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestTenantStatusMiddleware_Suspended(t *testing.T) {
	mw := &TenantStatusMiddleware{
		Checker:  &mockTenantStatusProvider{status: "suspended"},
		TenantID: "t1",
	}

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestTenantStatusMiddleware_Deleted(t *testing.T) {
	mw := &TenantStatusMiddleware{
		Checker:  &mockTenantStatusProvider{status: "deleted"},
		TenantID: "t1",
	}

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTenantStatusMiddleware_NilChecker(t *testing.T) {
	mw := &TenantStatusMiddleware{TenantID: "t1"}

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestTenantAuthzMiddleware_NilProvider(t *testing.T) {
	s := &Server{}
	mw := s.tenantAuthzMiddleware()
	assert.NotNil(t, mw)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}
