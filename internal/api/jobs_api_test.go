package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func TestHandleGetJobs(t *testing.T) {
	t.Run("500 when store unavailable", func(t *testing.T) {
		rec := httptest.NewRecorder()
		(&Server{}).handleGetJobs(rec, httptest.NewRequest(http.MethodGet, "/api/jobs", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("code = %d; want 500", rec.Code)
		}
	})

	t.Run("lists and filters applications", func(t *testing.T) {
		st := openTestStore(t)
		now := time.Now().UTC()
		for _, a := range []store.Application{
			{Provider: "greenhouse", Company: "Acme", Role: "Backend Engineer", URL: "https://jobs.example.com/a",
				Status: store.StatusApplied, Reason: "fit", AppliedAt: now, Location: "Remote", PostedAt: now},
			{Provider: "lever", Company: "Globex", Role: "Frontend Engineer", URL: "https://jobs.example.com/b",
				Status: store.StatusQueued, Reason: "new", AppliedAt: now, PostedAt: now},
		} {
			if err := st.Insert(a); err != nil {
				t.Fatalf("insert fixture: %v", err)
			}
		}
		s := &Server{store: st}

		get := func(t *testing.T, q string) []Application {
			t.Helper()
			path := "/api/jobs"
			if q != "" {
				path += "?q=" + q
			}
			rec := httptest.NewRecorder()
			s.handleGetJobs(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("code = %d; want 200", rec.Code)
			}
			var out []Application
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("body not JSON: %v", err)
			}
			return out
		}

		if got := get(t, ""); len(got) != 2 {
			t.Errorf("all jobs = %d; want 2", len(got))
		}
		if got := get(t, "Acme"); len(got) != 1 || got[0].Company != "Acme" {
			t.Errorf("Acme jobs = %+v; want just Acme", got)
		}
		if got := get(t, "Backend"); len(got) != 1 || got[0].Role != "Backend Engineer" {
			t.Errorf("Backend jobs = %+v; want the backend role", got)
		}
		if got := get(t, "Remote"); len(got) != 1 {
			t.Errorf("Remote jobs = %d; want 1 (location filter)", len(got))
		}
		if got := get(t, "queued"); len(got) != 1 {
			t.Errorf("queued jobs = %d; want 1 (status filter)", len(got))
		}
		if got := get(t, "no-match"); len(got) != 0 {
			t.Errorf("no-match jobs = %d; want 0", len(got))
		}
	})
}

func TestHandlePatchJobOutcome(t *testing.T) {
	t.Run("errors on missing store/bad input", func(t *testing.T) {
		tests := []struct {
			name string
			s    *Server
			id   string
			body string
			want int
		}{
			{name: "no store", s: &Server{}, id: "1", body: `{"outcome":"rejected"}`, want: 500},
			{name: "non-numeric id", s: &Server{store: openTestStore(t)}, id: "abc", body: `{"outcome":"rejected"}`, want: 400},
			{name: "malformed json", s: &Server{store: openTestStore(t)}, id: "1", body: "{", want: 400},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPatch, "/api/jobs/"+tt.id+"/outcome",
					strings.NewReader(tt.body))
				req.SetPathValue("id", tt.id)
				rec := httptest.NewRecorder()
				tt.s.handlePatchJobOutcome(rec, req)
				if rec.Code != tt.want {
					t.Errorf("code = %d; want %d", rec.Code, tt.want)
				}
			})
		}
	})

	t.Run("records the outcome", func(t *testing.T) {
		st := openTestStore(t)
		if err := st.Insert(store.Application{
			Provider: "greenhouse", Company: "Acme", Role: "Backend Engineer",
			URL: "https://jobs.example.com/x", Status: store.StatusApplied,
			AppliedAt: time.Now().UTC(), PostedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
		s := &Server{store: st}

		req := httptest.NewRequest(http.MethodPatch, "/api/jobs/1/outcome",
			strings.NewReader(`{"outcome":"rejected"}`))
		req.SetPathValue("id", "1")
		rec := httptest.NewRecorder()
		s.handlePatchJobOutcome(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d; want 200", rec.Code)
		}

		apps, err := st.List()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if apps[0].Outcome != store.Outcome("rejected") {
			t.Errorf("outcome = %q; want rejected", apps[0].Outcome)
		}
	})
}

func TestCompanyKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "Acme Inc", want: "acme-inc"},
		{in: "AT&T", want: "at-t"},
		{in: "  Google ", want: "google"},
		{in: "", want: ""},
		{in: "Data&AI Labs, LLC", want: "data-ai-labs--llc"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := companyKey(tt.in); got != tt.want {
				t.Errorf("companyKey(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}
