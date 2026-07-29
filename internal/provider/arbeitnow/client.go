package arbeitnow

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

const (
	maxPages = 3   // pages to paginate through (100 per page)
	perPage  = 100
)

// Client implements provider.Provider for Arbeitnow.
type Client struct {
	http *http.Client
}

// New creates an Arbeitnow client — a board-wide aggregator, no API key needed.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "arbeitnow" }

// Search paginates the Arbeitnow feed and filters by criteria.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	// Dedup across pages by URL.
	byURL := make(map[string]provider.Job)

	for page := 1; page <= maxPages; page++ {
		select {
		case <-ctx.Done():
			return dedupValues(byURL), ctx.Err()
		default:
		}

		raw, err := fetchPage(ctx, c.http, page)
		if err != nil {
			return dedupValues(byURL), fmt.Errorf("arbeitnow page %d: %w", page, err)
		}

		for _, j := range raw {
			pj := normalizeJob(j)
			if pj == nil {
				continue
			}
			if len(criteria.Titles) > 0 && !provider.MatchesTitle(pj.Title, criteria.Titles) {
				continue
			}
			if !provider.MatchesLocation(pj.Location, pj.Remote, criteria) {
				continue
			}
			byURL[pj.URL] = *pj
		}

		// Short page → last page reached.
		if len(raw) < perPage {
			break
		}

		// Polite delay between pages.
		time.Sleep(300 * time.Millisecond)
	}

	return dedupValues(byURL), nil
}

// Apply marks as skipped — jobs link to the Arbeitnow posting page.
func (c *Client) Apply(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{
		Status: "skipped",
		Reason: "apply manually at " + job.URL,
	}, nil
}

func dedupValues(m map[string]provider.Job) []provider.Job {
	out := make([]provider.Job, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
