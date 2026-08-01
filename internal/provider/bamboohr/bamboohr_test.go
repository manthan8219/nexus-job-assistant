package bamboohr

import (
	"context"
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
		{"Engineer", []string{}, true}, // no keywords = match all
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
	if c.Name() != "bamboohr" {
		t.Errorf("Name() = %q; want \"bamboohr\"", c.Name())
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
