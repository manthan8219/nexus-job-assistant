package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/manthan8219/nexus-job-assistant/internal/auth"
)

const testAuthSecret = "test-secret-that-is-long-enough"

func signAuthTokenWith(t *testing.T, secret string, life time.Duration) string {
	t.Helper()
	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "https://abc.supabase.co/auth/v1",
			Audience:  jwt.ClaimStrings{auth.DefaultAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(life)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Email: "ada@example.com",
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func signAuthToken(t *testing.T, life time.Duration) string {
	return signAuthTokenWith(t, testAuthSecret, life)
}

func newAuthedServer() *Server {
	return &Server{auth: auth.New(testAuthSecret, "https://abc.supabase.co/auth/v1", "")}
}

// pipeThrough returns a handler that records the user attached to the request
// context and answers 204, so middleware tests observe both the gate and the
// identity the gate handed to the downstream handler.
func pipeThrough(seen *auth.User) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := auth.FromContext(r.Context()); ok {
			*seen = u
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func TestWithAuthDisabledAllowsAll(t *testing.T) {
	var seen auth.User
	s := &Server{} // auth nil → disabled
	h := s.withAuth(pipeThrough(&seen))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("disabled auth: code = %d; want 204 (pass-through)", rec.Code)
	}
	if seen != (auth.User{}) {
		t.Errorf("disabled auth attached user %+v; want none", seen)
	}
}

func TestWithAuthGate(t *testing.T) {
	s := newAuthedServer()
	tests := []struct {
		name    string
		path    string
		header  string
		want    int
		wantSub string
	}{
		{name: "missing token", path: "/api/config", want: http.StatusUnauthorized},
		{name: "garbage token", path: "/api/config", header: "Bearer garbage", want: http.StatusUnauthorized},
		{name: "wrong secret", path: "/api/config", header: "Bearer " + signAuthTokenWith(t, "a-different-secret", time.Hour), want: http.StatusUnauthorized},
		{name: "expired token", path: "/api/config", header: "Bearer " + signAuthToken(t, -time.Hour), want: http.StatusUnauthorized},
		{name: "basic scheme ignored", path: "/api/config", header: "Basic ZGVtbw==", want: http.StatusUnauthorized},
		{name: "valid token admitted", path: "/api/config", header: "Bearer " + signAuthToken(t, time.Hour), want: http.StatusNoContent, wantSub: "user-123"},
		{name: "health is public", path: "/health", want: http.StatusNoContent},
		{name: "auth status is public", path: "/api/auth/status", want: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen auth.User
			h := s.withAuth(pipeThrough(&seen))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			h.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("%s: code = %d; want %d", tt.path, rec.Code, tt.want)
			}
			if tt.wantSub != "" && seen.ID != tt.wantSub {
				t.Errorf("%s: attached user ID = %q; want %q", tt.path, seen.ID, tt.wantSub)
			}
		})
	}
}

func TestWithAuthQueryTokenRestrictedToSseAndPdf(t *testing.T) {
	s := newAuthedServer()
	tok := signAuthToken(t, time.Hour)

	t.Run("mission stream accepts access_token query", func(t *testing.T) {
		var seen auth.User
		h := s.withAuth(pipeThrough(&seen))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/mission/stream?access_token="+tok, nil))
		if rec.Code != http.StatusNoContent || seen.ID != "user-123" {
			t.Errorf("code = %d, user = %q; want 204 and user-123", rec.Code, seen.ID)
		}
	})

	t.Run("pdf download accepts access_token query", func(t *testing.T) {
		h := s.withAuth(pipeThrough(&auth.User{}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/resume/templates/1/preview.pdf?access_token="+tok, nil))
		if rec.Code != http.StatusNoContent {
			t.Errorf("code = %d; want 204", rec.Code)
		}
	})

	t.Run("regular api route ignores query token", func(t *testing.T) {
		h := s.withAuth(pipeThrough(&auth.User{}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config?access_token="+tok, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("code = %d; want 401 (query fallback not honored outside SSE/pdf)", rec.Code)
		}
	})
}

func TestHandleGetAuthStatus(t *testing.T) {
	decode := func(t *testing.T, rec *httptest.ResponseRecorder) bool {
		t.Helper()
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("auth/status body not JSON: %v", err)
		}
		return body.Enabled
	}

	t.Run("disabled when no verifier", func(t *testing.T) {
		rec := httptest.NewRecorder()
		(&Server{}).handleGetAuthStatus(rec, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
		if got := decode(t, rec); got {
			t.Errorf("enabled = true; want false when no verifier")
		}
	})

	t.Run("enabled when verifier configured", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newAuthedServer().handleGetAuthStatus(rec, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
		if got := decode(t, rec); !got {
			t.Errorf("enabled = false; want true when verifier configured")
		}
	})
}

func TestHandleGetAuthMe(t *testing.T) {
	t.Run("401 without a user on context", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newAuthedServer().handleGetAuthMe(rec, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("code = %d; want 401", rec.Code)
		}
	})

	t.Run("returns the authenticated user", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req = req.WithContext(auth.WithUser(req.Context(),
			auth.User{ID: "user-123", Email: "ada@example.com", Name: "Ada Lovelace"}))
		newAuthedServer().handleGetAuthMe(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d; want 200", rec.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("auth/me body not JSON: %v", err)
		}
		if body["id"] != "user-123" || body["email"] != "ada@example.com" || body["name"] != "Ada Lovelace" {
			t.Errorf("body = %v; want id=user-123 email=ada@example.com name=Ada Lovelace", body)
		}
	})
}

func TestFullHandlerAuthComposition(t *testing.T) {
	// The real route table wrapped in the auth gate: exactly how
	// ListenAndServe serves traffic.
	s := newAuthedServer()
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	h := s.withAuth(mux)

	get := func(t *testing.T, path, header string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	t.Run("health stays public", func(t *testing.T) {
		if rec := get(t, "/health", ""); rec.Code != http.StatusOK {
			t.Errorf("health code = %d; want 200", rec.Code)
		}
	})
	t.Run("api blocked without token", func(t *testing.T) {
		if rec := get(t, "/api/mission", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("mission without token code = %d; want 401", rec.Code)
		}
	})
	t.Run("auth/me answers with valid token", func(t *testing.T) {
		rec := get(t, "/api/auth/me", "Bearer "+signAuthToken(t, time.Hour))
		if rec.Code != http.StatusOK {
			t.Fatalf("auth/me code = %d; want 200", rec.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("auth/me body not JSON: %v", err)
		}
		if body["id"] != "user-123" {
			t.Errorf("auth/me id = %q; want user-123", body["id"])
		}
	})
	t.Run("auth/status is public", func(t *testing.T) {
		if rec := get(t, "/api/auth/status", ""); rec.Code != http.StatusOK {
			t.Errorf("auth/status code = %d; want 200", rec.Code)
		}
	})
}
