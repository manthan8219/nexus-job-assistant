package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/companies"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// CrawlResult is what comes back for each company after a crawl attempt.
type CrawlResult struct {
	Company   string
	CareerURL string // discovered or existing BoardURL
	JobsFound int
	JobsSaved int // new (not already in store)
	Err       error
}

// CrawlCompanies discovers career pages for each company (using Website / BoardURL),
// scrapes job listings via the local scraper service, and saves new jobs to the store.
//
// titleKeywords filters jobs — pass nil to accept all.
// onProgress receives one line per company processed.
func CrawlCompanies(
	ctx context.Context,
	db *companies.DB,
	st *store.Store,
	titleKeywords []string,
	onProgress func(CrawlResult),
) error {
	if !Running() {
		return fmt.Errorf("scraper service not running — install and start it first")
	}

	all, err := db.Search("", "", 5000)
	if err != nil {
		return fmt.Errorf("list companies: %w", err)
	}

	// Filter to companies without a known ATS (those are handled by existing providers)
	var targets []companies.Company
	for _, c := range all {
		if c.ATS == "" && (c.Website != "" || c.BoardURL != "") {
			targets = append(targets, c)
		}
	}

	for _, c := range targets {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		result := CrawlResult{Company: c.Name}

		// 1. Resolve careers URL
		careerURL := c.BoardURL
		if careerURL == "" {
			discovered, err := DiscoverCareersURL(c.Website)
			if err != nil || discovered == "" {
				result.Err = fmt.Errorf("no careers page found")
				if onProgress != nil {
					onProgress(result)
				}
				continue
			}
			careerURL = discovered
		}
		result.CareerURL = careerURL

		// 2. Scrape jobs via service
		jobs, err := scrapeViaService(ctx, c.Name, careerURL, titleKeywords)
		if err != nil {
			result.Err = err
			if onProgress != nil {
				onProgress(result)
			}
			continue
		}
		result.JobsFound = len(jobs)

		// 3. Save new jobs to store
		for _, j := range jobs {
			exists, _ := st.Exists(j.URL)
			if exists {
				continue
			}
			app := store.Application{
				Provider:  "careerscraper",
				Company:   j.Company,
				Role:      j.Title,
				URL:       j.URL,
				Status:    store.StatusSkipped, // queued for review, not yet applied
				Reason:    "discovered via career page crawl",
				Location:  j.Location,
				Remote:    j.Remote,
				AppliedAt: time.Now(),
			}
			if err := st.Insert(app); err == nil {
				result.JobsSaved++
			}
		}

		if onProgress != nil {
			onProgress(result)
		}
	}
	return nil
}

// scrapeViaService calls POST /scrape on the local Python microservice.
func scrapeViaService(ctx context.Context, company, careerURL string, keywords []string) ([]provider.Job, error) {
	type req struct {
		URL      string   `json:"url"`
		Company  string   `json:"company"`
		Keywords []string `json:"title_keywords"`
	}
	type jobResult struct {
		Title    string `json:"title"`
		Company  string `json:"company"`
		Location string `json:"location"`
		ApplyURL string `json:"apply_url"`
		Remote   bool   `json:"remote"`
	}
	type resp struct {
		Jobs  []jobResult `json:"jobs"`
		Error string      `json:"error"`
	}

	if keywords == nil {
		keywords = []string{}
	}
	body, _ := json.Marshal(req{URL: careerURL, Company: company, Keywords: keywords})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL+"/scrape", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("scraper service: %w", err)
	}
	defer httpResp.Body.Close()

	var r resp
	if err := json.NewDecoder(httpResp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if r.Error != "" {
		return nil, fmt.Errorf("scraper: %s", r.Error)
	}

	jobs := make([]provider.Job, 0, len(r.Jobs))
	for _, j := range r.Jobs {
		url := strings.TrimSpace(j.ApplyURL)
		if url == "" {
			url = careerURL
		}
		jobs = append(jobs, provider.Job{
			ID:       url,
			Title:    j.Title,
			Company:  j.Company,
			Location: j.Location,
			Remote:   j.Remote,
			URL:      url,
			Provider: "careerscraper",
			Board:    "career_page",
		})
	}
	return jobs, nil
}
