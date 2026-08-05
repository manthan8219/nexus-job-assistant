package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// withMiddleware wraps a handler with CORS, logging, and panic recovery.
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS — only configured origins (NEXUS_ALLOWED_ORIGINS) may call the
		// API with browser credentials. When no allow-list is configured the
		// classic echo-any-origin behavior is kept for local development; a
		// production deployment must always set the list.
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !s.allowsOrigin(origin) {
				writeError(w, http.StatusForbidden, "origin not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", strings.TrimRight(origin, "/"))
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// Panic recovery
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("PANIC: %v", rec)
				writeJSON(w, http.StatusInternalServerError, apiResponse{OK: false, Error: "internal server error"})
			}
		}()

		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// allowsOrigin reports whether the request origin may call the API. The
// allow-list comes from NEXUS_ALLOWED_ORIGINS; an empty list allows any origin
// (legacy local-development behavior).
func (s *Server) allowsOrigin(origin string) bool {
	if len(s.allowedOrigins) == 0 {
		return true
	}
	o := strings.TrimRight(origin, "/")
	for _, allowed := range s.allowedOrigins {
		if strings.TrimRight(allowed, "/") == o {
			return true
		}
	}
	return false
}

// parseOrigins splits a comma-separated origin list, dropping empties.
func parseOrigins(s string) []string {
	var out []string
	for _, o := range strings.Split(s, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// apiResponse is a standard JSON envelope.
type apiResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// writeJSON marshals v and writes it as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiResponse{OK: false, Error: msg})
}

// readJSON decodes the request body into v, rejecting unknown fields.
func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
