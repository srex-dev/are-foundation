package gateway

import (
	"net/http"
	"strings"
)

// WrapCORSIfConfigured wraps the handler with CORS when allowedOrigins is non-empty.
// Origins must be full values as sent in the Origin header (e.g. http://localhost:5173).
func WrapCORSIfConfigured(allowedOrigins []string, next http.Handler) http.Handler {
	if len(allowedOrigins) == 0 {
		return next
	}
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(o)
		if o != "" {
			allowed[o] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return next
	}
	const allowHeaders = "Authorization, Content-Type, X-Request-ID, X-ARE-Agent-ID, Idempotency-Key"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		_, ok := allowed[origin]
		if r.Method == http.MethodOptions {
			if ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", "86400")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		next.ServeHTTP(w, r)
	})
}

// ParseCORSAllowedOrigins splits a comma-separated env value into trimmed non-empty origins.
func ParseCORSAllowedOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
