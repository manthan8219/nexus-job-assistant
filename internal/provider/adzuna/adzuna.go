// Package adzuna implements provider.Provider for the Adzuna job board API
// (documented public API; requires an app id + key from config).
//
// Adzuna is a search-only aggregator: every posting redirects to the
// employer's own apply page, so Apply always returns "skipped" with the
// redirect URL. Activate by setting provider_keys.adzuna_id and
// provider_keys.adzuna_key in config.json.
package adzuna

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

// adzunaResult is the top-level search response.
type adzunaResult struct {
	Results []adzunaJob `json:"results"`
	Count   int         `json:"count"`
}

// adzunaJob is one normalized posting from the API.
type adzunaJob struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Company     adzunaCompany  `json:"company"`
	Location    adzunaLocation `json:"location"`
	RedirectURL string         `json:"redirect_url"`
	SalaryMin   int            `json:"salary_min"`
	SalaryMax   int            `json:"salary_max"`
	Created     string         `json:"created"`
}

type adzunaCompany struct {
	DisplayName string `json:"display_name"`
}

type adzunaLocation struct {
	Area []string `json:"area"`
}

// Client implements provider.Provider for Adzuna.
type Client struct {
	http    *http.Client
	appID   string
	appKey  string
	country string
	// baseURL overrides the API host for tests; defaults to
	// https://api.adzuna.com.
	baseURL string
}

// New creates an Adzuna client. country defaults to "gb" when empty.
func New(appID, appKey, country string) *Client {
	if country == "" {
		country = "gb"
	}
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		appID:   appID,
		appKey:  appKey,
		country: country,
		baseURL: "https://api.adzuna.com",
	}
}

func (c *Client) Name() string { return "adzuna" }

// Search queries the Adzuna API per target title (bounded concurrency) and
// filters results by the search criteria.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	titles := criteria.Titles
	if len(titles) == 0 {
		titles = []string{""}
	}

	const maxPages = 2
	const perPage = 50

	var jobs []provider.Job
	for _, title := range titles {
		select {
		case <-ctx.Done():
			return jobs, ctx.Err()
		default:
		}

		for page := 1; page <= maxPages; page++ {
			pageJobs, err := c.fetchPage(ctx, title, page, perPage, criteria)
			if err != nil {
				// One query failing must never abort the run (§10).
				break
			}
			jobs = append(jobs, pageJobs...)
			if len(pageJobs) < perPage {
				break
			}
		}
	}
	return jobs, nil
}

func (c *Client) fetchPage(ctx context.Context, title string, page, perPage int, criteria provider.SearchCriteria) ([]provider.Job, error) {
	q := url.Values{}
	q.Set("app_id", c.appID)
	q.Set("app_key", c.appKey)
	q.Set("what", title)
	q.Set("results_per_page", fmt.Sprintf("%d", perPage))
	q.Set("content-type", "application/json")
	u := fmt.Sprintf("%s/v1/api/jobs/%s/search/%d?%s", c.baseURL, c.country, page, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
		return nil, fmt.Errorf("adzuna: HTTP %d", resp.StatusCode)
	}

	var result adzunaResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("adzuna: decode: %w", err)
	}

	var out []provider.Job
	for _, j := range result.Results {
		jobTitle := strings.TrimSpace(j.Title)
		redirect := strings.TrimSpace(j.RedirectURL)
		if jobTitle == "" || redirect == "" {
			continue
		}

		if len(criteria.Titles) > 0 && !provider.MatchesTitle(jobTitle, criteria.Titles) {
			continue
		}
		location := strings.Join(j.Location.Area, ", ")
		remote := strings.Contains(strings.ToLower(location), "remote")
		if !provider.MatchesLocation(location, remote, criteria) {
			continue
		}
		// Salary floor: skip when the board reports a known ceiling below the
		// candidate's minimum.
		if criteria.MinSalary > 0 && j.SalaryMax > 0 && j.SalaryMax < criteria.MinSalary {
			continue
		}

		out = append(out, provider.Job{
			ID:       j.ID,
			Title:    jobTitle,
			Company:  j.Company.DisplayName,
			Location: location,
			Remote:   remote,
			URL:      redirect,
			Provider: "adzuna",
			Board:    "adzuna",
		})
	}
	return out, nil
}

// Apply marks as skipped — Adzuna postings redirect to the employer's own
// apply page, so there is no programmatic apply API.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually: " + job.URL}, nil
}
