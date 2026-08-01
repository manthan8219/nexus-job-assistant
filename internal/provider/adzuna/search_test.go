package adzuna

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// testSearchServer serves an Adzuna-style response with one matching job and
// one that should be filtered out by title.
func testSearchServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("app_id") == "" || r.URL.Query().Get("app_key") == "" {
			http.Error(w, "missing credentials", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("what") == "Backend Engineer" {
			json.NewEncoder(w).Encode(adzunaResult{
				Count: 1,
				Results: []adzunaJob{
					{
						ID:          "ad-1",
						Title:       "Senior Backend Engineer",
						Company:     adzunaCompany{DisplayName: "Acme"},
						Location:    adzunaLocation{Area: []string{"London", "UK"}},
						RedirectURL: "https://acme.example.com/jobs/1",
						SalaryMax:   90000,
					},
				},
			})
			return
		}
		// Any other query returns an empty page.
		json.NewEncoder(w).Encode(adzunaResult{Results: []adzunaJob{}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSearch(t *testing.T) {
	srv := testSearchServer(t)
	c := &Client{http: srv.Client(), appID: "id", appKey: "key", country: "gb", baseURL: srv.URL}

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{
		Titles:    []string{"Backend Engineer"},
		WorkType:  "Onsite",
		Locations: []string{"London"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	got := jobs[0]
	if got.ID != "ad-1" || got.Title != "Senior Backend Engineer" || got.Company != "Acme" {
		t.Errorf("job = %+v", got)
	}
	if got.URL != "https://acme.example.com/jobs/1" {
		t.Errorf("URL = %q, want the redirect URL", got.URL)
	}
	if got.Location != "London, UK" {
		t.Errorf("location = %q, want London, UK", got.Location)
	}
}

func TestSearchFiltersBySalaryFloor(t *testing.T) {
	srv := testSearchServer(t)
	c := &Client{http: srv.Client(), appID: "id", appKey: "key", country: "gb", baseURL: srv.URL}

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{
		Titles:    []string{"Backend Engineer"},
		WorkType:  "Onsite",
		Locations: []string{"London"},
		MinSalary: 150000, // above the posting's 90000 ceiling
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (below the salary floor)", len(jobs))
	}
}

func TestSearchSkipsOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), appID: "id", appKey: "key", country: "gb", baseURL: srv.URL}

	// A failing query must not abort the run — Search returns the jobs it
	// did collect (none here) without an error.
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Backend Engineer"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestApplyIsSkipped(t *testing.T) {
	c := &Client{http: &http.Client{}}
	res, err := c.Apply(context.Background(), provider.Job{URL: "https://acme.example.com/jobs/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", res.Status)
	}
}
