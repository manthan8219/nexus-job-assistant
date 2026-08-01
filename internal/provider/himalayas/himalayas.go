package himalayas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

var apiURL = "https://himalayas.app/jobs/api?limit=50"

type himJob struct {
	Title          string `json:"title"`
	ApplicationURL string `json:"applicationUrl"`
	Company        struct {
		Name string `json:"name"`
	} `json:"company"`
	Locations   []string `json:"locations"`
	IsRemote    bool     `json:"isRemote"`
	Description string   `json:"description"`
	PubDate     int64    `json:"pubDate"`
}

type himResponse struct {
	Jobs []himJob `json:"jobs"`
}

// Client implements provider.Provider for Himalayas.
type Client struct {
	http *http.Client
}

// New creates a Himalayas client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "himalayas" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("himalayas API: HTTP %d", resp.StatusCode)
	}

	var result himResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("himalayas API: decode: %w", err)
	}

	var jobs []provider.Job
	for _, j := range result.Jobs {
		title := strings.TrimSpace(j.Title)
		u := strings.TrimSpace(j.ApplicationURL)
		if title == "" || u == "" {
			continue
		}
		location := strings.Join(j.Locations, ", ")
		if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
			continue
		}
		if !matchesLocation(location, j.IsRemote, criteria) {
			continue
		}
		postedAt := time.Time{}
		if j.PubDate > 0 {
			postedAt = time.Unix(j.PubDate, 0)
		}
		jobs = append(jobs, provider.Job{
			Title:       title,
			Company:     j.Company.Name,
			Board:       "himalayas",
			Location:    location,
			Remote:      j.IsRemote,
			URL:         u,
			PostedAt:    postedAt,
			Provider:    "himalayas",
			Description: j.Description,
		})
	}
	return jobs, nil
}

func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually: " + job.URL}, nil
}

func matchesTitle(title string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	lower := strings.ToLower(title)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(kw))) {
			return true
		}
	}
	return false
}

func matchesLocation(location string, remote bool, criteria provider.SearchCriteria) bool {
	if criteria.WorkType == "Remote" {
		return remote
	}
	if len(criteria.Locations) == 0 {
		return true
	}
	loc := strings.ToLower(location)
	for _, t := range criteria.Locations {
		if strings.Contains(loc, strings.ToLower(strings.TrimSpace(t))) {
			return true
		}
	}
	return false
}
