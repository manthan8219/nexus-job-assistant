// Package careerscraper is a provider that fetches job listings from company
// career pages via the local Python scraper microservice (ScrapeGraphAI).
package careerscraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/companies"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

const defaultBaseURL = "http://localhost:" + scraper.Port

// Target is one company career page to scrape.
type Target struct {
	Company string // human-readable name e.g. "Stripe"
	URL     string // career listing page e.g. "https://stripe.com/jobs"
}

// Provider implements provider.Provider by calling the Python scraper service.
type Provider struct {
	baseURL     string
	targets     []Target
	client      *http.Client
	ollamaModel string
	ollamaURL   string
}

// New creates a Provider.
//   - targets: company career pages to scrape.
//   - ollamaModel/ollamaURL: forwarded to the scraper service (e.g. "llama3.2", "http://localhost:11434").
func New(targets []Target, ollamaModel, ollamaURL string) *Provider {
	return &Provider{
		baseURL:     defaultBaseURL,
		targets:     targets,
		client:      &http.Client{Timeout: 5 * time.Minute},
		ollamaModel: ollamaModel,
		ollamaURL:   ollamaURL,
	}
}

func (p *Provider) Name() string { return "careerscraper" }

func (p *Provider) Search(ctx context.Context, c provider.SearchCriteria) ([]provider.Job, error) {
	// Auto-start the scraper service if installed but not running.
	if !scraper.Running() {
		if !scraper.Installed() {
			return nil, fmt.Errorf("careerscraper: not installed — open Settings › Career Scraper to install")
		}
		if err := scraper.Start(p.ollamaModel, p.ollamaURL); err != nil {
			return nil, fmt.Errorf("careerscraper: start service: %w", err)
		}
		if err := scraper.WaitReady(20 * time.Second); err != nil {
			return nil, fmt.Errorf("careerscraper: %w", err)
		}
	}

	type scrapeRequest struct {
		URL           string   `json:"url"`
		Company       string   `json:"company"`
		TitleKeywords []string `json:"title_keywords"`
	}
	type batchRequest struct {
		Targets []scrapeRequest `json:"targets"`
	}

	// If no targets provided, auto-discover from companies DB (no-ATS companies only).
	targets := p.targets
	if len(targets) == 0 {
		targets = loadTargetsFromDB()
	}
	if len(targets) == 0 {
		return nil, nil // no companies to scrape
	}

	reqs := make([]scrapeRequest, len(targets))
	for i, t := range targets {
		reqs[i] = scrapeRequest{
			URL:           t.URL,
			Company:       t.Company,
			TitleKeywords: c.Titles,
		}
	}

	body, err := json.Marshal(batchRequest{Targets: reqs})
	if err != nil {
		return nil, fmt.Errorf("careerscraper: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/scrape/batch", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("careerscraper: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("careerscraper: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("careerscraper: service returned %d", resp.StatusCode)
	}

	type jobResult struct {
		Title      string `json:"title"`
		Company    string `json:"company"`
		Location   string `json:"location"`
		Department string `json:"department"`
		ApplyURL   string `json:"apply_url"`
		Remote     bool   `json:"remote"`
	}
	type scrapeResponse struct {
		Company string      `json:"company"`
		URL     string      `json:"url"`
		Jobs    []jobResult `json:"jobs"`
		Error   string      `json:"error"`
	}
	type batchResponse struct {
		Results []scrapeResponse `json:"results"`
	}

	var batch batchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batch); err != nil {
		return nil, fmt.Errorf("careerscraper: decode response: %w", err)
	}

	var jobs []provider.Job
	for _, result := range batch.Results {
		if result.Error != "" {
			// log but don't fail the whole batch
			continue
		}
		for _, j := range result.Jobs {
			if !provider.MatchesLocation(j.Location, j.Remote, c) {
				continue
			}
			jobs = append(jobs, provider.Job{
				ID:       j.ApplyURL, // URL as stable dedup key
				Title:    j.Title,
				Company:  j.Company,
				Location: j.Location,
				Remote:   j.Remote,
				URL:      j.ApplyURL,
				Provider: p.Name(),
				Board:    "career_page",
			})
		}
	}

	return jobs, nil
}

// loadTargetsFromDB returns career-page targets for all companies without a known ATS.
// If BoardURL is set it is used directly; otherwise DiscoverCareersURL probes the website.
func loadTargetsFromDB() []Target {
	db, err := companies.OpenDefault()
	if err != nil {
		return nil
	}
	defer db.Close()

	all, err := db.Search("", "", 5000)
	if err != nil {
		return nil
	}

	var targets []Target
	for _, c := range all {
		if c.ATS != "" {
			continue // handled by dedicated ATS provider
		}
		careerURL := c.BoardURL
		if careerURL == "" && c.Website != "" {
			// Probe for /careers, /jobs, etc.
			discovered, err := scraper.DiscoverCareersURL(c.Website)
			if err != nil || discovered == "" {
				continue
			}
			careerURL = discovered
		}
		if careerURL == "" {
			continue
		}
		targets = append(targets, Target{Company: c.Name, URL: careerURL})
	}
	return targets
}

// Apply is not supported for career-page scraping (manual apply via URL).
func (p *Provider) Apply(_ context.Context, j provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{
		Status: "skipped",
		Reason: "careerscraper: open apply URL manually: " + j.URL,
	}, nil
}
