package provider

import (
	"testing"
)

func TestMatchesTitle(t *testing.T) {
	cases := []struct {
		title    string
		keywords []string
		want     bool
	}{
		{"Senior Software Engineer", []string{"Software Engineer"}, true},
		{"Backend Engineer", []string{"frontend engineer"}, false},
		{"Staff Engineer, Backend", []string{"backend", "frontend"}, true},
		{"Product Manager", []string{"engineer", "developer"}, false},
		{"", []string{"engineer"}, false},
		{"Engineer", []string{}, false},
	}
	for _, c := range cases {
		got := MatchesTitle(c.title, c.keywords)
		if got != c.want {
			t.Errorf("MatchesTitle(%q, %v) = %v, want %v", c.title, c.keywords, got, c.want)
		}
	}
}

func TestMatchesLocation(t *testing.T) {
	cases := []struct {
		loc      string
		remote   bool
		criteria SearchCriteria
		want     bool
	}{
		{"Remote", true, SearchCriteria{WorkType: "Remote"}, true},
		{"Remote", true, SearchCriteria{WorkType: "Onsite"}, false},
		{"San Francisco, CA", false, SearchCriteria{WorkType: "Onsite", Locations: []string{"San Francisco"}}, true},
		{"New York, NY", false, SearchCriteria{WorkType: "Onsite", Locations: []string{"San Francisco"}}, false},
		{"Austin, TX", false, SearchCriteria{WorkType: "Onsite", Locations: []string{}}, true},
		// Remote keyword detection even when remote flag is false
		{"Remote - Worldwide", false, SearchCriteria{WorkType: "Remote"}, true},
		{"San Francisco, CA", false, SearchCriteria{WorkType: "Remote"}, false},
	}
	for _, c := range cases {
		got := MatchesLocation(c.loc, c.remote, c.criteria)
		if got != c.want {
			t.Errorf("MatchesLocation(%q, %v, wt=%s locs=%v) = %v, want %v",
				c.loc, c.remote, c.criteria.WorkType, c.criteria.Locations, got, c.want)
		}
	}
}
