package remoteok

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

// Aggregator APIs like RemoteOK reject non-browser user agents.
const browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

var feedURL = "https://remoteok.com/api"

// fetchJobs pulls the board-wide RemoteOK feed.
func fetchJobs(ctx context.Context, client *http.Client) ([]rokJob, error) {
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
		return nil, fmt.Errorf("remoteok API: HTTP %d", resp.StatusCode)
	}

	var jobs []rokJob
	if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
		return nil, fmt.Errorf("remoteok API: decode: %w", err)
	}
	return jobs, nil
}

// toProviderJob converts a RemoteOK posting to the shared Job type.
// Returns nil for the metadata entry (index 0) or malformed rows.
func toProviderJob(j rokJob) *provider.Job {
	title := strings.TrimSpace(j.Position)
	url := strings.TrimSpace(j.URL)
	if title == "" || !strings.HasPrefix(url, "http") {
		return nil
	}

	company := strings.TrimSpace(j.Company)
	if company == "" {
		company = "RemoteOK"
	}

	postedAt, _ := time.Parse(time.RFC3339, j.Date)

	return &provider.Job{
		ID:       j.ID,
		Title:    title,
		Company:  company,
		Board:    "remoteok",
		Location: strings.TrimSpace(j.Location),
		Remote:   true, // RemoteOK only lists remote jobs
		URL:      url,
		PostedAt: postedAt,
		Provider:    "remoteok",
		Description: j.Description,
	}
}
