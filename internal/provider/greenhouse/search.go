package greenhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/textutil"
)

const baseURL = "https://boards-api.greenhouse.io/v1/boards"

// fetchJobs pulls all open jobs for a company board.
func fetchJobs(ctx context.Context, client *http.Client, board string) ([]ghJob, error) {
	url := fmt.Sprintf("%s/%s/jobs?content=true", baseURL, board)

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
		return nil, nil // board token invalid or no open jobs
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("greenhouse API %s: HTTP %d", board, resp.StatusCode)
	}

	var result jobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("greenhouse API %s: decode: %w", board, err)
	}
	return result.Jobs, nil
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

// matchesLocation returns true if the job location matches criteria.
func matchesLocation(jobLocation string, criteria provider.SearchCriteria) bool {
	loc := strings.ToLower(jobLocation)

	// Remote jobs match if worktype is Remote or Hybrid
	if strings.Contains(loc, "remote") {
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

// toProviderJob converts a Greenhouse job to the shared Job type.
func toProviderJob(j ghJob, company Company) provider.Job {
	isRemote := strings.Contains(strings.ToLower(j.Location.Name), "remote")
	postedAt, _ := time.Parse(time.RFC3339, j.UpdatedAt)

	return provider.Job{
		ID:          fmt.Sprintf("%d", j.ID),
		Title:       j.Title,
		Company:     company.Name,
		Board:       company.Board,
		Location:    j.Location.Name,
		Remote:      isRemote,
		URL:         fmt.Sprintf("%s/%s/jobs/%d", baseURL, company.Board, j.ID),
		PostedAt:    postedAt,
		Provider:    "greenhouse",
		Description: textutil.HTMLToPlain(j.Content),
	}
}
