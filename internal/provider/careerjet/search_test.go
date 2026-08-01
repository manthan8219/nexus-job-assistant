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
