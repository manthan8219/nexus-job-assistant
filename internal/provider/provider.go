package provider

import (
	"context"
	"strings"
	"time"
)

// Provider is the interface every job board must implement.
type Provider interface {
	// Name returns the provider identifier e.g. "greenhouse".
	Name() string
	// Search returns jobs matching the given criteria.
	Search(ctx context.Context, c SearchCriteria) ([]Job, error)
	// Apply submits an application for the given job.
	Apply(ctx context.Context, j Job, p Profile) (ApplyResult, error)
}

// ── Shared matching helpers ─────────────────────────────────────────────────
// Used by aggregator providers (remoteok, remotive, arbeitnow, jobicy,
// hackernews, workday). Per-ATS providers keep their own local matchers.

// MatchesTitle returns true if the job title contains any of the target keywords.
func MatchesTitle(jobTitle string, keywords []string) bool {
	lower := strings.ToLower(jobTitle)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(kw))) {
			return true
		}
	}
	return false
}

// MatchesLocation returns true if a job location matches the search criteria.
// remote should be pre-computed by the provider (explicit flag or "remote"
// keyword in the location string).
func MatchesLocation(jobLocation string, remote bool, criteria SearchCriteria) bool {
	loc := strings.ToLower(jobLocation)

	// Remote jobs match if worktype is Remote or Hybrid
	if remote || strings.Contains(loc, "remote") {
		return criteria.WorkType == "Remote" || criteria.WorkType == "Hybrid"
	}

	// If user wants Remote only, non-remote jobs don't match
	if criteria.WorkType == "Remote" {
		return false
	}

	// Match against target locations
	for _, target := range criteria.Locations {
		if strings.Contains(loc, strings.ToLower(strings.TrimSpace(target))) {
			return true
		}
	}

	// No locations specified → accept all
	return len(criteria.Locations) == 0
}



// Job is a normalized job listing across all providers.
type Job struct {
	ID          string
	Title       string
	Company     string    // human-readable company name
	Board       string    // provider board token / slug
	Location    string
	Remote      bool
	URL         string    // canonical apply URL — used as dedup key
	PostedAt    time.Time
	Provider    string    // "greenhouse", "lever", etc.
	Description string    // plain-text or HTML job description (optional)
}

// SearchCriteria is built from the user's config.
type SearchCriteria struct {
	Titles    []string // keywords to match against job title
	Locations []string // target cities
	WorkType  string   // "Remote" | "Onsite" | "Hybrid"
	MinSalary int
}

// Profile is the applicant's personal data, built from config.
type Profile struct {
	FirstName  string
	LastName   string
	Email      string
	Phone      string
	ResumePath string
	LinkedInID string
	City       string
	YearsExp   string
	MinSalary  string
	Website    string
}

// ApplyResult is what comes back after an apply attempt.
type ApplyResult struct {
	Status string // "applied" | "skipped" | "failed"
	Reason string // populated when skipped or failed
}
