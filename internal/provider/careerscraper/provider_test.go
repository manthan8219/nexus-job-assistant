package careerscraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestNew(t *testing.T) {
	p := New([]Target{{Company: "Acme", URL: "https://acme.com/jobs"}}, "llama3.2", "http://localhost:11434")
	if len(p.targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(p.targets))
	}
	if p.ollamaModel != "llama3.2" {
		t.Errorf("ollamaModel = %q; want \"llama3.2\"", p.ollamaModel)
	}
	if p.ollamaURL != "http://localhost:11434" {
		t.Errorf("ollamaURL = %q", p.ollamaURL)
	}
}

func TestName(t *testing.T) {
	if p := New(nil, "", ""); p.Name() != "careerscraper" {
		t.Errorf("Name() = %q; want \"careerscraper\"", p.Name())
	}
}

func TestApply(t *testing.T) {
	p := New(nil, "", "")
	res, err := p.Apply(context.Background(), provider.Job{URL: "https://acme.com/jobs/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Status != "skipped" {
		t.Errorf("status = %q; want \"skipped\"", res.Status)
	}
}

func TestSearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/scrape/batch" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"results":[{"company":"Acme","url":"https://acme.com/jobs","jobs":[{"title":"Go Engineer","company":"Acme","location":"Berlin","apply_url":"https://acme.com/jobs/1","remote":false}]}]}`))
	}))
	defer ts.Close()

	origEnsure := ensureScraper
	ensureScraper = func(_, _ string) error { return nil }
	defer func() { ensureScraper = origEnsure }()

	p := New([]Target{{Company: "Acme", URL: "https://acme.com/jobs"}}, "", "")
	p.baseURL = ts.URL

	jobs, err := p.Search(context.Background(), provider.SearchCriteria{})
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
	if j.Provider != "careerscraper" {
		t.Errorf("provider = %q; want \"careerscraper\"", j.Provider)
	}
	if j.Board != "career_page" {
		t.Errorf("board = %q; want \"career_page\"", j.Board)
	}
	if j.URL != "https://acme.com/jobs/1" {
		t.Errorf("url = %q; want apply url", j.URL)
	}
}

func TestSearch_ResultErrorSkipped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"results":[{"company":"Acme","error":"scrape timeout"}]}`))
	}))
	defer ts.Close()

	origEnsure := ensureScraper
	ensureScraper = func(_, _ string) error { return nil }
	defer func() { ensureScraper = origEnsure }()

	p := New([]Target{{Company: "Acme", URL: "https://acme.com/jobs"}}, "", "")
	p.baseURL = ts.URL

	jobs, err := p.Search(context.Background(), provider.SearchCriteria{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("result with error should yield 0 jobs, got %d", len(jobs))
	}
}

func TestSearch_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	origEnsure := ensureScraper
	ensureScraper = func(_, _ string) error { return nil }
	defer func() { ensureScraper = origEnsure }()

	p := New([]Target{{Company: "Acme", URL: "https://acme.com/jobs"}}, "", "")
	p.baseURL = ts.URL

	if _, err := p.Search(context.Background(), provider.SearchCriteria{}); err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}
