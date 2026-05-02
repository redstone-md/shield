package webapi

import (
	"net/http"
	"strings"
)

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self' https://cdn.jsdelivr.net; "+
				"img-src 'self' data:; "+
				"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; "+
				"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; "+
				"font-src 'self' https://cdn.jsdelivr.net")
		h.Del("Server")
		next.ServeHTTP(w, r)
	})
}

func CSRFTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("HX-Request") != "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-CSRF-Token") == "" {
			http.Error(w, "missing CSRF token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func SanitizeInputMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			if ct := r.Header.Get("Content-Type"); ct != "" && strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
				if err := r.ParseForm(); err != nil {
					http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
					return
				}
				for key, vals := range r.Form {
					for i, v := range vals {
						r.Form.Set(key, strings.TrimSpace(v))
						_ = i
					}
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
