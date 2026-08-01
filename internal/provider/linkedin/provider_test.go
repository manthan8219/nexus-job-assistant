package linkedin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestNew_DefaultMaxPages(t *testing.T) {
	if p := New(0); p.maxPages != 3 {
		t.Errorf("New(0).maxPages = %d; want 3", p.maxPages)
	}
	if p := New(5); p.maxPages != 5 {
		t.Errorf("New(5).maxPages = %d; want 5", p.maxPages)
	}
}

func TestNew_NegativeMaxPages(t *testing.T) {
	if p := New(-1); p.maxPages != 3 {
		t.Errorf("New(-1).maxPages = %d; want 3", p.maxPages)
	}
}

func TestName(t *testing.T) {
	if p := New(1); p.Name() != "linkedin" {
		t.Errorf("Name() = %q; want \"linkedin\"", p.Name())
	}
}

func TestApply(t *testing.T) {
	p := New(1)
	res, err := p.Apply(context.Background(), provider.Job{URL: "https://linkedin.com/jobs/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "skipped" {
		t.Errorf("status = %q; want \"skipped\"", res.Status)
	}
}

func TestSearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/scrape/linkedin" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"jobs":[{"title":"Go Engineer","company":"Acme","location":"Berlin","apply_url":"https://linkedin.com/jobs/1","remote":false}],"total_found":1}`))
	}))
	defer ts.Close()

	origEnsure := ensureScraper
	ensureScraper = func() error { return nil }
	defer func() { ensureScraper = origEnsure }()

	p := New(2)
	p.baseURL = ts.URL

	jobs, err := p.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Go"}, Locations: []string{"Berlin"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	j := jobs[0]
	if j.Title != "Go Engineer" {
		t.Errorf("title = %q; want \"Go Engineer\"", j.Title)
	}
	if j.Company != "Acme" {
		t.Errorf("company = %q; want \"Acme\"", j.Company)
	}
	if j.Provider != "linkedin" {
		t.Errorf("provider = %q; want \"linkedin\"", j.Provider)
	}
	if j.URL != "https://linkedin.com/jobs/1" {
		t.Errorf("url = %q; want apply url", j.URL)
	}
}

func TestSearch_EmptyKeywordsReturnsNil(t *testing.T) {
	origEnsure := ensureScraper
	ensureScraper = func() error { return nil }
	defer func() { ensureScraper = origEnsure }()

	p := New(1)
	jobs, err := p.Search(context.Background(), provider.SearchCriteria{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if jobs != nil {
		t.Errorf("expected nil jobs for empty keywords, got %d", len(jobs))
	}
}

func TestSearch_ErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"error":"scraper failed"}`))
	}))
	defer ts.Close()

	origEnsure := ensureScraper
	ensureScraper = func() error { return nil }
	defer func() { ensureScraper = origEnsure }()

	p := New(1)
	p.baseURL = ts.URL

	if _, err := p.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Go"}}); err == nil {
		t.Fatal("expected error for error response")
	}
}

func TestSearch_NoTitleMatchSkips(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"jobs":[{"title":"Product Manager","company":"Acme","location":"Berlin","apply_url":"https://linkedin.com/jobs/2","remote":false}],"total_found":1}`))
	}))
	defer ts.Close()

	origEnsure := ensureScraper
	ensureScraper = func() error { return nil }
	defer func() { ensureScraper = origEnsure }()

	p := New(1)
	p.baseURL = ts.URL

	jobs, err := p.Search(context.Background(), provider.SearchCriteria{Titles: []string{"engineer"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs for non-matching title, got %d", len(jobs))
	}
}
