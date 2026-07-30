package jobicy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

const browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

var feedURL = "https://jobicy.com/api/v2/remote-jobs?count=50"

// fetchJobs pulls the Jobicy remote jobs feed.
func fetchJobs(ctx context.Context, client *http.Client) ([]jcyJob, error) {
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
		return nil, fmt.Errorf("jobicy API: HTTP %d", resp.StatusCode)
	}

	var body jcyResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("jobicy API: decode: %w", err)
	}
	return body.Jobs, nil
}

// toProviderJob converts a Jobicy job to the shared Job type.
func toProviderJob(j jcyJob) *provider.Job {
	title := strings.TrimSpace(j.Title)
	if title == "" {
		return nil
	}

	// Validate URL — must be https://jobicy.com or https://www.jobicy.com.
	rawURL := strings.TrimSpace(j.URL)
	if rawURL == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || (parsed.Host != "jobicy.com" && parsed.Host != "www.jobicy.com") {
		return nil
	}

	company := strings.TrimSpace(j.Company)
	if company == "" {
		company = "Jobicy"
	}

	var postedAt time.Time
	if t, err := time.Parse(time.RFC3339, j.PubDate); err == nil {
		postedAt = t
	} else if t, err := time.Parse("2006-01-02T15:04:05", j.PubDate); err == nil {
		postedAt = t
	} else if t, err := time.Parse("2006-01-02", j.PubDate); err == nil {
		postedAt = t
	}

	return &provider.Job{
		ID:          fmt.Sprintf("%d", j.ID),
		Title:       title,
		Company:     company,
		Board:       "jobicy",
		Location:    strings.TrimSpace(j.Geo),
		Remote:      true, // Jobicy is a remote-only aggregator
		URL:         parsed.String(),
		PostedAt:    postedAt,
		Provider:    "jobicy",
		Description: j.Description,
	}
}
