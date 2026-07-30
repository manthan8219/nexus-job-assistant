package pinpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

type pptCompany struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type pptLocation struct {
	Name string `json:"name"`
}

type pptJob struct {
	Title    string      `json:"title"`
	URL      string      `json:"url"`
	Path     string      `json:"path"`
	Location pptLocation `json:"location"`
}

type pptResponse struct {
	Data []pptJob `json:"data"`
}

// Client implements provider.Provider for Pinpoint.
type Client struct {
	http      *http.Client
	companies []pptCompany
	base      []pptCompany
}

// New creates a Pinpoint client from embedded JSON bytes.
func New(companiesJSON []byte) (*Client, error) {
	var companies []pptCompany
	if err := json.Unmarshal(companiesJSON, &companies); err != nil {
		return nil, fmt.Errorf("pinpoint: parse companies: %w", err)
	}
	base := append([]pptCompany(nil), companies...)
	return &Client{
		http:      &http.Client{Timeout: 30 * time.Second},
		companies: companies,
		base:      base,
	}, nil
}

func (c *Client) Name() string { return "pinpoint" }

// MergeBoards rebuilds the scan list as embedded base ∪ extra (deduped by Slug).
func (c *Client) MergeBoards(extra []provider.NamedBoard) {
	seen := map[string]struct{}{}
	out := make([]pptCompany, 0, len(c.base)+len(extra))
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
		out = append(out, pptCompany{Name: b.Name, Slug: b.Board})
	}
	c.companies = out
}

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	var jobs []provider.Job

	for _, company := range c.companies {
		select {
		case <-ctx.Done():
			return jobs, ctx.Err()
		default:
		}

		u := fmt.Sprintf("https://%s.pinpointhq.com/postings.json", company.Slug)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

		resp, err := c.http.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var result pptResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		for _, j := range result.Data {
			title := strings.TrimSpace(j.Title)
			if title == "" {
				continue
			}

			jobURL := strings.TrimSpace(j.URL)
			if jobURL == "" {
				jobURL = "https://" + company.Slug + ".pinpointhq.com" + j.Path
			}

			location := j.Location.Name
			remote := strings.Contains(strings.ToLower(location), "remote")

			if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
				continue
			}
			if !matchesLocation(location, remote, criteria) {
				continue
			}
			jobs = append(jobs, provider.Job{
				Title:    title,
				Company:  company.Name,
				Board:    company.Slug,
				Location: location,
				Remote:   remote,
				URL:      jobURL,
				Provider: "pinpoint",
			})
		}

	}
	return jobs, nil
}

func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually: " + job.URL}, nil
}

func matchesTitle(title string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	lower := strings.ToLower(title)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(kw))) {
			return true
		}
	}
	return false
}

func matchesLocation(location string, remote bool, criteria provider.SearchCriteria) bool {
	if criteria.WorkType == "Remote" {
		return remote
	}
	if len(criteria.Locations) == 0 {
		return true
	}
	loc := strings.ToLower(location)
	for _, t := range criteria.Locations {
		if strings.Contains(loc, strings.ToLower(strings.TrimSpace(t))) {
			return true
		}
	}
	return false
}
