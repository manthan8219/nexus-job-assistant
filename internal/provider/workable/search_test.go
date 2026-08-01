package workable

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
		{"Backend Engineer", []string{"frontend"}, false},
		{"Staff Engineer, Backend", []string{"backend", "frontend"}, true},
		{"Product Manager", []string{"engineer", "developer"}, false},
		{"", []string{"engineer"}, false},
		{"Engineer", []string{}, false},
	}
	for _, c := range cases {
		if got := matchesTitle(c.title, c.keywords); got != c.want {
			t.Errorf("matchesTitle(%q, %v) = %v; want %v", c.title, c.keywords, got, c.want)
		}
	}
}

func TestMatchesLocation(t *testing.T) {
	remote := workableJob{Location: workableLocation{City: "Berlin", Country: "Germany", Remote: true}}
	onsite := workableJob{Location: workableLocation{City: "Austin", Country: "USA", Remote: false}}
	cases := []struct {
		name     string
		job      workableJob
		criteria provider.SearchCriteria
		want     bool
	}{
		{"remote matches Remote", remote, provider.SearchCriteria{WorkType: "Remote"}, true},
		{"remote matches Hybrid", remote, provider.SearchCriteria{WorkType: "Hybrid"}, true},
		{"onsite fails Remote filter", onsite, provider.SearchCriteria{WorkType: "Remote"}, false},
		{"onsite matches by city", onsite, provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"Austin"}}, true},
		{"onsite fails other city", onsite, provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"Berlin"}}, false},
		{"no locations accepts all", onsite, provider.SearchCriteria{WorkType: "Onsite"}, true},
	}
	for _, c := range cases {
		if got := matchesLocation(c.job, c.criteria); got != c.want {
			t.Errorf("%s: matchesLocation = %v; want %v", c.name, got, c.want)
		}
	}
}

func TestToProviderJob(t *testing.T) {
	co := workableCompany{Name: "Acme", Slug: "acme"}
	t.Run("full location and explicit URL", func(t *testing.T) {
		p := workableJob{ID: "1", Title: "Backend Engineer", Shortcode: "BE1",
			Location: workableLocation{City: "Berlin", Country: "Germany", Remote: true},
			URL:      "https://apply.workable.com/acme/j/BE1/"}
		j := toProviderJob(p, co)
		if j.Title != "Backend Engineer" || j.Company != "Acme" || j.Board != "acme" {
			t.Errorf("identity fields wrong: %+v", j)
		}
		if !strings.Contains(j.Location, "Berlin") || !strings.Contains(j.Location, "Germany") {
			t.Errorf("location = %q; want Berlin, Germany", j.Location)
		}
		if !j.Remote {
			t.Error("expected Remote=true")
		}
		if j.URL != p.URL {
			t.Errorf("URL = %q; want explicit %q", j.URL, p.URL)
		}
		if j.Provider != "workable" {
			t.Errorf("Provider = %q; want workable", j.Provider)
		}
	})
	t.Run("empty URL derives apply URL from slug+shortcode", func(t *testing.T) {
		p := workableJob{ID: "2", Title: "Dev", Shortcode: "SC2", Location: workableLocation{}}
		j := toProviderJob(p, co)
		if !strings.Contains(j.URL, "acme") || !strings.Contains(j.URL, "SC2") {
			t.Errorf("derived URL = %q; want to contain slug and shortcode", j.URL)
		}
	})
	t.Run("country only location", func(t *testing.T) {
		p := workableJob{ID: "3", Title: "X", Shortcode: "S3", Location: workableLocation{Country: "India"}}
		j := toProviderJob(p, co)
		if j.Location != "India" {
			t.Errorf("location = %q; want India", j.Location)
		}
	})
}

func TestFetchPostings(t *testing.T) {
	t.Run("200 returns jobs", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.Path, "acme") {
				t.Errorf("unexpected slug in path: %s", r.URL.Path)
			}
			json.NewEncoder(w).Encode(workableJobsResponse{Results: []workableJob{
				{ID: "1", Title: "Engineer", Shortcode: "S1", URL: "https://apply.workable.com/acme/j/S1/"},
			}})
		}))
		defer ts.Close()
		orig := workableBaseURL
		workableBaseURL = ts.URL
		defer func() { workableBaseURL = orig }()

		jobs, err := fetchPostings(context.Background(), &http.Client{}, "acme")
		if err != nil {
			t.Fatalf("fetchPostings: %v", err)
		}
		if len(jobs) != 1 || jobs[0].Title != "Engineer" {
			t.Fatalf("got %d jobs: %+v", len(jobs), jobs)
		}
	})
	t.Run("404 returns nil nil", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()
		orig := workableBaseURL
		workableBaseURL = ts.URL
		defer func() { workableBaseURL = orig }()
		jobs, err := fetchPostings(context.Background(), &http.Client{}, "ghost")
		if err != nil {
			t.Fatalf("404 must be nil,nil; got err=%v", err)
		}
		if jobs != nil {
			t.Fatalf("404 must return nil jobs; got %d", len(jobs))
		}
	})
	t.Run("500 returns error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()
		orig := workableBaseURL
		workableBaseURL = ts.URL
		defer func() { workableBaseURL = orig }()
		if _, err := fetchPostings(context.Background(), &http.Client{}, "acme"); err == nil {
			t.Fatal("expected error for HTTP 500")
		}
	})
	t.Run("malformed body returns error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{not json"))
		}))
		defer ts.Close()
		orig := workableBaseURL
		workableBaseURL = ts.URL
		defer func() { workableBaseURL = orig }()
		if _, err := fetchPostings(context.Background(), &http.Client{}, "acme"); err == nil {
			t.Fatal("expected decode error for malformed body")
		}
	})
}
