package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/manthan8219/nexus-job-assistant/internal/auth"
	"github.com/manthan8219/nexus-job-assistant/internal/userstore"
)

// signUserToken issues an auth token for an arbitrary subject + email, so the
// tenant tests can act as two distinct users against one server.
func signUserToken(t *testing.T, secret, sub, email string) string {
	t.Helper()
	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			Issuer:    "https://abc.supabase.co/auth/v1",
			Audience:  jwt.ClaimStrings{auth.DefaultAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Email: email,
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testAuthSecret))
	if err != nil {
		t.Fatalf("sign user token: %v", err)
	}
	return raw
}

// TestTenantIsolation proves the core multi-tenancy guarantee end-to-end: two
// authenticated users share one server but each reads/writes their own island
// (config, jobs, analytics), and neither can see the other's data.
func TestTenantIsolation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NEXUS_HOME", root)
	t.Setenv("NEXUS_ADMIN_EMAILS", "")

	s := &Server{
		auth:  auth.New(testAuthSecret, "https://abc.supabase.co/auth/v1", ""),
		users: userstore.NewRegistry(filepath.Join(root, "users"), nil, 0),
	}
	defer s.users.Close()

	mux := http.NewServeMux()
	s.registerRoutes(mux)
	h := s.withAuth(mux)

	tokenA := signUserToken(t, testAuthSecret, "user-a", "alice@example.com")
	tokenB := signUserToken(t, testAuthSecret, "user-b", "bob@example.com")

	request := func(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	getConfig := func(t *testing.T, token string) NexusConfig {
		t.Helper()
		rec := request(t, http.MethodGet, "/api/config", "", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/config = %d: %s", rec.Code, rec.Body.String())
		}
		var nc NexusConfig
		if err := json.Unmarshal(rec.Body.Bytes(), &nc); err != nil {
			t.Fatalf("config body not JSON: %v", err)
		}
		return nc
	}
	countJobs := func(t *testing.T, token string) int {
		t.Helper()
		rec := request(t, http.MethodGet, "/api/jobs", "", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/jobs = %d: %s", rec.Code, rec.Body.String())
		}
		var out []Application
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("jobs body not JSON: %v", err)
		}
		return len(out)
	}

	// Alice saves her profile.
	if rec := request(t, http.MethodPut, "/api/config",
		`{"firstName":"Alice","lastName":"A","email":"alice@example.com","targetJobTitles":"Engineer"}`, tokenA); rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/config (A) = %d: %s", rec.Code, rec.Body.String())
	}

	// Alice sees her own profile; Bob sees an empty one.
	if got := getConfig(t, tokenA).FirstName; got != "Alice" {
		t.Errorf("alice first name = %q; want Alice", got)
	}
	if got := getConfig(t, tokenB).FirstName; got != "" {
		t.Errorf("bob first name = %q; want empty (isolated)", got)
	}

	// Alice adds a manual job; Bob's queue stays empty.
	if rec := request(t, http.MethodPost, "/api/jobs",
		`{"role":"Engineer","company":"Acme","url":"https://jobs.example.com/a1"}`, tokenA); rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/jobs (A) = %d: %s", rec.Code, rec.Body.String())
	}
	if got, want := countJobs(t, tokenA), 1; got != want {
		t.Errorf("alice jobs = %d; want %d", got, want)
	}
	if got, want := countJobs(t, tokenB), 0; got != want {
		t.Errorf("bob jobs = %d; want %d (must never see alice's job)", got, want)
	}

	// Disk layout matches: only Alice's island has a config.
	if _, err := os.Stat(filepath.Join(root, "users", "user-a", "config.json")); err != nil {
		t.Errorf("alice island config missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "users", "user-b", "config.json")); !os.IsNotExist(err) {
		t.Errorf("bob island config exists; want untouched: %v", err)
	}
}

func TestAdminEmails(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want []string
	}{
		{name: "empty", env: "", want: nil},
		{name: "single", env: "ada@example.com", want: []string{"ada@example.com"}},
		{name: "trims and drops empties", env: " Ada@Example.com , ,bob@x.com ", want: []string{"Ada@Example.com", "bob@x.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NEXUS_ADMIN_EMAILS", tt.env)
			got := adminEmails()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("adminEmails() = %v; want %v", got, tt.want)
			}
		})
	}
}
