package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type runtimeProbe struct {
	instanceID string
	revision   string
	ready      atomic.Bool
}

type probeResponse struct {
	Status     string `json:"status"`
	InstanceID string `json:"instance_id,omitempty"`
	Revision   string `json:"revision,omitempty"`
}

func newRuntimeProbe(instanceID, revision string) *runtimeProbe {
	return &runtimeProbe{
		instanceID: strings.TrimSpace(instanceID),
		revision:   strings.TrimSpace(revision),
	}
}

func (p *runtimeProbe) SetReady(ready bool) {
	p.ready.Store(ready)
}

func (p *runtimeProbe) Handler(ctx context.Context) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		status := http.StatusOK
		resp := probeResponse{Status: "ok", InstanceID: p.instanceID, Revision: p.revision}
		if ctx.Err() != nil {
			status = http.StatusServiceUnavailable
			resp.Status = "shutting_down"
		}
		writeProbeResponse(w, status, resp)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		status := http.StatusOK
		resp := probeResponse{Status: "ready", InstanceID: p.instanceID, Revision: p.revision}
		if ctx.Err() != nil || !p.ready.Load() {
			status = http.StatusServiceUnavailable
			resp.Status = "not_ready"
		}
		writeProbeResponse(w, status, resp)
	})
	return mux
}

func activateRuntimeProbe(ctx context.Context, listenAddr string, probe *runtimeProbe) error {
	if probe == nil || strings.TrimSpace(listenAddr) == "" {
		return nil
	}

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      probe.Handler(ctx),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("[WARN] failed to shutdown runtime probe server: %v", err)
		} else {
			log.Printf("[INFO] runtime probe server stopped")
		}
	}()

	go func() {
		log.Printf("[INFO] start runtime probe server on %s", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[ERROR] runtime probe server failed, %v", err)
		}
	}()

	return nil
}

func writeProbeResponse(w http.ResponseWriter, status int, resp probeResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode probe response: %v", err), http.StatusInternalServerError)
	}
}
