package smartrecruiters

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
		name     string
		jobName  string
		keywords []string
		want     bool
	}{
		{"match", "Senior Software Engineer", []string{"software"}, true},
		{"no match", "Backend Engineer", []string{"frontend"}, false},
		{"case insensitive", "Senior Go Engineer", []string{"GO"}, true},
		{"empty keywords", "Engineer", []string{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := matchesTitle(c.jobName, c.keywords)
			if got != c.want {
				t.Errorf("matchesTitle(%q, %v) = %v; want %v", c.jobName, c.keywords, got, c.want)
			}
		})
	}
}

func TestMatchesLocation(t *testing.T) {
	cases := []struct {
		name     string
		posting  srPosting
		criteria provider.SearchCriteria
		want     bool
	}{
		{"remote matches Remote", srPosting{Location: srLocation{Remote: true}},
			provider.SearchCriteria{WorkType: "Remote"}, true},
		{"onsite fails Remote", srPosting{Location: srLocation{Remote: false}},
			provider.SearchCriteria{WorkType: "Remote"}, false},
		{"remote matches Hybrid", srPosting{Location: srLocation{Remote: true}},
			provider.SearchCriteria{WorkType: "Hybrid"}, true},
		{"city match", srPosting{Location: srLocation{FullLocation: "San Francisco, CA"}},
			provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"San Francisco"}}, true},
		{"no locations accepts all", srPosting{Location: srLocation{FullLocation: "Austin"}},
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
	co := srCompanyEntry{Name: "Acme", Identifier: "acme"}
	t.Run("basic conversion", func(t *testing.T) {
		p := srPosting{
			ID:           "1",
			Name:         "Go Engineer",
			ReleasedDate: "2026-07-28T12:00:00.000Z",
			Location:     srLocation{FullLocation: "San Francisco, CA", Remote: false},
		}
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
		if j.Provider != "smartrecruiters" {
			t.Errorf("provider = %q; want \"smartrecruiters\"", j.Provider)
		}
		if j.URL != "https://jobs.smartrecruiters.com/acme/1" {
			t.Errorf("URL = %q", j.URL)
		}
	})
	t.Run("city fallback when no full location", func(t *testing.T) {
		p := srPosting{ID: "2", Name: "Dev", Location: srLocation{City: "Berlin", FullLocation: ""}}
		j := toProviderJob(p, co)
		if j.Location != "Berlin" {
			t.Errorf("location = %q; want \"Berlin\"", j.Location)
		}
	})
	t.Run("empty released date", func(t *testing.T) {
		p := srPosting{ID: "3", Name: "X", ReleasedDate: "", Location: srLocation{}}
		j := toProviderJob(p, co)
		if !j.PostedAt.IsZero() {
			t.Error("PostedAt should be zero for empty released date")
		}
	})
}

func TestNew(t *testing.T) {
	c, err := New([]byte(`[{"name":"Acme","identifier":"acme"}]`))
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
	c, _ := New([]byte(`[{"name":"Acme","identifier":"acme"}]`))
	if c.Name() != "smartrecruiters" {
		t.Errorf("Name() = %q; want \"smartrecruiters\"", c.Name())
	}
}

func TestMergeBoards(t *testing.T) {
	c, _ := New([]byte(`[{"name":"Acme","identifier":"acme"}]`))
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
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	orig := baseURL
	baseURL = ts.URL
	defer func() { baseURL = orig }()

	c, _ := New([]byte(`[{"name":"Acme","identifier":"acme"}]`))
	res, err := c.Apply(context.Background(), provider.Job{URL: "https://example.com/job/1", Board: "acme", ID: "1"}, provider.Profile{})
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
		json.NewEncoder(w).Encode(srResponse{
			Content: []srPosting{
				{ID: "1", Name: "Go Engineer", ReleasedDate: "2026-07-28T12:00:00.000Z",
					Location: srLocation{FullLocation: "Remote", Remote: true}},
			},
		})
	}))
	defer ts.Close()

	orig := baseURL
	baseURL = ts.URL
	defer func() { baseURL = orig }()

	postings, err := fetchPostings(context.Background(), &http.Client{}, "acme")
	if err != nil {
		t.Fatalf("fetchPostings: %v", err)
	}
	if len(postings) != 1 {
		t.Fatalf("expected 1 posting, got %d", len(postings))
	}
}

func TestFetchPostings_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	orig := baseURL
	baseURL = ts.URL
	defer func() { baseURL = orig }()

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

	orig := baseURL
	baseURL = ts.URL
	defer func() { baseURL = orig }()

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

	orig := baseURL
	baseURL = ts.URL
	defer func() { baseURL = orig }()

	_, err := fetchPostings(context.Background(), &http.Client{}, "acme")
	if err == nil {
		t.Fatal("expected decode error for malformed body")
	}
}
