package webapi

import (
	"log"
	"net/http"
	"strings"
	"time"
)

type AdminAuditLogger struct{}

func NewAdminAuditLogger() *AdminAuditLogger {
	return &AdminAuditLogger{}
}

func (a *AdminAuditLogger) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAdminPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		duration := time.Since(start)

		userID, _, _ := r.BasicAuth()
		tenantID := r.Header.Get("X-Tenant-ID")

		var sb strings.Builder
		sb.WriteString("[AUDIT] ")
		sb.WriteString("method=")
		sb.WriteString(r.Method)
		sb.WriteString(" path=")
		sb.WriteString(r.URL.Path)
		sb.WriteString(" ip=")
		sb.WriteString(r.RemoteAddr)
		sb.WriteString(" user=")
		sb.WriteString(userID)
		sb.WriteString(" status=")
		sb.WriteString(http.StatusText(sw.status))
		sb.WriteString(" duration=")
		sb.WriteString(duration.String())

		if tenantID != "" {
			sb.WriteString(" tenant=")
			sb.WriteString(tenantID)
		}

		log.Print(sb.String())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func isAdminPath(path string) bool {
	prefixes := []string{"/api/", "/rules", "/dictionary", "/users", "/update", "/delete", "/download", "/tenants"}
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
