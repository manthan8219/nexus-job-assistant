package lever

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

// Client implements provider.Provider for Lever.
type Client struct {
	http      *http.Client
	companies []leverCompany
	base      []leverCompany // embedded list; MergeBoards rebuilds from this
}

// New creates a Lever client from embedded JSON bytes.
func New(companiesJSON []byte) (*Client, error) {
	var companies []leverCompany
	if err := json.Unmarshal(companiesJSON, &companies); err != nil {
		return nil, fmt.Errorf("lever: parse companies: %w", err)
	}
	base := append([]leverCompany(nil), companies...)
	return &Client{
		http:      &http.Client{Timeout: 30 * time.Second},
		companies: companies,
		base:      base,
	}, nil
}

func (c *Client) Name() string { return "lever" }

// MergeBoards rebuilds the scan list as embedded base ∪ extra (deduped by Slug).
func (c *Client) MergeBoards(extra []provider.NamedBoard) {
	seen := map[string]struct{}{}
	out := make([]leverCompany, 0, len(c.base)+len(extra))
	for _, co := range c.base {
		k := strings.ToLower(strings.TrimSpace(co.Slug))
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
		out = append(out, leverCompany{Name: b.Name, Slug: b.Board})
	}
	c.companies = out
}

// Search fetches job postings from all companies concurrently.
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

			postings, err := fetchPostings(ctx, c.http, company.Slug)
			if err != nil {
				return
			}
			var jobs []provider.Job
			for _, p := range postings {
				if len(criteria.Titles) > 0 && !matchesTitle(p.Text, criteria.Titles) {
					continue
				}
				if !matchesLocation(p, criteria) {
					continue
				}
				jobs = append(jobs, toProviderJob(p, company))
			}
			if len(jobs) == 0 {
				return
			}
			mu.Lock()
			all = append(all, jobs...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return all, nil
}

// Apply submits an application for a single Lever job.
func (c *Client) Apply(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	return submitApplication(ctx, c.http, job, profile)
}
