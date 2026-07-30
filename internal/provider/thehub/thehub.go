package thehub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

type hubLocation struct {
	Locality string `json:"locality"`
	Country  string `json:"country"`
}

type hubCompany struct {
	Name string `json:"name"`
}

type hubJob struct {
	Title          string      `json:"title"`
	AbsoluteJobURL string      `json:"absoluteJobUrl"`
	Company        hubCompany  `json:"company"`
	Location       hubLocation `json:"location"`
	IsRemote       bool        `json:"isRemote"`
}

type hubResponse struct {
	Docs  []hubJob `json:"docs"`
	Pages int      `json:"pages"`
}

// Client implements provider.Provider for TheHub.
type Client struct {
	http *http.Client
}

// New creates a TheHub client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "thehub" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	var jobs []provider.Job

	for page := 1; page <= 3; page++ {
		u := fmt.Sprintf("https://thehub.io/api/jobs?page=%d", page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return jobs, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

		resp, err := c.http.Do(req)
		if err != nil {
			return jobs, err
		}

		var result hubResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return jobs, fmt.Errorf("thehub: page %d decode: %w", page, err)
		}
		resp.Body.Close()

		for _, j := range result.Docs {
			title := strings.TrimSpace(j.Title)
			jobURL := strings.TrimSpace(j.AbsoluteJobURL)
			if title == "" || jobURL == "" {
				continue
			}
			parts := []string{}
			if j.Location.Locality != "" {
				parts = append(parts, j.Location.Locality)
			}
			if j.Location.Country != "" {
				parts = append(parts, j.Location.Country)
			}
			location := strings.Join(parts, ", ")

			if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
				continue
			}
			if !matchesLocation(location, j.IsRemote, criteria) {
				continue
			}
			jobs = append(jobs, provider.Job{
				Title:    title,
				Company:  j.Company.Name,
				Board:    "thehub",
				Location: location,
				Remote:   j.IsRemote,
				URL:      jobURL,
				Provider: "thehub",
			})
		}

		if page >= result.Pages {
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
