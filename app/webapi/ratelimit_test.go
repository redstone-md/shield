package webapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"golang.org/x/time/rate"
)

func TestTenantRateLimiter_AllowsWithinBurst(t *testing.T) {
	rl := NewTenantRateLimiter(rate.Limit(1), 5)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := range 5 {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("X-Tenant-ID", "tenant-a")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d should succeed", i+1)
	}
}

func TestTenantRateLimiter_BlocksOverBurst(t *testing.T) {
	rl := NewTenantRateLimiter(rate.Limit(1), 2)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("X-Tenant-ID", "tenant-a")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("X-Tenant-ID", "tenant-a")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestTenantRateLimiter_Isolation(t *testing.T) {
	rl := NewTenantRateLimiter(rate.Limit(1), 1)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	reqA := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	reqA.Header.Set("X-Tenant-ID", "tenant-a")
	recA := httptest.NewRecorder()
	handler.ServeHTTP(recA, reqA)
	assert.Equal(t, http.StatusOK, recA.Code)

	reqA2 := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	reqA2.Header.Set("X-Tenant-ID", "tenant-a")
	recA2 := httptest.NewRecorder()
	handler.ServeHTTP(recA2, reqA2)
	assert.Equal(t, http.StatusTooManyRequests, recA2.Code, "tenant-a should be rate limited")

	reqB := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	reqB.Header.Set("X-Tenant-ID", "tenant-b")
	recB := httptest.NewRecorder()
	handler.ServeHTTP(recB, reqB)
	assert.Equal(t, http.StatusOK, recB.Code, "tenant-b should not be affected by tenant-a")
}

func TestTenantRateLimiter_DefaultTenant(t *testing.T) {
	rl := NewTenantRateLimiter(rate.Limit(1), 1)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestTenantRateLimiter_NilMiddleware(t *testing.T) {
	s := &Server{}
	mw := s.tenantRateLimitMiddleware()
	assert.NotNil(t, mw)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	assert.Equal(t, http.StatusOK, rec.Code)
}
