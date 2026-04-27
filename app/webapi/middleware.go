package webapi

import (
	"context"
	"net/http"

	"github.com/go-pkgz/rest"
)

type TenantStatusProvider interface {
	Status(ctx context.Context, tenantID string) (string, error)
}

type TenantStatusMiddleware struct {
	Checker  TenantStatusProvider
	TenantID string
}

func (m *TenantStatusMiddleware) Handler(next http.Handler) http.Handler {
	if m.Checker == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, err := m.Checker.Status(r.Context(), m.TenantID)
		if err != nil {
			_ = rest.EncodeJSON(w, http.StatusServiceUnavailable, rest.JSON{"error": "tenant lookup failed"})
			return
		}

		switch status {
		case "active":
			next.ServeHTTP(w, r)
		case "suspended":
			_ = rest.EncodeJSON(w, http.StatusForbidden, rest.JSON{"error": "tenant suspended"})
		case "deleted":
			_ = rest.EncodeJSON(w, http.StatusNotFound, rest.JSON{"error": "tenant not found"})
		default:
			_ = rest.EncodeJSON(w, http.StatusForbidden, rest.JSON{"error": "tenant status invalid"})
		}
	})
}
