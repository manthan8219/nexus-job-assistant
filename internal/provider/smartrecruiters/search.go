package smartrecruiters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// fetchPostings fetches all open postings for a company from SmartRecruiters.
func fetchPostings(ctx context.Context, client *http.Client, identifier string) ([]srPosting, error) {
	url := fmt.Sprintf("%s/v1/companies/%s/postings?limit=100", baseURL, identifier)

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
		return nil, fmt.Errorf("smartrecruiters API %s: HTTP %d", identifier, resp.StatusCode)
	}

	var result srResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("smartrecruiters API %s: decode: %w", identifier, err)
	}
	return result.Content, nil
}

// matchesTitle returns true if the job name contains any of the target keywords.
func matchesTitle(jobName string, keywords []string) bool {
	lower := strings.ToLower(jobName)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(kw))) {
			return true
		}
	}
	return false
}

// matchesLocation returns true if the job location matches the search criteria.
func matchesLocation(p srPosting, criteria provider.SearchCriteria) bool {
	if criteria.WorkType == "Remote" {
		return p.Location.Remote
	}
	if criteria.WorkType == "Hybrid" && p.Location.Remote {
		return true
	}

	loc := strings.ToLower(p.Location.FullLocation)
	city := strings.ToLower(p.Location.City)

	for _, target := range criteria.Locations {
		t := strings.ToLower(strings.TrimSpace(target))
		if strings.Contains(loc, t) || strings.Contains(city, t) {
			return true
		}
	}

	return len(criteria.Locations) == 0
}

// toProviderJob converts a SmartRecruiters posting to the shared Job type.
func toProviderJob(p srPosting, company srCompanyEntry) provider.Job {
	applyURL := fmt.Sprintf("https://jobs.smartrecruiters.com/%s/%s", company.Identifier, p.ID)

	var postedAt time.Time
	if p.ReleasedDate != "" {
		postedAt, _ = time.Parse("2006-01-02T15:04:05.999Z", p.ReleasedDate)
	}

	location := p.Location.FullLocation
	if location == "" {
		location = p.Location.City
	}

	return provider.Job{
		ID:       p.ID,
		Title:    p.Name,
		Company:  company.Name,
		Board:    company.Identifier,
		Location: location,
		Remote:   p.Location.Remote,
		URL:      applyURL,
		PostedAt: postedAt,
		Provider: "smartrecruiters",
	}
}
