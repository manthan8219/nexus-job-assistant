// Package talent implements provider.Provider for the Talent.com public job
// search API (https://developer.talent.com). Talent.com (formerly Neuvoo) is a
// global aggregator covering the US, Europe, LatAm, Asia, and India in a single
// source.
//
// The API requires a free API key. Activate by setting provider_keys.talent in
// config.json.
//
// Talent.com is a search-only aggregator: every posting links to the
// employer's apply page, so Apply always returns "skipped" with the posting URL.
package talent

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

// talentResponse is the top-level search response.
type talentResponse struct {
	Jobs []talentJob `json:"jobs"`
}

// talentJob is one normalized posting from the API.
type talentJob struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	Location    string `json:"location"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Posted      string `json:"posted"`
	Remote      bool   `json:"remote"`
	SalaryMin   int    `json:"salary_min"`
	SalaryMax   int    `json:"salary_max"`
}

// Client implements provider.Provider for Talent.com.
type Client struct {
	http   *http.Client
	apiKey string
	// baseURL overrides the API host for tests; defaults to
	// https://www.talent.com.
	baseURL string
}

// New creates a Talent.com client. apiKey is the Talent.com developer API key.
func New(apiKey string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		apiKey:  apiKey,
		baseURL: "https://www.talent.com",
	}
}

func (c *Client) Name() string { return "talent" }

// Search queries the Talent.com API per target title and filters results by
// the search criteria. One title failing must never abort the run (§10).
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	titles := criteria.Titles
	if len(titles) == 0 {
		titles = []string{""}
	}

	location := ""
	if len(criteria.Locations) > 0 {
		location = criteria.Locations[0]
	}
	if criteria.WorkType == "Remote" {
		location = "Remote"
	}

	const perPage = 30
	const maxPages = 3

	var jobs []provider.Job
	for _, title := range titles {
		select {
		case <-ctx.Done():
			return jobs, ctx.Err()
		default:
		}
		for page := 1; page <= maxPages; page++ {
			pageJobs, err := c.fetchPage(ctx, title, location, page, perPage, criteria)
			if err != nil {
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

func (c *Client) fetchPage(ctx context.Context, title, location string, page, perPage int, criteria provider.SearchCriteria) ([]provider.Job, error) {
	q := url.Values{}
	q.Set("key", c.apiKey)
	q.Set("jobtitle", title)
	if location != "" {
		q.Set("location", location)
	}
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("num", fmt.Sprintf("%d", perPage))
	u := fmt.Sprintf("%s/api/v1/jobs?%s", c.baseURL, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("talent: HTTP %d", resp.StatusCode)
	}

	var result talentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("talent: decode: %w", err)
	}

	var out []provider.Job
	for _, j := range result.Jobs {
		jobTitle := strings.TrimSpace(j.Title)
		jobURL := strings.TrimSpace(j.URL)
		if jobTitle == "" || jobURL == "" {
			continue
		}

		if len(criteria.Titles) > 0 && !provider.MatchesTitle(jobTitle, criteria.Titles) {
			continue
		}
		locationName := strings.TrimSpace(j.Location)
		remote := j.Remote || strings.Contains(strings.ToLower(locationName), "remote")
		if !provider.MatchesLocation(locationName, remote, criteria) {
			continue
		}
		if criteria.MinSalary > 0 && j.SalaryMax > 0 && j.SalaryMax < criteria.MinSalary {
			continue
		}

		postedAt, _ := time.Parse("2006-01-02", j.Posted)
		out = append(out, provider.Job{
			ID:          strings.TrimSpace(j.ID),
			Title:       jobTitle,
			Company:     strings.TrimSpace(j.Company),
			Location:    locationName,
			Remote:      remote,
			URL:         jobURL,
			PostedAt:    postedAt,
			Provider:    "talent",
			Board:       "talent",
			Description: strings.TrimSpace(j.Description),
		})
	}
	return out, nil
}

// Apply marks as skipped — Talent.com postings redirect to the employer's own
// apply page, so there is no programmatic apply API.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually: " + job.URL}, nil
}
