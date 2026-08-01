package himalayas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		{"onsite fails Remote filter", "Berlin", false, provider.SearchCriteria{WorkType: "Remote"}, false},
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

func TestName(t *testing.T) {
	c := New()
	if c.Name() != "himalayas" {
		t.Errorf("Name() = %q; want \"himalayas\"", c.Name())
	}
}

func TestSearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := himResponse{
			Jobs: []himJob{
				{Title: "Go Engineer", ApplicationURL: "https://himalayas.app/job/1",
					Company: struct {
						Name string `json:"name"`
					}{Name: "Acme"}, Locations: []string{"Berlin"}, IsRemote: false, PubDate: 1722000000},
				{Title: "Frontend Dev", ApplicationURL: "https://himalayas.app/job/2",
					Company: struct {
						Name string `json:"name"`
					}{Name: "Web Inc"}, Locations: []string{"Remote"}, IsRemote: true, PubDate: 1722000000},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	orig := apiURL
	apiURL = ts.URL
	defer func() { apiURL = orig }()

	c := New()
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Go"}})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Title != "Go Engineer" {
		t.Errorf("title = %q; want \"Go Engineer\"", jobs[0].Title)
	}
	if jobs[0].Company != "Acme" {
		t.Errorf("company = %q; want \"Acme\"", jobs[0].Company)
	}
	if jobs[0].Provider != "himalayas" {
		t.Errorf("provider = %q; want \"himalayas\"", jobs[0].Provider)
	}
}

func TestSearch_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	orig := apiURL
	apiURL = ts.URL
	defer func() { apiURL = orig }()

	c := New()
	_, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestSearch_MalformedBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{not json"))
	}))
	defer ts.Close()

	orig := apiURL
	apiURL = ts.URL
	defer func() { apiURL = orig }()

	c := New()
	_, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err == nil {
		t.Fatal("expected decode error for malformed body")
	}
}

func TestSearch_EmptyTitleAndURLSkipped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := himResponse{
			Jobs: []himJob{
				{Title: "", ApplicationURL: "https://himalayas.app/job/1"},
				{Title: "Go Engineer", ApplicationURL: ""},
				{Title: "Valid", ApplicationURL: "https://himalayas.app/job/3"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	orig := apiURL
	apiURL = ts.URL
	defer func() { apiURL = orig }()

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
		json.NewEncoder(w).Encode(himResponse{})
	}))
	defer ts.Close()

	orig := apiURL
	apiURL = ts.URL
	defer func() { apiURL = orig }()

	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Search(ctx, provider.SearchCriteria{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSearch_PostedAt(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := himResponse{
			Jobs: []himJob{
				{Title: "Go Engineer", ApplicationURL: "https://himalayas.app/job/1",
					Company: struct {
						Name string `json:"name"`
					}{Name: "Acme"}, Locations: []string{"Berlin"}, IsRemote: false, PubDate: 1722000000},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	orig := apiURL
	apiURL = ts.URL
	defer func() { apiURL = orig }()

	c := New()
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	expected := time.Unix(1722000000, 0).UTC()
	if !jobs[0].PostedAt.Equal(expected) {
		t.Errorf("PostedAt = %v; want %v", jobs[0].PostedAt, expected)
	}
}
