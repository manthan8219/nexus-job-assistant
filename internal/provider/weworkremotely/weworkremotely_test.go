package weworkremotely

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestMatchesTitle(t *testing.T) {
	cases := []struct {
		title    string
		keywords []string
		want     bool
	}{
		{"Senior Go Engineer", []string{"go"}, true},
		{"Senior Go Engineer", []string{"python"}, false},
		{"Senior Go Engineer", []string{"GO"}, true},
		{"", []string{"go"}, false},
		{"Senior Go Engineer", []string{}, true},
	}
	for _, c := range cases {
		got := matchesTitle(c.title, c.keywords)
		if got != c.want {
			t.Errorf("matchesTitle(%q, %v) = %v; want %v", c.title, c.keywords, got, c.want)
		}
	}
}

func TestMatchesLocation(t *testing.T) {
	cases := []struct {
		name     string
		location string
		remote   bool
		criteria provider.SearchCriteria
		want     bool
	}{
		{"remote matches Remote", "Remote", true, provider.SearchCriteria{WorkType: "Remote"}, true},
		{"onsite fails Remote", "Berlin", false, provider.SearchCriteria{WorkType: "Remote"}, false},
		{"no locations accepts all", "Berlin", false, provider.SearchCriteria{WorkType: "Onsite"}, true},
		{"remote matches Hybrid", "Remote", true, provider.SearchCriteria{WorkType: "Hybrid"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := matchesLocation(c.location, c.remote, c.criteria)
			if got != c.want {
				t.Errorf("matchesLocation(%q, %v, wt=%s) = %v; want %v",
					c.location, c.remote, c.criteria.WorkType, got, c.want)
			}
		})
	}
}

func TestName(t *testing.T) {
	c := New()
	if c.Name() != "weworkremotely" {
		t.Errorf("Name() = %q; want \"weworkremotely\"", c.Name())
	}
}

func TestApply(t *testing.T) {
	c := New()
	res, err := c.Apply(context.Background(), provider.Job{URL: "https://example.com/job/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if res.Status != "skipped" {
		t.Errorf("Apply status = %q; want \"skipped\"", res.Status)
	}
}

func TestSearch(t *testing.T) {
	rss := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Acme: Senior Go Engineer</title>
      <link>https://weworkremotely.com/job/1</link>
      <description>Great opportunity</description>
    </item>
    <item>
      <title>Web Inc: Frontend Developer</title>
      <link>https://weworkremotely.com/job/2</link>
    </item>
  </channel>
</rss>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(rss))
	}))
	defer ts.Close()

	orig := feedURL
	feedURL = ts.URL
	defer func() { feedURL = orig }()

	c := New()
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Go"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Title != "Senior Go Engineer" {
		t.Errorf("title = %q; want \"Senior Go Engineer\"", jobs[0].Title)
	}
	if jobs[0].Company != "Acme" {
		t.Errorf("company = %q; want \"Acme\"", jobs[0].Company)
	}
	if !jobs[0].Remote {
		t.Error("expected remote=true for weworkremotely")
	}
	if jobs[0].Provider != "weworkremotely" {
		t.Errorf("provider = %q; want \"weworkremotely\"", jobs[0].Provider)
	}
	if jobs[0].Description != "Great opportunity" {
		t.Errorf("description = %q; want \"Great opportunity\"", jobs[0].Description)
	}
}

func TestSearch_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	orig := feedURL
	feedURL = ts.URL
	defer func() { feedURL = orig }()

	c := New()
	_, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err == nil {
		t.Fatal("expected error for HTTP 403")
	}
}

func TestSearch_MalformedXML(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<not valid xml"))
	}))
	defer ts.Close()

	orig := feedURL
	feedURL = ts.URL
	defer func() { feedURL = orig }()

	c := New()
	_, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err == nil {
		t.Fatal("expected xml parse error for malformed body")
	}
}

func TestSearch_EmptyTitleOrURISkipped(t *testing.T) {
	rss := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item><title></title><link>https://weworkremotely.com/job/1</link></item>
    <item><title>Valid</title><link></link></item>
    <item><title>Acme: Go Engineer</title><link>https://weworkremotely.com/job/3</link></item>
  </channel>
</rss>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(rss))
	}))
	defer ts.Close()

	orig := feedURL
	feedURL = ts.URL
	defer func() { feedURL = orig }()

	c := New()
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 valid job, got %d", len(jobs))
	}
}

func TestSearch_TitleWithoutColon(t *testing.T) {
	rss := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <item>
      <title>No Colon Here</title>
      <link>https://weworkremotely.com/job/1</link>
    </item>
  </channel>
</rss>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(rss))
	}))
	defer ts.Close()

	orig := feedURL
	feedURL = ts.URL
	defer func() { feedURL = orig }()

	c := New()
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Title != "No Colon Here" {
		t.Errorf("title = %q; want \"No Colon Here\"", jobs[0].Title)
	}
	if jobs[0].Company != "We Work Remotely" {
		t.Errorf("company = %q; want \"We Work Remotely\"", jobs[0].Company)
	}
}

func TestSearch_ContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))
	}))
	defer ts.Close()

	orig := feedURL
	feedURL = ts.URL
	defer func() { feedURL = orig }()

	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Search(ctx, provider.SearchCriteria{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
