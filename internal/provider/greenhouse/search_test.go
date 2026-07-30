package greenhouse

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestFetchJobs_RealAPI(t *testing.T) {
	// Hits the real Greenhouse API — verifies the board token + response shape.
	client := &http.Client{Timeout: 15 * time.Second}

	// Notion is a stable board that reliably has open jobs.
	jobs, err := fetchJobs(context.Background(), client, "notion")
	if err != nil {
		t.Fatalf("fetchJobs: %v", err)
	}
	if len(jobs) == 0 {
		t.Log("WARNING: notion board returned 0 jobs (may have no openings right now)")
		return
	}

	t.Logf("notion: got %d jobs", len(jobs))
	j := jobs[0]
	if j.Title == "" {
		t.Error("first job has empty title")
	}
	if j.ID == 0 {
		t.Error("first job has zero ID")
	}
	t.Logf("  sample: [%d] %q @ %q", j.ID, j.Title, j.Location.Name)
}

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
		{"Engineer", []string{}, false}, // no keywords → no match
	}
	for _, c := range cases {
		got := matchesTitle(c.title, c.keywords)
		if got != c.want {
			t.Errorf("matchesTitle(%q, %v) = %v, want %v", c.title, c.keywords, got, c.want)
		}
	}
}

func TestMatchesLocation(t *testing.T) {
	cases := []struct {
		loc      string
		criteria provider.SearchCriteria
		want     bool
	}{
		{"Remote", provider.SearchCriteria{WorkType: "Remote"}, true},
		{"Remote", provider.SearchCriteria{WorkType: "Onsite"}, false},
		{"Remote", provider.SearchCriteria{WorkType: "Hybrid"}, true},
		{"San Francisco, CA", provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"San Francisco"}}, true},
		{"New York, NY", provider.SearchCriteria{WorkType: "Onsite", Locations: []string{"San Francisco"}}, false},
		{"Austin, TX", provider.SearchCriteria{WorkType: "Onsite", Locations: []string{}}, true}, // no filter = accept all
	}
	for _, c := range cases {
		got := matchesLocation(c.loc, c.criteria)
		if got != c.want {
			t.Errorf("matchesLocation(%q, wt=%s locs=%v) = %v, want %v",
				c.loc, c.criteria.WorkType, c.criteria.Locations, got, c.want)
		}
	}
}
