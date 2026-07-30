package themuse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

type museLocation struct {
	Name string `json:"name"`
}

type museRefs struct {
	LandingPage string `json:"landing_page"`
}

type museCompany struct {
	Name string `json:"name"`
}

type museJob struct {
	Name      string         `json:"name"`
	Refs      museRefs       `json:"refs"`
	Company   museCompany    `json:"company"`
	Locations []museLocation `json:"locations"`
}

type museResponse struct {
	Results   []museJob `json:"results"`
	Page      int       `json:"page"`
	PageCount int       `json:"page_count"`
}

// Client implements provider.Provider for The Muse.
type Client struct {
	http *http.Client
}

// New creates a The Muse client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "themuse" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	var jobs []provider.Job

	for page := 0; page <= 2; page++ {
		u := fmt.Sprintf("https://www.themuse.com/api/public/jobs?page=%d", page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return jobs, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

		resp, err := c.http.Do(req)
		if err != nil {
			return jobs, err
		}

		var result museResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return jobs, fmt.Errorf("themuse: page %d decode: %w", page, err)
		}
		resp.Body.Close()

		for _, j := range result.Results {
			title := strings.TrimSpace(j.Name)
			jobURL := strings.TrimSpace(j.Refs.LandingPage)
			if title == "" || jobURL == "" {
				continue
			}
			var location string
			if len(j.Locations) > 0 {
				location = j.Locations[0].Name
			}
			if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
				continue
			}
			if !matchesLocation(location, false, criteria) {
				continue
			}
			jobs = append(jobs, provider.Job{
				Title:    title,
				Company:  j.Company.Name,
				Board:    "themuse",
				Location: location,
				Remote:   false,
				URL:      jobURL,
				Provider: "themuse",
			})
		}

		if page >= result.PageCount-1 {
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
