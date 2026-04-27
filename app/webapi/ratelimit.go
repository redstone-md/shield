package webapi

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

type TenantRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	burst    int
}

func NewTenantRateLimiter(r rate.Limit, burst int) *TenantRateLimiter {
	return &TenantRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		r:        r,
		burst:    burst,
	}
}

func (rl *TenantRateLimiter) getLimiter(tenantID string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if l, ok := rl.limiters[tenantID]; ok {
		return l
	}

	l := rate.NewLimiter(rl.r, rl.burst)
	rl.limiters[tenantID] = l
	return l
}

func (rl *TenantRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := tenantIDFromContext(r)
		limiter := rl.getLimiter(tenantID)

		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func tenantIDFromContext(r *http.Request) string {
	if tid := r.Header.Get("X-Tenant-ID"); tid != "" {
		return tid
	}
	return "default"
}
