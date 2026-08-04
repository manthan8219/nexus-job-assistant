// Package instahyre implements provider.Provider for Instahyre.com, an
// India-focused job board matching candidates with top companies.
//
// Instahyre is an Angular SPA whose job results load via XHR after page
// render, so plain HTTP returns an empty shell. Search routes through the
// local scraper service with use_session=true: Playwright connects to the
// user's already-running Chrome (launched with --remote-debugging-port=9222)
// so the logged-in session renders the full job list.
//
// Instahyre is a search-only board: Apply always returns "skipped" with the
// posting URL.
//
// Requires: (1) the scraper service installed and running, AND (2) Chrome
// launched with --remote-debugging-port=9222 and logged into Instahyre.
package instahyre

import (
	"context"
	"fmt"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

const searchURL = "https://www.instahyre.com/search-jobs"

// scrapeFn is the injected board-scrape dependency.
type scrapeFn func(ctx context.Context, url, company string, kws []string, useSession bool) ([]scraper.BoardJob, error)

// Client implements provider.Provider for Instahyre.
type Client struct {
	scrape scrapeFn
}

// New creates an Instahyre client.
func New() *Client {
	return &Client{scrape: scraper.ScrapeBoard}
}

func (c *Client) Name() string { return "instahyre" }

// Search scrapes the Instahyre search page via the scraper service using the
// user's logged-in Chrome session (CDP) and filters results by criteria.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	keywords := criteria.Titles
	if len(keywords) == 0 {
		keywords = []string{""}
	}

	location := ""
	if len(criteria.Locations) > 0 {
		location = criteria.Locations[0]
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
		params := []string{}
		if strings.TrimSpace(kw) != "" {
			params = append(params, "query="+strings.ReplaceAll(strings.TrimSpace(kw), " ", "+"))
		}
		if location != "" {
			params = append(params, "location="+strings.ReplaceAll(location, " ", "+"))
		}
		if len(params) > 0 {
			u = fmt.Sprintf("%s?%s", searchURL, strings.Join(params, "&"))
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
		company = "Instahyre"
	}
	return &provider.Job{
		ID:       url,
		Title:    title,
		Company:  company,
		Location: strings.TrimSpace(j.Location),
		Remote:   j.Remote,
		URL:      url,
		Provider: providerName,
		Board:    "instahyre",
	}
}

// Apply marks as skipped — Instahyre postings link to the posting page.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually at " + job.URL}, nil
}
