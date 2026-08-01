package smartrecruiters

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

var baseURL = "https://api.smartrecruiters.com"

// Client implements provider.Provider for SmartRecruiters.
type Client struct {
	http      *http.Client
	companies []srCompanyEntry
	base      []srCompanyEntry
	// apiBase overrides the SmartRecruiters API host for tests; defaults to
	// baseURL.
	apiBase string
}

// New creates a SmartRecruiters client from embedded JSON bytes.
func New(companiesJSON []byte) (*Client, error) {
	var companies []srCompanyEntry
	if err := json.Unmarshal(companiesJSON, &companies); err != nil {
		return nil, fmt.Errorf("smartrecruiters: parse companies: %w", err)
	}
	base := append([]srCompanyEntry(nil), companies...)
	return &Client{
		http:      &http.Client{Timeout: 30 * time.Second},
		companies: companies,
		base:      base,
		apiBase:   baseURL,
	}, nil
}

func (c *Client) Name() string { return "smartrecruiters" }

// MergeBoards rebuilds the scan list as embedded base ∪ extra (deduped by Identifier).
func (c *Client) MergeBoards(extra []provider.NamedBoard) {
	seen := map[string]struct{}{}
	out := make([]srCompanyEntry, 0, len(c.base)+len(extra))
	for _, co := range c.base {
		k := strings.ToLower(strings.TrimSpace(co.Identifier))
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
		out = append(out, srCompanyEntry{Name: b.Name, Identifier: b.Board})
	}
	c.companies = out
}

// Search iterates all companies, fetches their job postings, and filters by criteria.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	const maxConcurrent = 10
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

			postings, err := fetchPostings(ctx, c.http, company.Identifier)
			if err != nil {
				return
			}
			var matched []provider.Job
			for _, p := range postings {
				if len(criteria.Titles) > 0 && !matchesTitle(p.Name, criteria.Titles) {
					continue
				}
				if !matchesLocation(p, criteria) {
					continue
				}
				matched = append(matched, toProviderJob(p, company))
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

// Apply submits an application for a single SmartRecruiters job.
func (c *Client) Apply(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	return c.submitApplication(ctx, job, profile)
}
