package ashby

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
		{"Senior Software Engineer", []string{"software"}, true},
		{"Backend Engineer", []string{"frontend"}, false},
		{"Staff Engineer, Backend", []string{"backend"}, true},
		{"Product Manager", []string{"engineer"}, false},
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
		posting  ashbyJobPosting
		criteria provider.SearchCriteria
		want     bool
	}{
		{"remote matches Remote", ashbyJobPosting{IsRemote: true, LocationName: "Remote"},
			provider.SearchCriteria{WorkType: "Remote"}, true},
		{"onsite fails Remote", ashbyJobPosting{IsRemote: false, LocationName: "New York"},
			provider.SearchCriteria{WorkType: "Remote"}, false},
		{"remote matches Hybrid", ashbyJobPosting{IsRemote: true, LocationName: "Remote"},
			provider.SearchCriteria{WorkType: "Hybrid"}, true},
		{"onsite matches by city", ashbyJobPosting{IsRemote: false, LocationName: "San Francisco"},
			provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"San Francisco"}}, true},
		{"onsite fails other city", ashbyJobPosting{IsRemote: false, LocationName: "New York"},
			provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"San Francisco"}}, false},
		{"no locations accepts all", ashbyJobPosting{IsRemote: false, LocationName: "Austin"},
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
	co := ashbyCompany{Name: "Acme", Slug: "acme"}
	p := ashbyJobPosting{ID: "1", Title: "Go Engineer", LocationName: "Berlin", IsRemote: true}
	j := toProviderJob(p, co)
	if j.ID != "1" {
		t.Errorf("ID = %q; want \"1\"", j.ID)
	}
	if j.Title != "Go Engineer" {
		t.Errorf("title = %q", j.Title)
	}
	if j.Company != "Acme" {
		t.Errorf("company = %q", j.Company)
	}
	if j.Board != "acme" {
		t.Errorf("board = %q; want \"acme\"", j.Board)
	}
	if j.Location != "Berlin" {
		t.Errorf("location = %q", j.Location)
	}
	if !j.Remote {
		t.Error("expected remote=true")
	}
	if !strings.Contains(j.URL, "acme") || !strings.Contains(j.URL, "1") {
		t.Errorf("URL = %q; should contain slug and ID", j.URL)
	}
	if j.Provider != "ashby" {
		t.Errorf("provider = %q; want \"ashby\"", j.Provider)
	}
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
	if c.Name() != "ashby" {
		t.Errorf("Name() = %q; want \"ashby\"", c.Name())
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

func TestApply_SkipsManually(t *testing.T) {
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
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(ashbyGraphQLResponse{
			Data: struct {
				JobBoard ashbyJobBoard `json:"jobBoard"`
			}{
				JobBoard: ashbyJobBoard{
					JobPostings: []ashbyJobPosting{
						{ID: "1", Title: "Go Engineer", LocationName: "Berlin", IsRemote: true},
					},
				},
			},
		})
	}))
	defer ts.Close()

	orig := baseGraphQLURL
	baseGraphQLURL = ts.URL
	defer func() { baseGraphQLURL = orig }()

	postings, err := fetchPostings(context.Background(), &http.Client{}, "acme")
	if err != nil {
		t.Fatalf("fetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("expected 1 posting, got %d", len(postings))
	}
	if postings[0].Title != "Go Engineer" {
		t.Errorf("title = %q", postings[0].Title)
	}
}

func TestFetchPostings_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	orig := baseGraphQLURL
	baseGraphQLURL = ts.URL
	defer func() { baseGraphQLURL = orig }()

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

	orig := baseGraphQLURL
	baseGraphQLURL = ts.URL
	defer func() { baseGraphQLURL = orig }()

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

	orig := baseGraphQLURL
	baseGraphQLURL = ts.URL
	defer func() { baseGraphQLURL = orig }()

	_, err := fetchPostings(context.Background(), &http.Client{}, "acme")
	if err == nil {
		t.Fatal("expected decode error for malformed body")
	}
}
