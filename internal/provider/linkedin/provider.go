// Package linkedin scrapes LinkedIn job search results via the local Python
// scraper microservice (Playwright backend, no login required).
package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

const defaultBaseURL = "http://localhost:" + scraper.Port

// ensureScraper starts the local scraper service if it is not already running.
// Package-level so tests can substitute a no-op and avoid the real service.
var ensureScraper = func() error {
	if !scraper.Running() {
		if !scraper.Installed() {
			return fmt.Errorf("linkedin: scraper service not installed — open Settings › Career Scraper")
		}
		if err := scraper.Start("", ""); err != nil {
			return fmt.Errorf("linkedin: start scraper service: %w", err)
		}
		if err := scraper.WaitReady(20 * time.Second); err != nil {
			return fmt.Errorf("linkedin: %w", err)
		}
	}
	return nil
}

// Provider implements provider.Provider for LinkedIn job search.
type Provider struct {
	baseURL  string
	client   *http.Client
	maxPages int
}

// New creates a LinkedIn provider.
// maxPages controls how many pages to scrape per search (25 jobs/page).
func New(maxPages int) *Provider {
	if maxPages <= 0 {
		maxPages = 3
	}
	return &Provider{
		baseURL:  defaultBaseURL,
		client:   &http.Client{Timeout: 5 * time.Minute},
		maxPages: maxPages,
	}
}

func (p *Provider) Name() string { return "linkedin" }

func (p *Provider) Search(ctx context.Context, c provider.SearchCriteria) ([]provider.Job, error) {
	if err := ensureScraper(); err != nil {
		return nil, err
	}

	keywords := strings.Join(c.Titles, " OR ")
	if keywords == "" {
		return nil, nil
	}

	location := ""
	if len(c.Locations) > 0 {
		location = c.Locations[0]
	}
	if c.WorkType == "Remote" {
		location = "Remote"
	}

	type reqBody struct {
		Keywords      string `json:"keywords"`
		Location      string `json:"location"`
		MaxPages      int    `json:"max_pages"`
		EasyApplyOnly bool   `json:"easy_apply_only"`
	}
	type jobResult struct {
		Title    string `json:"title"`
		Company  string `json:"company"`
		Location string `json:"location"`
		ApplyURL string `json:"apply_url"`
		Remote   bool   `json:"remote"`
	}
	type respBody struct {
		Jobs       []jobResult `json:"jobs"`
		TotalFound int         `json:"total_found"`
		Error      string      `json:"error"`
	}

	body, _ := json.Marshal(reqBody{
		Keywords:      keywords,
		Location:      location,
		MaxPages:      p.maxPages,
		EasyApplyOnly: false, // scrape all, filter Easy Apply at review time
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/scrape/linkedin", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("linkedin: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedin: http: %w", err)
	}
	defer resp.Body.Close()

	var r respBody
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("linkedin: decode response: %w", err)
	}
	if r.Error != "" {
		return nil, fmt.Errorf("linkedin: %s", r.Error)
	}

	var jobs []provider.Job
	for _, j := range r.Jobs {
		if len(c.Titles) > 0 && !provider.MatchesTitle(j.Title, c.Titles) {
			continue
		}
		if !provider.MatchesLocation(j.Location, j.Remote, c) {
			continue
		}
		jobs = append(jobs, provider.Job{
			ID:       j.ApplyURL,
			Title:    j.Title,
			Company:  j.Company,
			Location: j.Location,
			Remote:   j.Remote,
			URL:      j.ApplyURL,
			Provider: p.Name(),
			Board:    "linkedin",
		})
	}
	return jobs, nil
}

// Apply is not automated — LinkedIn applications require manual interaction.
func (p *Provider) Apply(_ context.Context, j provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{
		Status: "skipped",
		Reason: "linkedin: open apply URL manually: " + j.URL,
	}, nil
}
