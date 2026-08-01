// Package usajobs implements provider.Provider for the USAJOBS public API
// (https://developer.usajobs.gov). It requires an API key from config.
//
// USAJOBS is a search-only board: applications happen on the USAJOBS site
// itself, so Apply returns "skipped" with the posting URI. Activate by
// setting provider_keys.usajobs in config.json.
package usajobs

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

// searchResponse is the top-level API envelope.
type searchResponse struct {
	SearchResult struct {
		Items []searchResultItem `json:"SearchResultItems"`
	} `json:"SearchResult"`
}

type searchResultItem struct {
	Descriptor struct {
		ID               string         `json:"PositionID"`
		Title            string         `json:"PositionTitle"`
		OrganizationName string         `json:"OrganizationName"`
		Location         string         `json:"PositionLocationDisplay"`
		URI              string         `json:"PositionURI"`
		Remuneration     []remuneration `json:"PositionRemuneration"`
	} `json:"MatchedObjectDescriptor"`
}

type remuneration struct {
	MinimumRange int `json:"MinimumRange"`
	MaximumRange int `json:"MaximumRange"`
}

// Client implements provider.Provider for USAJOBS.
type Client struct {
	http   *http.Client
	apiKey string
	email  string // required in the User-Agent header by the API terms
	// baseURL overrides the API host for tests; defaults to
	// https://data.usajobs.gov.
	baseURL string
}

// New creates a USAJOBS client. email is the account address the API
// requires in the User-Agent header.
func New(apiKey, email string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		apiKey:  apiKey,
		email:   email,
		baseURL: "https://data.usajobs.gov",
	}
}

func (c *Client) Name() string { return "usajobs" }

// Search queries the USAJOBS API per target title and filters results by
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
	q.Set("Keyword", title)
	q.Set("ResultsPerPage", "50")
	q.Set("Page", "1")
	u := fmt.Sprintf("%s/api/search?%s", c.baseURL, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Host", "data.usajobs.gov")
	req.Header.Set("User-Agent", c.email)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usajobs: HTTP %d", resp.StatusCode)
	}

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("usajobs: decode: %w", err)
	}

	var out []provider.Job
	for _, item := range result.SearchResult.Items {
		d := item.Descriptor
		titleText := strings.TrimSpace(d.Title)
		uri := strings.TrimSpace(d.URI)
		if titleText == "" || uri == "" {
			continue
		}

		if len(criteria.Titles) > 0 && !provider.MatchesTitle(titleText, criteria.Titles) {
			continue
		}
		remote := strings.Contains(strings.ToLower(d.Location), "remote")
		if !provider.MatchesLocation(d.Location, remote, criteria) {
			continue
		}
		// Salary floor: skip when every reported remuneration ceiling is
		// below the candidate's minimum.
		if criteria.MinSalary > 0 {
			maxCeiling := 0
			for _, r := range d.Remuneration {
				if r.MaximumRange > maxCeiling {
					maxCeiling = r.MaximumRange
				}
			}
			if maxCeiling > 0 && maxCeiling < criteria.MinSalary {
				continue
			}
		}

		out = append(out, provider.Job{
			ID:       d.ID,
			Title:    titleText,
			Company:  d.OrganizationName,
			Location: d.Location,
			Remote:   remote,
			URL:      uri,
			Provider: "usajobs",
			Board:    "usajobs",
		})
	}
	return out, nil
}

// Apply marks as skipped — USAJOBS applications must be submitted on the
// site itself.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually: " + job.URL}, nil
}
