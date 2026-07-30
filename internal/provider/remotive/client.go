package remotive

import (
	"context"
	"net/http"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// Client implements provider.Provider for Remotive.
type Client struct {
	http *http.Client
}

// New creates a Remotive client — a board-wide aggregator, no company list needed.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "remotive" }

// Search fetches the Remotive feed and filters by criteria.
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

// Apply marks the job as skipped — Remotive postings link out to third-party sites.
func (c *Client) Apply(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{
		Status: "skipped",
		Reason: "apply manually at " + job.URL,
	}, nil
}
