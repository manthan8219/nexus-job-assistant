package usajobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// testSearchServer serves a USAJOBS-style response with one matching remote
// posting.
func testSearchServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("User-Agent header is required by the USAJOBS API terms")
		}

		resp := searchResponse{}
		resp.SearchResult.Items = []searchResultItem{{
			Descriptor: struct {
				ID               string         `json:"PositionID"`
				Title            string         `json:"PositionTitle"`
				OrganizationName string         `json:"OrganizationName"`
				Location         string         `json:"PositionLocationDisplay"`
				URI              string         `json:"PositionURI"`
				Remuneration     []remuneration `json:"PositionRemuneration"`
			}{
				ID:               "job-1",
				Title:            "Software Engineer",
				OrganizationName: "Dept of Test",
				Location:         "Remote",
				URI:              "https://www.usajobs.gov/GetJob/ViewDetails/job-1",
				Remuneration:     []remuneration{{MinimumRange: 70000, MaximumRange: 110000}},
			},
		}}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSearch(t *testing.T) {
	srv := testSearchServer(t)
	c := &Client{http: srv.Client(), apiKey: "k", email: "ada@example.com", baseURL: srv.URL}

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{
		Titles:   []string{"Software Engineer"},
		WorkType: "Remote",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	got := jobs[0]
	if got.ID != "job-1" || got.Title != "Software Engineer" || got.Company != "Dept of Test" {
		t.Errorf("job = %+v", got)
	}
	if !got.Remote {
		t.Error("remote flag = false, want true")
	}
	if got.URL != "https://www.usajobs.gov/GetJob/ViewDetails/job-1" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestSearchFiltersBelowSalaryFloor(t *testing.T) {
	srv := testSearchServer(t)
	c := &Client{http: srv.Client(), apiKey: "k", email: "ada@example.com", baseURL: srv.URL}

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{
		Titles:    []string{"Software Engineer"},
		WorkType:  "Remote",
		MinSalary: 150000, // above the 110000 ceiling
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
	c := &Client{http: srv.Client(), apiKey: "k", email: "ada@example.com", baseURL: srv.URL}

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Engineer"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestApplyIsSkipped(t *testing.T) {
	c := &Client{http: &http.Client{}}
	res, err := c.Apply(context.Background(), provider.Job{URL: "https://www.usajobs.gov/GetJob/ViewDetails/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "skipped" {
		t.Fatalf("status = %q, want skipped", res.Status)
	}
}
