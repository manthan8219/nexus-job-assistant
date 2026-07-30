package jobicy

import (
	"context"
	"net/http"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// Client implements provider.Provider for Jobicy.
type Client struct {
	http *http.Client
}

// New creates a Jobicy client — a board-wide aggregator, no API key needed.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "jobicy" }

// Search fetches the Jobicy feed and filters by criteria.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	jobs, err := fetchJobs(ctx, c.http)
	if err != nil {
		return nil, err
	}

	var results []provider.Job
	for _, j := range jobs {
		pj := toProviderJob(j)
		if pj == nil {
			continue
		}
		if len(criteria.Titles) > 0 && !provider.MatchesTitle(pj.Title, criteria.Titles) {
			continue
		}
		if !provider.MatchesLocation(pj.Location, pj.Remote, criteria) {
			continue
		}
		results = append(results, *pj)
	}
	return results, nil
}

// Apply marks as skipped — jobs link to the Jobicy posting page (external).
func (c *Client) Apply(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{
		Status: "skipped",
		Reason: "apply manually at " + job.URL,
	}, nil
}
