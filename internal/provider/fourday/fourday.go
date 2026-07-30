package fourday

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

type fdLocation struct {
	City    string `json:"city"`
	Country string `json:"country"`
}

type fdCompany struct {
	Name string `json:"name"`
}

type fdJob struct {
	Title           string       `json:"title"`
	Slug            string       `json:"slug"`
	Company         fdCompany    `json:"company"`
	Locations       []fdLocation `json:"locations"`
	WorkArrangement string       `json:"work_arrangement"`
	IsExpired       bool         `json:"is_expired"`
}

type fdResponse struct {
	Jobs    []fdJob `json:"jobs"`
	HasMore bool    `json:"has_more"`
}

// Client implements provider.Provider for 4DayWeek.
type Client struct {
	http *http.Client
}

// New creates a 4DayWeek client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "4dayweek" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	var jobs []provider.Job

	for page := 1; page <= 3; page++ {
		u := fmt.Sprintf("https://4dayweek.io/api/jobs?page=%d", page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return jobs, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

		resp, err := c.http.Do(req)
		if err != nil {
			return jobs, err
		}

		var result fdResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return jobs, fmt.Errorf("4dayweek: page %d decode: %w", page, err)
		}
		resp.Body.Close()

		for _, j := range result.Jobs {
			if j.IsExpired {
				continue
			}
			title := strings.TrimSpace(j.Title)
			if title == "" || j.Slug == "" {
				continue
			}
			jobURL := "https://4dayweek.io/job/" + j.Slug
			remote := strings.Contains(strings.ToLower(j.WorkArrangement), "remote")

			var location string
			if len(j.Locations) > 0 {
				parts := []string{}
				if j.Locations[0].City != "" {
					parts = append(parts, j.Locations[0].City)
				}
				if j.Locations[0].Country != "" {
					parts = append(parts, j.Locations[0].Country)
				}
				location = strings.Join(parts, ", ")
			}

			if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
				continue
			}
			if !matchesLocation(location, remote, criteria) {
				continue
			}
			jobs = append(jobs, provider.Job{
				Title:    title,
				Company:  j.Company.Name,
				Board:    "4dayweek",
				Location: location,
				Remote:   remote,
				URL:      jobURL,
				Provider: "4dayweek",
			})
		}

		if !result.HasMore {
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
