package jooble

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/api/key-123") {
			t.Errorf("path = %q, want /api/key-123", r.URL.Path)
		}
		var reqBody map[string]string
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if reqBody["keywords"] == "" {
			t.Error("keywords must be set in the request body")
		}
		json.NewEncoder(w).Encode(joobleResponse{
			TotalCount: 1,
			Jobs: []joobleJob{
				{Title: "Senior Backend Engineer", Company: "Acme", Location: "Remote", Link: "https://acme.example.com/jobs/1"},
			},
		})
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), apiKey: "key-123", baseURL: srv.URL}

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{
		Titles:   []string{"Backend Engineer"},
		WorkType: "Remote",
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
	if !got.Remote {
		t.Error("remote flag = false, want true")
	}
	if got.URL != "https://acme.example.com/jobs/1" {
		t.Errorf("URL = %q", got.URL)
	}
}

func TestSearchSkipsOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), apiKey: "key-123", baseURL: srv.URL}

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Engineer"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestSearchFiltersByTitleAndLocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(joobleResponse{
			Jobs: []joobleJob{
				{Title: "Senior Backend Engineer", Company: "Acme", Location: "Warsaw", Link: "https://acme.example.com/jobs/1"},
				{Title: "Product Manager", Company: "Acme", Location: "Remote", Link: "https://acme.example.com/jobs/2"},
				{Title: "Backend Engineer", Company: "Acme", Location: "Remote", Link: "https://acme.example.com/jobs/3"},
			},
		})
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), apiKey: "key-123", baseURL: srv.URL}

	// Remote-only + backend titles → only the third job matches.
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{
		Titles:   []string{"Backend Engineer"},
		WorkType: "Remote",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (title + remote filters)", len(jobs))
	}
	if jobs[0].URL != "https://acme.example.com/jobs/3" {
		t.Errorf("URL = %q, want the third job", jobs[0].URL)
	}
}

func TestSearchSendsKeywords(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		json.NewEncoder(w).Encode(joobleResponse{})
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), apiKey: "key-123", baseURL: srv.URL}

	if _, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Go Developer"}}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotBody["keywords"] != "Go Developer" {
		t.Errorf("keywords = %q, want Go Developer", gotBody["keywords"])
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
