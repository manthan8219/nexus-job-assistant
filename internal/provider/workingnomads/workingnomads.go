package workingnomads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

const apiURL = "https://www.workingnomads.com/api/exposed_jobs/"

type wnJob struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	CompanyName string `json:"company_name"`
	Location    string `json:"location"`
}

// Client implements provider.Provider for WorkingNomads.
type Client struct {
	http *http.Client
}

// New creates a WorkingNomads client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "workingnomads" }

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
		return nil, fmt.Errorf("workingnomads API: HTTP %d", resp.StatusCode)
	}

	var rawJobs []wnJob
	if err := json.NewDecoder(resp.Body).Decode(&rawJobs); err != nil {
		return nil, fmt.Errorf("workingnomads API: decode: %w", err)
	}

	var jobs []provider.Job
	for _, j := range rawJobs {
		title := strings.TrimSpace(j.Title)
		jobURL := strings.TrimSpace(j.URL)
		if title == "" || jobURL == "" {
			continue
		}
		if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
			continue
		}
		// All workingnomads jobs are remote
		if !matchesLocation(j.Location, true, criteria) {
			continue
		}
		jobs = append(jobs, provider.Job{
			Title:    title,
			Company:  j.CompanyName,
			Board:    "workingnomads",
			Location: j.Location,
			Remote:   true,
			URL:      jobURL,
			Provider: "workingnomads",
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
