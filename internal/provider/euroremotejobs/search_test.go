package euroremotejobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

const testFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item><title>Senior Backend Engineer at Acme</title><link>https://acme.example.com/jobs/1</link><description>Remote</description></item>
    <item><title>Product Manager at Globex</title><link>https://globex.example.com/jobs/2</link><description>Remote</description></item>
  </channel>
</rss>`

func TestSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testFeed))
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), feedURL: srv.URL}

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
	if got.URL != "https://acme.example.com/jobs/1" {
		t.Errorf("URL = %q", got.URL)
	}
	if !got.Remote {
		t.Error("remote = false, want true")
	}
}

func TestSearchSkipsOnBadXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not xml"))
	}))
	t.Cleanup(srv.Close)
	c := &Client{http: srv.Client(), feedURL: srv.URL}

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Engineer"}})
	if err == nil {
		t.Fatalf("Search succeeded (%d jobs), want an error on malformed XML", len(jobs))
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
