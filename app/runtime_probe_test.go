package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeProbeHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	probe := newRuntimeProbe("inst", "rev")
	handler := probe.Handler(ctx)

	t.Run("health is live before ready", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		var resp probeResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Equal(t, "ok", resp.Status)
		assert.Equal(t, "inst", resp.TenantID)
		assert.Equal(t, "rev", resp.Revision)
	})

	t.Run("readiness is false until flipped", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusServiceUnavailable, rr.Code)
		var resp probeResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Equal(t, "not_ready", resp.Status)
	})

	t.Run("readiness becomes true", func(t *testing.T) {
		probe.SetReady(true)
		req := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		var resp probeResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Equal(t, "ready", resp.Status)
	})

	t.Run("shutdown drops liveness and readiness", func(t *testing.T) {
		cancel()

		healthReq := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
		healthRR := httptest.NewRecorder()
		handler.ServeHTTP(healthRR, healthReq)
		require.Equal(t, http.StatusServiceUnavailable, healthRR.Code)

		readyReq := httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody)
		readyRR := httptest.NewRecorder()
		handler.ServeHTTP(readyRR, readyReq)
		require.Equal(t, http.StatusServiceUnavailable, readyRR.Code)
	})
}
