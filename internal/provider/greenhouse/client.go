package greenhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

// Client implements provider.Provider for Greenhouse.
type Client struct {
	http      *http.Client
	companies []Company
	base      []Company
}

// New creates a Greenhouse client from embedded JSON bytes.
func New(companiesJSON []byte) (*Client, error) {
	return newFromBytes(companiesJSON)
}

// NewFromFile creates a Greenhouse client loading companies from a file path.
// Useful when the user supplies a custom --companies flag.
func NewFromFile(path string) (*Client, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load companies: %w", err)
	}
	return newFromBytes(b)
}

func newFromBytes(b []byte) (*Client, error) {
	var companies []Company
	if err := json.Unmarshal(b, &companies); err != nil {
		return nil, fmt.Errorf("parse companies: %w", err)
	}
	base := append([]Company(nil), companies...)
	return &Client{
		http:      &http.Client{Timeout: 30 * time.Second},
		companies: companies,
		base:      base,
	}, nil
}

func (c *Client) Name() string { return "greenhouse" }

// MergeBoards rebuilds the scan list as embedded base ∪ extra (deduped by board).
func (c *Client) MergeBoards(extra []provider.NamedBoard) {
	seen := map[string]struct{}{}
	out := make([]Company, 0, len(c.base)+len(extra))
	for _, co := range c.base {
		k := strings.ToLower(strings.TrimSpace(co.Board))
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
		out = append(out, Company{Name: b.Name, Board: b.Board})
	}
	c.companies = out
}

// Search iterates all companies, fetches their jobs, and filters by criteria.
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

			jobs, err := fetchJobs(ctx, c.http, company.Board)
			if err != nil {
				return
			}
			var matched []provider.Job
			for _, j := range jobs {
				if len(criteria.Titles) > 0 && !matchesTitle(j.Title, criteria.Titles) {
					continue
				}
				if !matchesLocation(j.Location.Name, criteria) {
					continue
				}
				matched = append(matched, toProviderJob(j, company))
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

// Apply submits an application for a single job.
// Fetches the individual job detail with ?questions=true to get the application form fields.
func (c *Client) Apply(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	questions, err := fetchJobQuestions(ctx, c.http, job.Board, job.ID)
	if err != nil {
		return provider.ApplyResult{Status: "failed", Reason: err.Error()}, nil
	}
	if len(questions) == 0 {
		return provider.ApplyResult{Status: "skipped", Reason: "no application form found for this job"}, nil
	}
	return submitApplication(ctx, c.http, job, questions, profile)
}

// ParseTitles splits a comma-separated titles string into a slice.
func ParseTitles(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ParseLocations splits a comma-separated locations string into a slice.
func ParseLocations(s string) []string {
	return ParseTitles(s) // same logic
}
