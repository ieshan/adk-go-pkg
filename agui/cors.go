package agui

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CORSConfig controls CORS behavior for the AG-UI HTTP handler.
type CORSConfig struct {
	// AllowOrigins lists permitted origin URIs. Default: ["*"].
	AllowOrigins []string

	// AllowMethods lists permitted HTTP methods. Default: ["POST", "OPTIONS"].
	AllowMethods []string

	// AllowHeaders lists permitted request headers. Default: ["Content-Type", "Cache-Control"].
	AllowHeaders []string

	// ExposeHeaders lists response headers exposed to the client. Optional.
	ExposeHeaders []string

	// AllowCredentials permits cookies and credentials. Optional.
	AllowCredentials bool

	// MaxAge is the preflight cache duration. Default: 300s.
	MaxAge time.Duration
}

// CORSMiddleware returns an http.Handler middleware that sets CORS headers
// and handles OPTIONS preflight requests. If cfg is nil, defaults are applied.
func CORSMiddleware(cfg *CORSConfig) func(http.Handler) http.Handler {
	if cfg == nil {
		cfg = &CORSConfig{}
	}
	origins := cfg.AllowOrigins
	if len(origins) == 0 {
		origins = []string{"*"}
	}
	methods := cfg.AllowMethods
	if len(methods) == 0 {
		methods = []string{"POST", "OPTIONS"}
	}
	headers := cfg.AllowHeaders
	if len(headers) == 0 {
		headers = []string{"Content-Type", "Cache-Control"}
	}
	maxAge := cfg.MaxAge
	if maxAge == 0 {
		maxAge = 300 * time.Second
	}

	allowOrigin := strings.Join(origins, ", ")
	allowMethods := strings.Join(methods, ", ")
	allowHeaders := strings.Join(headers, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Access-Control-Allow-Methods", allowMethods)
			h.Set("Access-Control-Allow-Headers", allowHeaders)
			if exposeHeaders != "" {
				h.Set("Access-Control-Expose-Headers", exposeHeaders)
			}
			// W3C CORS: Access-Control-Allow-Origin cannot be "*" when
			// Allow-Credentials is true — browsers reject that combination.
			// When credentials are enabled and the configured origin is a
			// wildcard, reflect the request's Origin header instead and add
			// Vary: Origin so caches distinguish responses per origin.
			if cfg.AllowCredentials {
				h.Set("Access-Control-Allow-Credentials", "true")
				if allowOrigin == "*" {
					reqOrigin := r.Header.Get("Origin")
					if reqOrigin != "" {
						h.Set("Access-Control-Allow-Origin", reqOrigin)
						h.Set("Vary", "Origin")
					} else {
						// No Origin header: still set the wildcard so
						// non-browser clients get a permissive response.
						h.Set("Access-Control-Allow-Origin", allowOrigin)
					}
				} else {
					h.Set("Access-Control-Allow-Origin", allowOrigin)
				}
			} else {
				h.Set("Access-Control-Allow-Origin", allowOrigin)
			}
			h.Set("Access-Control-Max-Age", strconv.Itoa(int(maxAge.Seconds())))

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
