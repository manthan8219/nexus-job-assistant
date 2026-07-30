package echojobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

type echoJob struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	CompanyName string   `json:"company_name"`
	Locations   []string `json:"locations"`
	RemoteType  string   `json:"remote_type"`
}

type echoResponse struct {
	Jobs []echoJob `json:"jobs"`
}

// Client implements provider.Provider for EchoJobs.
type Client struct {
	http *http.Client
}

// New creates an EchoJobs client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "echojobs" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	var jobs []provider.Job

	for page := 1; page <= 3; page++ {
		u := fmt.Sprintf("https://echojobs.io/api/jobs?page=%d&per_page=100", page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return jobs, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

		resp, err := c.http.Do(req)
		if err != nil {
			return jobs, err
		}

		var result echoResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return jobs, fmt.Errorf("echojobs: page %d decode: %w", page, err)
		}
		resp.Body.Close()

		for _, j := range result.Jobs {
			title := strings.TrimSpace(j.Title)
			jobURL := strings.TrimSpace(j.URL)
			if title == "" || jobURL == "" {
				continue
			}
			location := strings.Join(j.Locations, ", ")
			remote := j.RemoteType == "fully_remote" || j.RemoteType == "hybrid"
			if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
				continue
			}
			if !matchesLocation(location, remote, criteria) {
				continue
			}
			jobs = append(jobs, provider.Job{
				Title:    title,
				Company:  j.CompanyName,
				Board:    "echojobs",
				Location: location,
				Remote:   remote,
				URL:      jobURL,
				Provider: "echojobs",
			})
		}

		if len(result.Jobs) < 100 {
			break
		}
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
