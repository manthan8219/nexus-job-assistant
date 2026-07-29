package nofluffjobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

type nfjPlace struct {
	City string `json:"city"`
}

type nfjLocation struct {
	Places []nfjPlace `json:"places"`
}

type nfjPosting struct {
	URL          string      `json:"url"`
	Title        string      `json:"title"`
	Name         string      `json:"name"`
	Location     nfjLocation `json:"location"`
	FullyRemote  bool        `json:"fullyRemote"`
}

type nfjResponse struct {
	Postings []nfjPosting `json:"postings"`
}

type nfjRequest struct {
	CriteriaSearch map[string]interface{} `json:"criteriaSearch"`
	Page           int                    `json:"page"`
	PageSize       int                    `json:"pageSize"`
}

// Client implements provider.Provider for NoFluffJobs.
type Client struct {
	http *http.Client
}

// New creates a NoFluffJobs client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "nofluffjobs" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	var jobs []provider.Job

	for page := 1; page <= 3; page++ {
		body := nfjRequest{
			CriteriaSearch: map[string]interface{}{
				"requirement": []interface{}{},
				"remote":      "remote",
			},
			Page:     page,
			PageSize: 20,
		}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return jobs, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://nofluffjobs.com/api/search/posting", bytes.NewReader(bodyBytes))
		if err != nil {
			return jobs, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return jobs, err
		}

		var result nfjResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return jobs, fmt.Errorf("nofluffjobs: page %d decode: %w", page, err)
		}
		resp.Body.Close()

		if len(result.Postings) == 0 {
			break
		}

		for _, p := range result.Postings {
			title := strings.TrimSpace(p.Title)
			postingURL := "https://nofluffjobs.com" + p.URL
			if title == "" || p.URL == "" {
				continue
			}

			var location string
			if len(p.Location.Places) > 0 {
				location = p.Location.Places[0].City
			}

			if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
				continue
			}
			if !matchesLocation(location, p.FullyRemote, criteria) {
				continue
			}
			jobs = append(jobs, provider.Job{
				Title:    title,
				Company:  p.Name,
				Board:    "nofluffjobs",
				Location: location,
				Remote:   p.FullyRemote,
				URL:      postingURL,
				Provider: "nofluffjobs",
			})
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
