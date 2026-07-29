package personio

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

type psCompany struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type psPosition struct {
	JobPosition string `xml:"jobPosition"`
	JobLocation string `xml:"jobLocation"`
	URL         string `xml:"url"`
}

type psXML struct {
	Positions []psPosition `xml:"position"`
}

// Client implements provider.Provider for Personio.
type Client struct {
	http      *http.Client
	companies []psCompany
	base      []psCompany
}

// New creates a Personio client from embedded JSON bytes.
func New(companiesJSON []byte) (*Client, error) {
	var companies []psCompany
	if err := json.Unmarshal(companiesJSON, &companies); err != nil {
		return nil, fmt.Errorf("personio: parse companies: %w", err)
	}
	base := append([]psCompany(nil), companies...)
	return &Client{
		http:      &http.Client{Timeout: 30 * time.Second},
		companies: companies,
		base:      base,
	}, nil
}

func (c *Client) Name() string { return "personio" }

// MergeBoards rebuilds the scan list as embedded base ∪ extra (deduped by Slug).
func (c *Client) MergeBoards(extra []provider.NamedBoard) {
	seen := map[string]struct{}{}
	out := make([]psCompany, 0, len(c.base)+len(extra))
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
		out = append(out, psCompany{Name: b.Name, Slug: b.Board})
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

		u := fmt.Sprintf("https://%s.jobs.personio.de/xml", company.Slug)
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

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		var psData psXML
		if err := xml.Unmarshal(body, &psData); err != nil {
			continue
		}

		for _, pos := range psData.Positions {
			title := strings.TrimSpace(pos.JobPosition)
			jobURL := strings.TrimSpace(pos.URL)
			if title == "" || jobURL == "" {
				continue
			}

			location := pos.JobLocation
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
				Provider: "personio",
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
