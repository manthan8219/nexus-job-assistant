// Package reed implements provider.Provider for the Reed.co.uk public job
// search API (https://www.reed.co.uk/developers/jobseeker).
//
// Reed is the UK's largest job site. The API is free and uses HTTP Basic
// authentication with the API key as the username and an empty password.
// Activate by setting provider_keys.reed in config.json.
//
// Reed is a search-only aggregator: applications happen on the Reed posting
// page (or the employer's external site), so Apply always returns "skipped"
// with the posting URL.
package reed

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

// reedResult is the top-level search response.
type reedResult struct {
	Results []reedJob `json:"results"`
}

// reedJob is one normalized posting from the Reed API.
type reedJob struct {
	JobID          int    `json:"jobId"`
	JobTitle       string `json:"jobTitle"`
	EmployerName   string `json:"employerName"`
	LocationName   string `json:"locationName"`
	JobDescription string `json:"jobDescription"`
	MinimumSalary  int    `json:"minimumSalary"`
	MaximumSalary  int    `json:"maximumSalary"`
	DatePosted     string `json:"datePosted"`
	JobURL         string `json:"jobUrl"`
}

// Client implements provider.Provider for Reed.co.uk.
type Client struct {
	http   *http.Client
	apiKey string
	// baseURL overrides the API host for tests; defaults to
	// https://www.reed.co.uk.
	baseURL string
}

// New creates a Reed client. apiKey is the Reed developer API key.
func New(apiKey string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		apiKey:  apiKey,
		baseURL: "https://www.reed.co.uk",
	}
}

func (c *Client) Name() string { return "reed" }

// Search queries the Reed API per target title and filters results by the
// search criteria. One title failing must never abort the run (§10).
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

	const perPage = 100
	const maxPages = 3

	var jobs []provider.Job
	for _, title := range titles {
		select {
		case <-ctx.Done():
			return jobs, ctx.Err()
		default:
		}
		for page := 0; page < maxPages; page++ {
			pageJobs, err := c.fetchPage(ctx, title, location, page*perPage, perPage, criteria)
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

func (c *Client) fetchPage(ctx context.Context, title, location string, skip, perPage int, criteria provider.SearchCriteria) ([]provider.Job, error) {
	q := url.Values{}
	q.Set("keywords", title)
	if location != "" {
		q.Set("locationName", location)
	}
	q.Set("resultsToTake", fmt.Sprintf("%d", perPage))
	q.Set("resultsToSkip", fmt.Sprintf("%d", skip))
	u := fmt.Sprintf("%s/api/1.0/search?%s", c.baseURL, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	// Reed uses HTTP Basic auth: username = API key, password = empty.
	req.SetBasicAuth(c.apiKey, "")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reed: HTTP %d", resp.StatusCode)
	}

	var result reedResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("reed: decode: %w", err)
	}

	var out []provider.Job
	for _, j := range result.Results {
		jobTitle := strings.TrimSpace(j.JobTitle)
		jobURL := strings.TrimSpace(j.JobURL)
		if jobURL == "" {
			// Fall back to the canonical Reed posting URL built from the ID.
			jobURL = fmt.Sprintf("%s/jobs/%d", c.baseURL, j.JobID)
		}
		if jobTitle == "" || jobURL == "" {
			continue
		}

		if len(criteria.Titles) > 0 && !provider.MatchesTitle(jobTitle, criteria.Titles) {
			continue
		}
		locationName := strings.TrimSpace(j.LocationName)
		remote := strings.Contains(strings.ToLower(locationName), "remote")
		if !provider.MatchesLocation(locationName, remote, criteria) {
			continue
		}
		// Salary floor: skip when the board reports a known ceiling below the
		// candidate's minimum.
		if criteria.MinSalary > 0 && j.MaximumSalary > 0 && j.MaximumSalary < criteria.MinSalary {
			continue
		}

		postedAt, _ := time.Parse(time.RFC3339, j.DatePosted)
		out = append(out, provider.Job{
			ID:       fmt.Sprintf("%d", j.JobID),
			Title:    jobTitle,
			Company:  strings.TrimSpace(j.EmployerName),
			Location: locationName,
			Remote:   remote,
			URL:      jobURL,
			PostedAt: postedAt,
			Provider: "reed",
			Board:    "reed",
		})
	}
	return out, nil
}

// Apply marks as skipped — Reed postings link to the Reed posting page (or the
// employer's external apply site), so there is no programmatic apply API.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually: " + job.URL}, nil
}
