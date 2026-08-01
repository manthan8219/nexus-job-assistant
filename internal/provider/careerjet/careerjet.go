// Package careerjet implements provider.Provider for the Careerjet public
// API (https://www.careerjet.com/partners/api/). It requires an affiliate
// id and API key from config.
//
// Careerjet is a search-only aggregator: applications happen on the posting
// site, so Apply returns "skipped" with the posting URL. Activate by setting
// provider_keys.careerjet_affid and provider_keys.careerjet_key in
// config.json.
package careerjet

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

type careerjetResponse struct {
	Type string         `json:"type"`
	Jobs []careerjetJob `json:"jobs"`
}

type careerjetJob struct {
	Title     string   `json:"title"`
	Company   string   `json:"company"`
	Locations []string `json:"locations"`
	URL       string   `json:"url"`
}

// Client implements provider.Provider for Careerjet.
type Client struct {
	http   *http.Client
	affid  string
	apiKey string
	// baseURL overrides the API host for tests; defaults to
	// https://public.api.careerjet.net.
	baseURL string
}

// New creates a Careerjet client.
func New(affid, apiKey string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		affid:   affid,
		apiKey:  apiKey,
		baseURL: "https://public.api.careerjet.net",
	}
}

func (c *Client) Name() string { return "careerjet" }

// Search queries the Careerjet API per target title and filters results by
// the search criteria.
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
	q := url.Values{}
	q.Set("keywords", title)
	q.Set("location", "")
	q.Set("affid", c.affid)
	q.Set("api_key", c.apiKey)
	q.Set("user_ip", "127.0.0.1")
	q.Set("user_agent", "nexus-job-assistant")
	q.Set("pagesize", "50")
	u := fmt.Sprintf("%s/search?%s", c.baseURL, q.Encode())

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
		return nil, fmt.Errorf("careerjet: HTTP %d", resp.StatusCode)
	}

	var result careerjetResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("careerjet: decode: %w", err)
	}
	if result.Type != "SUCCESS" {
		return nil, fmt.Errorf("careerjet: API type %q", result.Type)
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
		location := strings.Join(j.Locations, ", ")
		remote := strings.Contains(strings.ToLower(location), "remote")
		if !provider.MatchesLocation(location, remote, criteria) {
			continue
		}

		out = append(out, provider.Job{
			Title:    jobTitle,
			Company:  strings.TrimSpace(j.Company),
			Location: location,
			Remote:   remote,
			URL:      jobURL,
			Provider: "careerjet",
			Board:    "careerjet",
		})
	}
	return out, nil
}

// Apply marks as skipped — Careerjet postings link to the employer's own
// apply page.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually: " + job.URL}, nil
}
