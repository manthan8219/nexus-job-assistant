// Auth middleware and endpoints: the Bearer-token gate that protects every API
// route when an identity provider is configured, plus /api/auth/status and
// /api/auth/me consumed by the web frontend.

package api

import (
	"net/http"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/auth"
)

// isPublicAuthPath reports whether a route stays reachable without a token.
// Hosts, load balancers, and the frontend probe liveness and auth state here.
func isPublicAuthPath(path string) bool {
	return path == "/health" || path == "/api/auth/status"
}

// queryTokenPath reports whether a route may fall back to the access_token
// query parameter. EventSource (/api/mission/stream) and raw <a href>
// downloads (…/preview.pdf) cannot send the Authorization header, so they use
// the Supabase-style query fallback; every other route requires the header.
func queryTokenPath(path string) bool {
	return path == "/api/mission/stream" || strings.HasSuffix(path, ".pdf")
}

// withAuth gates the whole handler on a verified identity token. When s.auth
// is nil the request passes through untouched (auth disabled — local/legacy
// single-user deployments). When enabled, public paths bypass the gate,
// everything else yields 401 without a valid token, and the authenticated
// user is attached to the request context for downstream handlers.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			next.ServeHTTP(w, r)
			return
		}
		if isPublicAuthPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		token := auth.BearerToken(r)
		if token == "" && queryTokenPath(r.URL.Path) {
			token = r.URL.Query().Get("access_token")
		}
		u, err := s.auth.Verify(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := auth.WithUser(r.Context(), u)
		if s.users != nil {
			// Multi-tenant mode: resolve (lazily opening) the user's island so
			// every handler reads/writes that user's own data.
			st, err := s.users.For(u.ID, u.Email)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "user data unavailable")
				return
			}
			ctx = withUserState(ctx, st)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// handleGetAuthStatus reports whether auth is enabled (public; the frontend
// uses it to decide whether to show the login gate).
func (s *Server) handleGetAuthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"enabled": s.auth != nil})
}

// handleGetAuthMe returns the authenticated user's profile. The auth gate
// guarantees a verified user on the context; without one this is a 401.
func (s *Server) handleGetAuthMe(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":    u.ID,
		"email": u.Email,
		"name":  u.Name,
	})
}
