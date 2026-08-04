package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// BoardJob is one job extracted by the /scrape/board endpoint.
type BoardJob struct {
	Title    string `json:"title"`
	Company  string `json:"company"`
	Location string `json:"location"`
	ApplyURL string `json:"apply_url"`
	Remote   bool   `json:"remote"`
}

// boardHTTPClient is the shared client for board-scrape requests (§10: one
// shared client, explicit timeout, never a fresh client per request).
var boardHTTPClient = &http.Client{Timeout: 3 * time.Minute}

// CDPReady reports whether a Chrome instance with remote debugging is
// reachable on localhost:9222 (i.e. the user launched Chrome with
// --remote-debugging-port=9222). Board providers that need a logged-in
// session register only when this is true.
func CDPReady() bool {
	return cdpStatus("http://localhost:9222")
}

// cdpStatus reports whether a Chrome DevTools endpoint is reachable at base
// (e.g. http://localhost:9222). Split out for hermetic tests.
func cdpStatus(base string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/json/version", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// EnsureRunning starts the scraper service if it is not already running,
// waiting up to 20s for it to become healthy. ollamaModel/ollamaURL are
// forwarded to the service; empty values default inside Start.
func EnsureRunning(ollamaModel, ollamaURL string) error {
	if Running() {
		return nil
	}
	if !Installed() {
		return fmt.Errorf("scraper service not installed — open Settings › Career Scraper")
	}
	if err := Start(ollamaModel, ollamaURL); err != nil {
		return fmt.Errorf("start scraper service: %w", err)
	}
	if err := WaitReady(20 * time.Second); err != nil {
		return err
	}
	return nil
}

// ScrapeBoard calls the /scrape/board endpoint to extract job listings from a
// job-board search page. When useSession is true it connects to the user's
// logged-in Chrome over CDP; otherwise it renders headless. It ensures the
// scraper service is running first.
func ScrapeBoard(ctx context.Context, url, company string, titleKeywords []string, useSession bool) ([]BoardJob, error) {
	if err := EnsureRunning("", ""); err != nil {
		return nil, err
	}
	return scrapeBoardHTTP(ctx, BaseURL, url, company, titleKeywords, useSession)
}

// boardRequest is the JSON body posted to /scrape/board.
type boardRequest struct {
	URL           string   `json:"url"`
	Company       string   `json:"company"`
	TitleKeywords []string `json:"title_keywords"`
	UseSession    bool     `json:"use_session"`
}

// boardResponse is the JSON body returned by /scrape/board.
type boardResponse struct {
	Jobs  []BoardJob `json:"jobs"`
	Error string     `json:"error"`
}

// scrapeBoardHTTP posts the board scrape request to the service at base and
// returns the extracted jobs. Split out for hermetic tests.
func scrapeBoardHTTP(ctx context.Context, base, url, company string, titleKeywords []string, useSession bool) ([]BoardJob, error) {
	body, err := json.Marshal(boardRequest{
		URL:           url,
		Company:       company,
		TitleKeywords: titleKeywords,
		UseSession:    useSession,
	})
	if err != nil {
		return nil, fmt.Errorf("scraper: marshal board request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/scrape/board", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("scraper: build board request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use the shared client so each request doesn't spin up a new one (§10).
	client := boardHTTPClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scraper: board http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scraper: board service returned %d", resp.StatusCode)
	}

	var r boardResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("scraper: decode board response: %w", err)
	}
	if r.Error != "" {
		return nil, fmt.Errorf("scraper: %s", r.Error)
	}
	return r.Jobs, nil
}
