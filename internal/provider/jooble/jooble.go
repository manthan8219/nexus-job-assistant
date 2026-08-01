// Package jooble implements provider.Provider for the Jooble job board API
// (https://jooble.org/api). It requires an API key from config.
//
// Jooble is a search-only aggregator: applications happen on the posting
// site, so Apply returns "skipped" with the posting link. Activate by
// setting provider_keys.jooble in config.json.
package jooble

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

type joobleResponse struct {
	TotalCount int         `json:"totalCount"`
	Jobs       []joobleJob `json:"jobs"`
}

type joobleJob struct {
	Title    string `json:"title"`
	Company  string `json:"company"`
	Location string `json:"location"`
	Link     string `json:"link"`
}

// Client implements provider.Provider for Jooble.
type Client struct {
	http   *http.Client
	apiKey string
	// baseURL overrides the API host for tests; defaults to
	// https://jooble.org.
	baseURL string
}

// New creates a Jooble client.
func New(apiKey string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		apiKey:  apiKey,
		baseURL: "https://jooble.org",
	}
}

func (c *Client) Name() string { return "jooble" }

// Search queries the Jooble API per target title and filters results by the
// search criteria.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	titles := criteria.Titles
	if len(titles) == 0 {
		titles = []string{""}
	}

	var jobs []provider.Job
	for _, title := range titles {
		select {
		case <-ctx.Done():
			return jobs, ctx.Err()
		default:
		}
		pageJobs, err := c.fetchQuery(ctx, title, criteria)
		if err != nil {
			// One query failing must never abort the run (§10).
			continue
		}
		jobs = append(jobs, pageJobs...)
	}
	return jobs, nil
}

func (c *Client) fetchQuery(ctx context.Context, title string, criteria provider.SearchCriteria) ([]provider.Job, error) {
	body, err := json.Marshal(map[string]string{"keywords": title, "location": ""})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/"+c.apiKey, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jooble: HTTP %d", resp.StatusCode)
	}

	var result joobleResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("jooble: decode: %w", err)
	}

	var out []provider.Job
	for _, j := range result.Jobs {
		jobTitle := strings.TrimSpace(j.Title)
		link := strings.TrimSpace(j.Link)
		if jobTitle == "" || link == "" {
			continue
		}

		if len(criteria.Titles) > 0 && !provider.MatchesTitle(jobTitle, criteria.Titles) {
			continue
		}
		location := strings.TrimSpace(j.Location)
		remote := strings.Contains(strings.ToLower(location), "remote")
		if !provider.MatchesLocation(location, remote, criteria) {
			continue
		}

		out = append(out, provider.Job{
			Title:    jobTitle,
			Company:  strings.TrimSpace(j.Company),
			Location: location,
			Remote:   remote,
			URL:      link,
			Provider: "jooble",
			Board:    "jooble",
		})
	}
	return out, nil
}

// Apply marks as skipped — Jooble postings link to the employer's own apply
// page.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually: " + job.URL}, nil
}
