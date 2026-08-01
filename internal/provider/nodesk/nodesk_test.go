package nodesk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestSanitizeXML(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"HTML entities replaced", "Hello &rsquo;world&rsquo;", "Hello 'world'"},
		{"Multiple entities", "&ldquo;quoted&rdquo;", "\u201cquoted\u201d"},
		{"Ampersand double-encoded", "&amp;amp;", "&amp;"},
		{"nbsp to space", "Hello&nbsp;World", "Hello World"},
		{"No entities", "Plain text", "Plain text"},
		{"Empty string", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(sanitizeXML([]byte(c.input)))
			if got != c.want {
				t.Errorf("sanitizeXML(%q) = %q; want %q", c.input, got, c.want)
			}
		})
	}
}

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
	if c.Name() != "nodesk" {
		t.Errorf("Name() = %q; want \"nodesk\"", c.Name())
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
      <title>Senior Go Engineer &rsquo;Remote&rsquo;</title>
      <link>https://nodesk.co/job/1</link>
    </item>
    <item>
      <title>Frontend Developer</title>
      <link>https://nodesk.co/job/2</link>
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
	if jobs[0].Title != "Senior Go Engineer 'Remote'" {
		t.Errorf("title = %q; want \"Senior Go Engineer 'Remote'\"", jobs[0].Title)
	}
	if jobs[0].Company != "NoDesk" {
		t.Errorf("company = %q; want \"NoDesk\"", jobs[0].Company)
	}
	if !jobs[0].Remote {
		t.Error("expected remote=true for nodesk")
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
    <item><title></title><link>https://nodesk.co/job/1</link></item>
    <item><title>Valid</title><link></link></item>
    <item><title>Go Engineer</title><link>https://nodesk.co/job/3</link></item>
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
