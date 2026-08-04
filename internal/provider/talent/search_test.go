package talent

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
				if r.URL.Query().Get("key") != "test-key" {
					t.Errorf("key = %q, want test-key", r.URL.Query().Get("key"))
				}
				if r.URL.Query().Get("jobtitle") != "Software Engineer" {
					t.Errorf("jobtitle = %q", r.URL.Query().Get("jobtitle"))
				}
				if r.URL.Query().Get("location") != "Remote" {
					t.Errorf("location = %q", r.URL.Query().Get("location"))
				}
				json.NewEncoder(w).Encode(talentResponse{Jobs: []talentJob{
					{ID: "1", Title: "Senior Software Engineer", Company: "Acme", Location: "Remote", URL: "https://talent.com/jobs/1", Remote: true, Posted: "2026-08-01"},
					{ID: "2", Title: "Software Engineer II", Company: "Web Inc", Location: "Remote", URL: "https://talent.com/jobs/2"},
				}})
			},
			wantJobs: 2,
		},
		{
			name:     "title filter excludes non-matching jobs",
			criteria: provider.SearchCriteria{Titles: []string{"Backend"}, WorkType: "Remote"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(talentResponse{Jobs: []talentJob{
					{ID: "1", Title: "Backend Engineer", Company: "A", Location: "Remote", URL: "https://talent.com/jobs/1"},
					{ID: "2", Title: "Marketing Manager", Company: "B", Location: "Remote", URL: "https://talent.com/jobs/2"},
				}})
			},
			wantJobs: 1,
		},
		{
			name:     "salary floor filters low-paying jobs",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Remote", MinSalary: 50000},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(talentResponse{Jobs: []talentJob{
					{ID: "1", Title: "Engineer", Company: "A", Location: "Remote", URL: "https://talent.com/jobs/1", SalaryMax: 40000},
					{ID: "2", Title: "Engineer", Company: "B", Location: "Remote", URL: "https://talent.com/jobs/2", SalaryMax: 90000},
				}})
			},
			wantJobs: 1,
		},
		{
			name:     "empty results returns no jobs",
			criteria: provider.SearchCriteria{Titles: []string{"Go"}, WorkType: "Remote"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(talentResponse{})
			},
			wantJobs: 0,
		},
		{
			name:     "onsite location filter",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Onsite", Locations: []string{"Mumbai"}},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(talentResponse{Jobs: []talentJob{
					{ID: "1", Title: "Engineer", Company: "A", Location: "Mumbai, India", URL: "https://talent.com/jobs/1"},
					{ID: "2", Title: "Engineer", Company: "B", Location: "Delhi", URL: "https://talent.com/jobs/2"},
				}})
			},
			wantJobs: 1,
		},
		{
			name:     "job without url is skipped",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Remote"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(talentResponse{Jobs: []talentJob{
					{ID: "1", Title: "Engineer", Company: "A", Location: "Remote", URL: ""},
				}})
			},
			wantJobs: 0,
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
		})
	}
}

func TestSearchSkipsOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
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

	_, err := c.fetchPage(context.Background(), "Go", "Remote", 1, 30, provider.SearchCriteria{WorkType: "Remote"})
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestFetchPageContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(talentResponse{})
	}))
	t.Cleanup(srv.Close)
	c := New("test-key")
	c.baseURL = srv.URL

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := c.fetchPage(ctx, "Go", "Remote", 1, 30, provider.SearchCriteria{WorkType: "Remote"})
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

func TestApplySkipped(t *testing.T) {
	c := New("test-key")
	res, err := c.Apply(context.Background(), provider.Job{URL: "https://talent.com/jobs/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != "skipped" {
		t.Errorf("status = %q, want skipped", res.Status)
	}
}
