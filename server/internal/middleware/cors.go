package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

// ResolveCORSOrigins parses CORS_ALLOWED_ORIGINS for the given deployment
// mode into the effective explicit origin list (spec decision 5).
//
// local mode: the designer is served same-origin, CORS is never needed —
// a configured value is ignored with a warning and nil is returned.
// cloud mode: comma-separated exact origins (scheme+host[+port]); a "*"
// entry is forbidden with credentials and downgrades to unconfigured
// (same-origin only) with a startup warning.
func ResolveCORSOrigins(raw, deploymentMode string) []string {
	raw = strings.TrimSpace(raw)
	if deploymentMode == "local" {
		if raw != "" {
			slog.Warn("CORS_ALLOWED_ORIGINS ignored in local mode (same-origin only)", "value", raw)
		}
		return nil
	}
	if raw == "" {
		return nil
	}

	var origins []string
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		if origin == "*" {
			slog.Warn("CORS_ALLOWED_ORIGINS \"*\" is not allowed with credentials — treating CORS as unconfigured (same-origin only); list explicit origins instead")
			return nil
		}
		origins = append(origins, origin)
	}
	return origins
}

// CORS answers preflights and sets CORS headers for cloud mode with an
// explicit origin list. With an empty list (always in local mode) no
// Access-Control-* header is ever set; OPTIONS is still answered with 204
// so preflights never reach the guard (they carry no cookies).
// Access-Control-Allow-Origin is only ever an exact echo of a configured
// origin — never "*" — because responses allow credentials.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowedOrigins) > 0 {
				// Responses vary by Origin whenever an origin list is active.
				w.Header().Add("Vary", "Origin")
				origin := r.Header.Get("Origin")
				if originInList(origin, allowedOrigins) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					if r.Method == http.MethodOptions {
						w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
						w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+ClientTokenHeader)
					}
				}
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originInList(origin string, list []string) bool {
	if origin == "" {
		return false
	}
	for _, allowed := range list {
		if origin == allowed {
			return true
		}
	}
	return false
}
