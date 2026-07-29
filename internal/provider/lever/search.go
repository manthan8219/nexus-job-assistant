package lever

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/manthanmanthan/nexus/internal/provider"
)

const leverBaseURL = "https://api.lever.co/v0/postings"

// fetchPostings queries the Lever REST API for a company's open job postings.
func fetchPostings(ctx context.Context, client *http.Client, slug string) ([]leverPosting, error) {
	url := fmt.Sprintf("%s/%s?mode=json", leverBaseURL, slug)

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
		return nil, fmt.Errorf("lever REST %s: HTTP %d", slug, resp.StatusCode)
	}

	var postings []leverPosting
	if err := json.NewDecoder(resp.Body).Decode(&postings); err != nil {
		return nil, fmt.Errorf("lever REST %s: decode: %w", slug, err)
	}
	return postings, nil
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
func matchesLocation(p leverPosting, criteria provider.SearchCriteria) bool {
	locLower := strings.ToLower(p.Categories.Location)
	isRemote := strings.Contains(locLower, "remote")

	// Remote match
	if criteria.WorkType == "Remote" {
		return isRemote
	}
	if criteria.WorkType == "Hybrid" && isRemote {
		return true
	}

	// Check against target locations
	for _, target := range criteria.Locations {
		if strings.Contains(locLower, strings.ToLower(strings.TrimSpace(target))) {
			return true
		}
	}

	// No locations specified → accept all
	return len(criteria.Locations) == 0
}

// toProviderJob converts a Lever job posting to the shared Job type.
func toProviderJob(p leverPosting, company leverCompany) provider.Job {
	applyURL := p.ApplyURL
	if applyURL == "" {
		applyURL = p.HostedURL
	}

	desc := strings.TrimSpace(p.DescriptionPlain)
	if desc == "" {
		desc = strings.TrimSpace(p.DescriptionHTML)
	}
	if add := strings.TrimSpace(p.AdditionalPlain); add != "" {
		if desc != "" {
			desc += "\n\n" + add
		} else {
			desc = add
		}
	}
	return provider.Job{
		ID:          p.ID,
		Title:       p.Text,
		Company:     company.Name,
		Board:       company.Slug,
		Location:    p.Categories.Location,
		Remote:      strings.Contains(strings.ToLower(p.Categories.Location), "remote"),
		URL:         applyURL,
		Provider:    "lever",
		Description: desc,
	}
}
