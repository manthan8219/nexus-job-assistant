package ashby

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

const jobBoardQuery = `query ApiJobBoardWithTeams($organizationHostedJobsPageName: String!) { jobBoard: jobBoardWithTeams(organizationHostedJobsPageName: $organizationHostedJobsPageName) { jobPostings { id title locationName isRemote } } }`

type graphQLRequest struct {
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
	Query         string                 `json:"query"`
}

// fetchPostings queries the Ashby GraphQL API for a company's open job postings.
func fetchPostings(ctx context.Context, client *http.Client, slug string) ([]ashbyJobPosting, error) {
	payload := graphQLRequest{
		OperationName: "ApiJobBoardWithTeams",
		Variables: map[string]interface{}{
			"organizationHostedJobsPageName": slug,
		},
		Query: jobBoardQuery,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseGraphQLURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
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
		return nil, fmt.Errorf("ashby GraphQL %s: HTTP %d", slug, resp.StatusCode)
	}

	var result ashbyGraphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ashby GraphQL %s: decode: %w", slug, err)
	}
	return result.Data.JobBoard.JobPostings, nil
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
func matchesLocation(p ashbyJobPosting, criteria provider.SearchCriteria) bool {
	// Remote match: if worktype is Remote and posting is remote
	if criteria.WorkType == "Remote" {
		return p.IsRemote
	}
	if criteria.WorkType == "Hybrid" && p.IsRemote {
		return true
	}

	loc := strings.ToLower(p.LocationName)

	// Check against target locations
	for _, target := range criteria.Locations {
		if strings.Contains(loc, strings.ToLower(strings.TrimSpace(target))) {
			return true
		}
	}

	// No locations specified → accept all
	return len(criteria.Locations) == 0
}

// toProviderJob converts an Ashby job posting to the shared Job type.
func toProviderJob(p ashbyJobPosting, company ashbyCompany) provider.Job {
	applyURL := fmt.Sprintf("https://jobs.ashbyhq.com/%s/%s/application", company.Slug, p.ID)

	return provider.Job{
		ID:       p.ID,
		Title:    p.Title,
		Company:  company.Name,
		Board:    company.Slug,
		Location: p.LocationName,
		Remote:   p.IsRemote,
		URL:      applyURL,
		Provider: "ashby",
	}
}
