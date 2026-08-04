package reed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestSearch(t *testing.T) {
	cases := []struct {
		name     string
		criteria provider.SearchCriteria
		handler  http.HandlerFunc
		wantJobs int
	}{
		{
			name:     "happy path returns matching jobs",
			criteria: provider.SearchCriteria{Titles: []string{"Software Engineer"}, WorkType: "Remote"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				user, _, ok := r.BasicAuth()
				if !ok || user != "test-key" {
					t.Errorf("expected basic auth user 'test-key', got %q", user)
				}
				if r.URL.Query().Get("keywords") != "Software Engineer" {
					t.Errorf("keywords = %q", r.URL.Query().Get("keywords"))
				}
				if r.URL.Query().Get("locationName") != "Remote" {
					t.Errorf("locationName = %q", r.URL.Query().Get("locationName"))
				}
				json.NewEncoder(w).Encode(reedResult{Results: []reedJob{
					{JobID: 1, JobTitle: "Senior Software Engineer", EmployerName: "Acme", LocationName: "London, Remote", JobURL: "https://www.reed.co.uk/jobs/1", MaximumSalary: 80000, DatePosted: "2026-08-01T10:00:00Z"},
					{JobID: 2, JobTitle: "Software Engineer (Frontend)", EmployerName: "Web Inc", LocationName: "Remote", JobURL: "https://www.reed.co.uk/jobs/2"},
				}})
			},
			wantJobs: 2,
		},
		{
			name:     "title filter excludes non-matching jobs",
			criteria: provider.SearchCriteria{Titles: []string{"Backend"}, WorkType: "Remote"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(reedResult{Results: []reedJob{
					{JobID: 1, JobTitle: "Backend Engineer", EmployerName: "A", LocationName: "Remote", JobURL: "https://reed.co.uk/jobs/1"},
					{JobID: 2, JobTitle: "Marketing Manager", EmployerName: "B", LocationName: "Remote", JobURL: "https://reed.co.uk/jobs/2"},
				}})
			},
			wantJobs: 1,
		},
		{
			name:     "salary floor filters low-paying jobs",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Remote", MinSalary: 50000},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(reedResult{Results: []reedJob{
					{JobID: 1, JobTitle: "Engineer", EmployerName: "A", LocationName: "Remote", JobURL: "https://reed.co.uk/jobs/1", MaximumSalary: 40000},
					{JobID: 2, JobTitle: "Engineer", EmployerName: "B", LocationName: "Remote", JobURL: "https://reed.co.uk/jobs/2", MaximumSalary: 90000},
				}})
			},
			wantJobs: 1,
		},
		{
			name:     "empty results returns no jobs",
			criteria: provider.SearchCriteria{Titles: []string{"Go"}, WorkType: "Remote"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(reedResult{Results: []reedJob{}})
			},
			wantJobs: 0,
		},
		{
			name:     "job without url falls back to constructed url",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Remote"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(reedResult{Results: []reedJob{
					{JobID: 42, JobTitle: "Engineer", EmployerName: "A", LocationName: "Remote"},
				}})
			},
			wantJobs: 1,
		},
		{
			name:     "onsite location filter",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Onsite", Locations: []string{"London"}},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(reedResult{Results: []reedJob{
					{JobID: 1, JobTitle: "Engineer", EmployerName: "A", LocationName: "London, UK", JobURL: "https://reed.co.uk/jobs/1"},
					{JobID: 2, JobTitle: "Engineer", EmployerName: "B", LocationName: "Manchester", JobURL: "https://reed.co.uk/jobs/2"},
				}})
			},
			wantJobs: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			defer ts.Close()

			c := New("test-key")
			c.baseURL = ts.URL

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			jobs, err := c.Search(ctx, tc.criteria)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(jobs) != tc.wantJobs {
				t.Errorf("got %d jobs, want %d", len(jobs), tc.wantJobs)
			}
			for _, j := range jobs {
				if j.URL == "" {
					t.Error("expected non-empty URL")
				}
			}
		})
	}
}

// TestSearchSkipsOnServerError verifies §10 provider isolation: a failing
// query must not abort the run — Search returns 0 jobs with no error.
func TestSearchSkipsOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c := New("test-key")
	c.baseURL = srv.URL

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Go"}, WorkType: "Remote"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

func TestFetchPageMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{not valid json"))
	}))
	t.Cleanup(srv.Close)
	c := New("test-key")
	c.baseURL = srv.URL

	_, err := c.fetchPage(context.Background(), "Go", "Remote", 0, 100, provider.SearchCriteria{WorkType: "Remote"})
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestFetchPageContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(reedResult{})
	}))
	t.Cleanup(srv.Close)
	c := New("test-key")
	c.baseURL = srv.URL

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := c.fetchPage(ctx, "Go", "Remote", 0, 100, provider.SearchCriteria{WorkType: "Remote"})
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

func TestSearchBadHost(t *testing.T) {
	c := New("test-key")
	c.baseURL = "http://127.0.0.1:0" // unreachable port — isolation: no error, 0 jobs
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Go"}, WorkType: "Remote"})
	if err != nil {
		t.Fatalf("unexpected error for bad host: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("got %d jobs, want 0", len(jobs))
	}
}

func TestApplySkipped(t *testing.T) {
	c := New("test-key")
	res, err := c.Apply(context.Background(), provider.Job{URL: "https://www.reed.co.uk/jobs/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != "skipped" {
		t.Errorf("status = %q, want skipped", res.Status)
	}
}
