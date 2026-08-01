package thehub

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

func TestName(t *testing.T) {
	c := New()
	if c.Name() != "thehub" {
		t.Errorf("Name() = %q; want \"thehub\"", c.Name())
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
