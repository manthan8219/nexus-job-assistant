package arbeitnow

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

var feedBase = "https://www.arbeitnow.com/api/job-board-api"

const trustedHost = "www.arbeitnow.com"

// fetchPage requests one page of the Arbeitnow API.
func fetchPage(ctx context.Context, client *http.Client, page int) ([]arbJob, error) {
	apiURL := fmt.Sprintf("%s?page=%d", feedBase, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
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
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var body arbResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return body.Data, nil
}

// normalizeJob converts an Arbeitnow job to the shared Job type.
// Returns nil when the entry is malformed or the URL is untrusted.
func normalizeJob(j arbJob) *provider.Job {
	title := strings.TrimSpace(j.Title)
	if title == "" {
		return nil
	}

	// Only trust https:// URLs on www.arbeitnow.com for SSRF safety.
	rawURL := strings.TrimSpace(j.URL)
	if rawURL == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != trustedHost {
		return nil
	}

	company := strings.TrimSpace(j.Company)
	if company == "" {
		company = "Arbeitnow"
	}

	baseLoc := strings.TrimSpace(j.Location)
	parts := []string{baseLoc}
	if j.Remote {
		parts = append(parts, "Remote")
	}
	location := strings.Join(parts, ", ")

	var postedAt time.Time
	if j.CreatedAt > 0 {
		postedAt = time.Unix(j.CreatedAt, 0)
	}

	return &provider.Job{
		ID:          j.Slug,
		Title:       title,
		Company:     company,
		Board:       "arbeitnow",
		Location:    location,
		Remote:      j.Remote,
		URL:         parsed.String(),
		PostedAt:    postedAt,
		Provider:    "arbeitnow",
		Description: j.Description,
	}
}
