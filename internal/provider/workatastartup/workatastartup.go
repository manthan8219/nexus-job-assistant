// Package workatastartup implements provider.Provider for Work at a Startup
// (workatastartup.com), Y Combinator's startup jobs board.
//
// YC has no official public API and full job listings require a logged-in
// session. Search routes through the local scraper service with
// use_session=true: Playwright connects to the user's already-running Chrome
// (launched with --remote-debugging-port=9222) so the logged-in YC session
// renders the job list.
//
// Work at a Startup is a search-only board: Apply always returns "skipped"
// with the posting URL.
//
// Requires: (1) the scraper service installed and running, AND (2) Chrome
// launched with --remote-debugging-port=9222 and logged into YC.
package workatastartup

import (
	"context"
	"fmt"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

const searchURL = "https://www.workatastartup.com/jobs"

// scrapeFn is the injected board-scrape dependency.
type scrapeFn func(ctx context.Context, url, company string, kws []string, useSession bool) ([]scraper.BoardJob, error)

// Client implements provider.Provider for Work at a Startup.
type Client struct {
	scrape scrapeFn
}

// New creates a Work at a Startup client.
func New() *Client {
	return &Client{scrape: scraper.ScrapeBoard}
}

func (c *Client) Name() string { return "workatastartup" }

// Search scrapes the Work at a Startup jobs page via the scraper service using
// the user's logged-in Chrome session (CDP) and filters results by criteria.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	keywords := criteria.Titles
	if len(keywords) == 0 {
		keywords = []string{""}
	}

	var jobs []provider.Job
	seen := make(map[string]bool)
	for _, kw := range keywords {
		select {
		case <-ctx.Done():
			return jobs, ctx.Err()
		default:
		}
		u := searchURL
		if strings.TrimSpace(kw) != "" {
			u = fmt.Sprintf("%s?query=%s", searchURL, strings.ReplaceAll(strings.TrimSpace(kw), " ", "+"))
		}
		boardJobs, err := c.scrape(ctx, u, "", criteria.Titles, true)
		if err != nil {
			continue
		}
		for _, j := range boardJobs {
			pj := toProviderJob(j, c.Name())
			if pj == nil {
				continue
			}
			if len(criteria.Titles) > 0 && !provider.MatchesTitle(pj.Title, criteria.Titles) {
				continue
			}
			if !provider.MatchesLocation(pj.Location, pj.Remote, criteria) {
				continue
			}
			if seen[pj.URL] {
				continue
			}
			seen[pj.URL] = true
			jobs = append(jobs, *pj)
		}
	}
	return jobs, nil
}

// toProviderJob converts a scraper board job to the shared Job type.
func toProviderJob(j scraper.BoardJob, providerName string) *provider.Job {
	title := strings.TrimSpace(j.Title)
	url := strings.TrimSpace(j.ApplyURL)
	if title == "" || url == "" {
		return nil
	}
	company := strings.TrimSpace(j.Company)
	if company == "" {
		company = "YC Startup"
	}
	return &provider.Job{
		ID:       url,
		Title:    title,
		Company:  company,
		Location: strings.TrimSpace(j.Location),
		Remote:   j.Remote,
		URL:      url,
		Provider: providerName,
		Board:    "workatastartup",
	}
}

// Apply marks as skipped — Work at a Startup postings link to the posting page.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually at " + job.URL}, nil
}
