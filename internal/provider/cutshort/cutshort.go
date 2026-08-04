// Package cutshort implements provider.Provider for Cutshort.io, an India-focused
// tech/startup job board. Its /jobs pages render category listings in
// server-side HTML; Search routes through the local scraper service (Playwright
// renders the page, generic extraction pulls job links).
//
// Cutshort is a search-only board: Apply always returns "skipped" with the
// posting URL. No login is required to browse the category pages.
//
// Requires the scraper service to be installed and running (Settings › Career
// Scraper).
package cutshort

import (
	"context"
	"fmt"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

const searchURL = "https://cutshort.io/jobs"

// scrapeFn is the injected board-scrape dependency.
type scrapeFn func(ctx context.Context, url, company string, kws []string, useSession bool) ([]scraper.BoardJob, error)

// Client implements provider.Provider for Cutshort.
type Client struct {
	scrape scrapeFn
}

// New creates a Cutshort client.
func New() *Client {
	return &Client{scrape: scraper.ScrapeBoard}
}

func (c *Client) Name() string { return "cutshort" }

// Search scrapes the Cutshort jobs page via the scraper service and filters
// results by the search criteria.
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
			u = fmt.Sprintf("%s?q=%s", searchURL, strings.ReplaceAll(strings.TrimSpace(kw), " ", "+"))
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
		company = "Cutshort"
	}
	return &provider.Job{
		ID:       url,
		Title:    title,
		Company:  company,
		Location: strings.TrimSpace(j.Location),
		Remote:   j.Remote,
		URL:      url,
		Provider: providerName,
		Board:    "cutshort",
	}
}

// Apply marks as skipped — Cutshort postings link to the posting page.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually at " + job.URL}, nil
}
