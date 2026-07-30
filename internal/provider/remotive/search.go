package remotive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

const browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

var feedURL = "https://remotive.com/api/remote-jobs"

// fetchJobs pulls the Remotive board-wide remote feed.
func fetchJobs(ctx context.Context, client *http.Client) ([]remJob, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remotive API: HTTP %d", resp.StatusCode)
	}

	var body remResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("remotive API: decode: %w", err)
	}
	return body.Jobs, nil
}

// toProviderJob converts a Remotive job posting to the shared Job type.
func toProviderJob(j remJob) *provider.Job {
	title := strings.TrimSpace(j.Title)
	url := strings.TrimSpace(j.URL)
	if title == "" || !strings.HasPrefix(url, "http") {
		return nil
	}

	company := strings.TrimSpace(j.Company)
	if company == "" {
		company = "Remotive"
	}

	var postedAt time.Time
	// Remotive returns "2021-11-30T15:44:12" (no timezone).
	if t, err := time.Parse("2006-01-02T15:04:05", j.PubDate); err == nil {
		postedAt = t
	} else if t, err := time.Parse(time.RFC3339, j.PubDate); err == nil {
		postedAt = t
	}

	return &provider.Job{
		ID:          fmt.Sprintf("%d", j.ID),
		Title:       title,
		Company:     company,
		Board:       "remotive",
		Location:    strings.TrimSpace(j.Location),
		Remote:      true, // Remotive is a remote-only board
		URL:         url,
		PostedAt:    postedAt,
		Provider:    "remotive",
		Description: j.Description,
	}
}
