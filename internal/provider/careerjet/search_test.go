package careerjet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("affid") == "" || r.URL.Query().Get("api_key") == "" {
			http.Error(w, "missing credentials", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(careerjetResponse{
			Type: "SUCCESS",
			Jobs: []careerjetJob{
				{Title: "Senior Backend Engineer", Company: "Acme", Locations: []string{"London", "UK"}, URL: "https://acme.example.com/jobs/1"},
			},
		})
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), affid: "aff-1", apiKey: "key-1", baseURL: srv.URL}

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
	if got.Title != "Senior Backend Engineer" || got.Company != "Acme" {
		t.Errorf("job = %+v", got)
	}
	if got.Location != "London, UK" {
		t.Errorf("location = %q, want London, UK", got.Location)
	}
}

func TestSearchSkipsOnAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(careerjetResponse{Type: "ERROR"})
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), affid: "aff-1", apiKey: "key-1", baseURL: srv.URL}

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Engineer"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestSearchFiltersByLocationAndTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(careerjetResponse{
			Type: "SUCCESS",
			Jobs: []careerjetJob{
				{Title: "Backend Engineer", Company: "Acme", Locations: []string{"Paris", "France"}, URL: "https://acme.example.com/jobs/1"},
				{Title: "Product Manager", Company: "Acme", Locations: []string{"London", "UK"}, URL: "https://acme.example.com/jobs/2"},
			},
		})
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), affid: "aff-1", apiKey: "key-1", baseURL: srv.URL}

	// London + backend → only the Paris job matches the title but not the
	// location, so nothing survives.
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{
		Titles:    []string{"Backend Engineer"},
		WorkType:  "Onsite",
		Locations: []string{"London"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0 (title matched but location did not)", len(jobs))
	}
}

func TestSearchPassesAffiliateAndKey(t *testing.T) {
	var gotAffid, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAffid = r.URL.Query().Get("affid")
		gotKey = r.URL.Query().Get("api_key")
		json.NewEncoder(w).Encode(careerjetResponse{Type: "SUCCESS"})
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), affid: "aff-1", apiKey: "key-1", baseURL: srv.URL}

	if _, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Engineer"}}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotAffid != "aff-1" || gotKey != "key-1" {
		t.Errorf("credentials = affid:%q key:%q, want aff-1/key-1", gotAffid, gotKey)
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
