// Package landingjobs implements provider.Provider for Landing.Jobs
// (landing.jobs), a European tech job board with remote, hybrid, and on-site
// roles across Portugal, the EU, and beyond.
//
// Landing.Jobs loads jobs via an internal JSON endpoint after page render, so
// Search routes through the local scraper service (Playwright renders the
// page, generic extraction pulls job links). No login is required to browse.
//
// Landing.Jobs is a search-only board: Apply always returns "skipped" with
// the posting URL.
//
// Requires the scraper service to be installed and running (Settings › Career
// Scraper).
package landingjobs

import (
	"context"
	"fmt"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

const searchURL = "https://landing.jobs/jobs"

// scrapeFn is the injected board-scrape dependency.
type scrapeFn func(ctx context.Context, url, company string, kws []string, useSession bool) ([]scraper.BoardJob, error)

// Client implements provider.Provider for Landing.Jobs.
type Client struct {
	scrape scrapeFn
}

// New creates a Landing.Jobs client.
func New() *Client {
	return &Client{scrape: scraper.ScrapeBoard}
}

func (c *Client) Name() string { return "landingjobs" }

// Search scrapes the Landing.Jobs jobs page via the scraper service and
// filters results by the search criteria.
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
			u = fmt.Sprintf("%s?search=%s", searchURL, strings.ReplaceAll(strings.TrimSpace(kw), " ", "+"))
		}
		boardJobs, err := c.scrape(ctx, u, "", criteria.Titles, false)
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
		company = "Landing.Jobs"
	}
	return &provider.Job{
		ID:       url,
		Title:    title,
		Company:  company,
		Location: strings.TrimSpace(j.Location),
		Remote:   j.Remote,
		URL:      url,
		Provider: providerName,
		Board:    "landingjobs",
	}
}

// Apply marks as skipped — Landing.Jobs postings link to the posting page.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually at " + job.URL}, nil
}
