package workable

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

const workableBaseURL = "https://apply.workable.com/api/v3/accounts"

// fetchPostings queries the Workable API for a company's published job postings.
func fetchPostings(ctx context.Context, client *http.Client, slug string) ([]workableJob, error) {
	url := fmt.Sprintf("%s/%s/jobs?status=published&limit=50", workableBaseURL, slug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("workable REST %s: HTTP %d", slug, resp.StatusCode)
	}

	var result workableJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("workable REST %s: decode: %w", slug, err)
	}
	return result.Results, nil
}

// matchesTitle returns true if the job title contains any of the target keywords.
func matchesTitle(jobTitle string, keywords []string) bool {
	lower := strings.ToLower(jobTitle)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(kw))) {
			return true
		}
	}
	return false
}

// matchesLocation returns true if the job location matches the search criteria.
func matchesLocation(p workableJob, criteria provider.SearchCriteria) bool {
	// Remote match
	if criteria.WorkType == "Remote" {
		return p.Location.Remote
	}
	if criteria.WorkType == "Hybrid" && p.Location.Remote {
		return true
	}

	loc := strings.ToLower(p.Location.City + " " + p.Location.Country)

	// Check against target locations
	for _, target := range criteria.Locations {
		if strings.Contains(loc, strings.ToLower(strings.TrimSpace(target))) {
			return true
		}
	}

	// No locations specified → accept all
	return len(criteria.Locations) == 0
}

// toProviderJob converts a Workable job posting to the shared Job type.
func toProviderJob(p workableJob, company workableCompany) provider.Job {
	loc := p.Location.City
	if p.Location.Country != "" {
		if loc != "" {
			loc += ", " + p.Location.Country
		} else {
			loc = p.Location.Country
		}
	}

	applyURL := p.URL
	if applyURL == "" {
		applyURL = fmt.Sprintf("https://apply.workable.com/%s/j/%s/", company.Slug, p.Shortcode)
	}

	return provider.Job{
		ID:       p.ID,
		Title:    p.Title,
		Company:  company.Name,
		Board:    company.Slug,
		Location: loc,
		Remote:   p.Location.Remote,
		URL:      applyURL,
		Provider: "workable",
	}
}
