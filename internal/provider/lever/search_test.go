package lever

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestMatchesTitle(t *testing.T) {
	cases := []struct {
		title    string
		keywords []string
		want     bool
	}{
		{"Senior Software Engineer", []string{"software engineer"}, true},
		{"Backend Engineer", []string{"frontend engineer"}, false},
		{"Staff Engineer, Backend", []string{"backend", "frontend"}, true},
		{"Product Manager", []string{"engineer", "developer"}, false},
		{"", []string{"engineer"}, false},
		{"Engineer", []string{}, false},
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
		posting  leverPosting
		criteria provider.SearchCriteria
		want     bool
	}{
		{"remote matches Remote", leverPosting{Categories: leverCategories{Location: "Remote"}},
			provider.SearchCriteria{WorkType: "Remote"}, true},
		{"onsite fails Remote", leverPosting{Categories: leverCategories{Location: "New York"}},
			provider.SearchCriteria{WorkType: "Remote"}, false},
		{"remote matches Hybrid", leverPosting{Categories: leverCategories{Location: "Remote"}},
			provider.SearchCriteria{WorkType: "Hybrid"}, true},
		{"onsite matches by city", leverPosting{Categories: leverCategories{Location: "San Francisco, CA"}},
			provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"San Francisco"}}, true},
		{"onsite fails other city", leverPosting{Categories: leverCategories{Location: "New York, NY"}},
			provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"San Francisco"}}, false},
		{"no locations accepts all", leverPosting{Categories: leverCategories{Location: "Austin"}},
			provider.SearchCriteria{WorkType: "Onsite"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := matchesLocation(c.posting, c.criteria)
			if got != c.want {
				t.Errorf("matchesLocation = %v; want %v", got, c.want)
			}
		})
	}
}

func TestToProviderJob(t *testing.T) {
	co := leverCompany{Name: "Acme", Slug: "acme"}
	t.Run("apply URL takes priority", func(t *testing.T) {
		p := leverPosting{
			ID:               "1",
			Text:             "Go Engineer",
			ApplyURL:         "https://apply.lever.co/acme/1",
			HostedURL:        "https://jobs.lever.co/acme/1",
			DescriptionPlain: "desc",
			Categories:       leverCategories{Location: "Remote"},
		}
		j := toProviderJob(p, co)
		if j.URL != "https://apply.lever.co/acme/1" {
			t.Errorf("URL = %q; want apply URL", j.URL)
		}
		if j.Title != "Go Engineer" {
			t.Errorf("title = %q", j.Title)
		}
		if j.Company != "Acme" {
			t.Errorf("company = %q", j.Company)
		}
		if !j.Remote {
			t.Error("expected remote=true")
		}
		if j.Provider != "lever" {
			t.Errorf("provider = %q; want lever", j.Provider)
		}
	})
	t.Run("falls back to hosted URL", func(t *testing.T) {
		p := leverPosting{ID: "2", Text: "Dev", HostedURL: "https://jobs.lever.co/acme/2",
			Categories: leverCategories{Location: "San Francisco"}}
		j := toProviderJob(p, co)
		if j.URL != "https://jobs.lever.co/acme/2" {
			t.Errorf("URL = %q; want hosted URL", j.URL)
		}
	})
	t.Run("description concatenation", func(t *testing.T) {
		p := leverPosting{ID: "3", Text: "X", ApplyURL: "https://apply.lever.co/acme/3",
			DescriptionPlain: "main desc", AdditionalPlain: "extra info",
			Categories: leverCategories{Location: "Remote"}}
		j := toProviderJob(p, co)
		if !strings.Contains(j.Description, "main desc") {
			t.Error("description should contain main desc")
		}
		if !strings.Contains(j.Description, "extra info") {
			t.Error("description should contain additional plain")
		}
	})
	t.Run("falls back to HTML description", func(t *testing.T) {
		p := leverPosting{ID: "4", Text: "Y", ApplyURL: "https://apply.lever.co/acme/4",
			DescriptionHTML: "<p>html desc</p>", Categories: leverCategories{Location: "Remote"}}
		j := toProviderJob(p, co)
		if !strings.Contains(j.Description, "html desc") {
			t.Error("description should contain HTML desc when plain is empty")
		}
	})
}

func TestNew(t *testing.T) {
	c, err := New([]byte(`[{"name":"Acme","slug":"acme"}]`))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNew_MalformedJSON(t *testing.T) {
	_, err := New([]byte("{not json"))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestName(t *testing.T) {
	c, _ := New([]byte(`[{"name":"Acme","slug":"acme"}]`))
	if c.Name() != "lever" {
		t.Errorf("Name() = %q; want \"lever\"", c.Name())
	}
}

func TestMergeBoards(t *testing.T) {
	c, _ := New([]byte(`[{"name":"Acme","slug":"acme"}]`))
	extra := []provider.NamedBoard{
		{Name: "Beta", Board: "beta"},
		{Name: "Acme", Board: "acme"},
		{Name: "Gamma", Board: ""},
	}
	c.MergeBoards(extra)
	if len(c.companies) != 2 {
		t.Fatalf("expected 2 companies after merge, got %d", len(c.companies))
	}
}

func TestApply(t *testing.T) {
	c, _ := New([]byte(`[{"name":"Acme","slug":"acme"}]`))
	res, err := c.Apply(context.Background(), provider.Job{URL: "https://example.com/job/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if res.Status != "skipped" {
		t.Errorf("Apply status = %q; want \"skipped\"", res.Status)
	}
}

func TestFetchPostings(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "acme") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]leverPosting{
			{ID: "1", Text: "Go Engineer", ApplyURL: "https://apply.lever.co/acme/1",
				Categories: leverCategories{Location: "Remote"}, DescriptionPlain: "desc"},
		})
	}))
	defer ts.Close()

	orig := leverBaseURL
	leverBaseURL = ts.URL
	defer func() { leverBaseURL = orig }()

	postings, err := fetchPostings(context.Background(), &http.Client{}, "acme")
	if err != nil {
		t.Fatalf("fetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("expected 1 posting, got %d", len(postings))
	}
	if postings[0].Text != "Go Engineer" {
		t.Errorf("title = %q", postings[0].Text)
	}
}

func TestFetchPostings_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	orig := leverBaseURL
	leverBaseURL = ts.URL
	defer func() { leverBaseURL = orig }()

	postings, err := fetchPostings(context.Background(), &http.Client{}, "ghost")
	if err != nil {
		t.Fatalf("404 must return nil,nil; got err=%v", err)
	}
	if postings != nil {
		t.Fatalf("404 must return nil postings; got %d", len(postings))
	}
}

func TestFetchPostings_500(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	orig := leverBaseURL
	leverBaseURL = ts.URL
	defer func() { leverBaseURL = orig }()

	_, err := fetchPostings(context.Background(), &http.Client{}, "acme")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestFetchPostings_MalformedBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{not json"))
	}))
	defer ts.Close()

	orig := leverBaseURL
	leverBaseURL = ts.URL
	defer func() { leverBaseURL = orig }()

	_, err := fetchPostings(context.Background(), &http.Client{}, "acme")
	if err == nil {
		t.Fatal("expected decode error for malformed body")
	}
}
