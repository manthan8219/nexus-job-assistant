package workday

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

const (
	// Max pages to fetch per tenant (100 jobs at page size 20). A
	// search-oriented tool doesn't need the full directory — the newest 100
	// jobs cover the most relevant openings.
	maxPages = 5
	pageSize = 20
)

// Client implements provider.Provider for Workday CXS.
type Client struct {
	http      *http.Client
	companies []wdayCompany
	base      []wdayCompany
}

// New creates a Workday client from embedded company JSON bytes.
func New(companiesJSON []byte) (*Client, error) {
	var companies []wdayCompany
	if err := json.Unmarshal(companiesJSON, &companies); err != nil {
		return nil, fmt.Errorf("workday: parse companies: %w", err)
	}
	base := append([]wdayCompany(nil), companies...)
	return &Client{
		http:      &http.Client{Timeout: 30 * time.Second},
		companies: companies,
		base:      base,
	}, nil
}

func (c *Client) Name() string { return "workday" }

// MergeBoards rebuilds the scan list as embedded base ∪ extra (deduped by URL).
func (c *Client) MergeBoards(extra []provider.NamedBoard) {
	seen := map[string]struct{}{}
	out := make([]wdayCompany, 0, len(c.base)+len(extra))
	for _, co := range c.base {
		k := strings.ToLower(strings.TrimSpace(co.URL))
		if k == "" {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, co)
	}
	for _, b := range extra {
		k := strings.ToLower(strings.TrimSpace(b.Board))
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, wdayCompany{Name: b.Name, URL: b.Board})
	}
	c.companies = out
}

// Search iterates all configured Workday tenants, fetches job postings,
// and filters by criteria.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	const maxConcurrent = 8
	sem := make(chan struct{}, maxConcurrent)
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		all []provider.Job
	)

	for _, company := range c.companies {
		if ctx.Err() != nil {
			break
		}
		company := company
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			tenant, instance, site, err := parseCareersURL(company.URL)
			if err != nil {
				return
			}
			jobs, err := fetchTenantJobs(ctx, c.http, company.Name, tenant, instance, site)
			if err != nil {
				return
			}
			var matched []provider.Job
			for _, j := range jobs {
				if len(criteria.Titles) > 0 && !provider.MatchesTitle(j.Title, criteria.Titles) {
					continue
				}
				if !provider.MatchesLocation(j.Location, j.Remote, criteria) {
					continue
				}
				matched = append(matched, j)
			}
			if len(matched) == 0 {
				return
			}
			mu.Lock()
			all = append(all, matched...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return all, nil
}

// Apply marks as skipped — Workday requires per-tenant accounts; there is no
// public apply API.
func (c *Client) Apply(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{
		Status: "skipped",
		Reason: "apply manually at " + job.URL,
	}, nil
}
