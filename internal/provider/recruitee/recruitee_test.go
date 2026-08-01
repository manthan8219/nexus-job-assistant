package recruitee

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
		{"remote matches Remote", "Worldwide", true, provider.SearchCriteria{WorkType: "Remote"}, true},
		{"onsite fails Remote", "Berlin", false, provider.SearchCriteria{WorkType: "Remote"}, false},
		{"onsite matches by city", "Berlin", false, provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"Berlin"}}, true},
		{"onsite fails other city", "Berlin", false, provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"Paris"}}, false},
		{"no locations accepts all", "Berlin", false, provider.SearchCriteria{WorkType: "Onsite"}, true},
		{"remote matches Hybrid", "Worldwide", true, provider.SearchCriteria{WorkType: "Hybrid"}, true},
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
	if c.Name() != "recruitee" {
		t.Errorf("Name() = %q; want \"recruitee\"", c.Name())
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

func TestSearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/offers/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(rctResponse{
			Offers: []rctOffer{
				{Title: "Go Engineer", CareersURL: "https://acme.recruitee.com/o/1", City: "Berlin", Country: "Germany", Remote: false},
			},
		})
	}))
	defer ts.Close()

	orig := offerURLFmt
	offerURLFmt = ts.URL + "/%s/api/offers/"
	defer func() { offerURLFmt = orig }()

	c, _ := New([]byte(`[{"name":"Acme","slug":"acme"}]`))
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{})
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
	if j.Location != "Berlin, Germany" {
		t.Errorf("location = %q; want \"Berlin, Germany\"", j.Location)
	}
	if j.Provider != "recruitee" {
		t.Errorf("provider = %q; want \"recruitee\"", j.Provider)
	}
	if j.Board != "acme" {
		t.Errorf("board = %q; want \"acme\"", j.Board)
	}
	if j.Remote {
		t.Error("expected remote=false for Berlin")
	}
}

func TestSearch_Non200Skipped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	orig := offerURLFmt
	offerURLFmt = ts.URL + "/%s/api/offers/"
	defer func() { offerURLFmt = orig }()

	c, _ := New([]byte(`[{"name":"Acme","slug":"acme"}]`))
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("non-200 should yield no jobs, got %d", len(jobs))
	}
}

func TestSearch_MalformedBodySkipped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{not json"))
	}))
	defer ts.Close()

	orig := offerURLFmt
	offerURLFmt = ts.URL + "/%s/api/offers/"
	defer func() { offerURLFmt = orig }()

	c, _ := New([]byte(`[{"name":"Acme","slug":"acme"}]`))
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("malformed body should yield no jobs, got %d", len(jobs))
	}
}

func TestSearch_EmptyTitleOrURLSkipped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(rctResponse{
			Offers: []rctOffer{
				{Title: "", CareersURL: "https://acme.recruitee.com/o/1"},
				{Title: "Go Engineer", CareersURL: ""},
				{Title: "Backend Dev", CareersURL: "https://acme.recruitee.com/o/3", City: "Paris"},
			},
		})
	}))
	defer ts.Close()

	orig := offerURLFmt
	offerURLFmt = ts.URL + "/%s/api/offers/"
	defer func() { offerURLFmt = orig }()

	c, _ := New([]byte(`[{"name":"Acme","slug":"acme"}]`))
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 valid job, got %d", len(jobs))
	}
	if jobs[0].Title != "Backend Dev" {
		t.Errorf("title = %q; want \"Backend Dev\"", jobs[0].Title)
	}
}

func TestSearch_ContextCancelled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(rctResponse{})
	}))
	defer ts.Close()

	orig := offerURLFmt
	offerURLFmt = ts.URL + "/%s/api/offers/"
	defer func() { offerURLFmt = orig }()

	c, _ := New([]byte(`[{"name":"Acme","slug":"acme"}]`))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Search(ctx, provider.SearchCriteria{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
