package weworkremotely

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

const feedURL = "https://weworkremotely.com/remote-jobs.rss"

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
}

type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

// Client implements provider.Provider for We Work Remotely.
type Client struct {
	http *http.Client
}

// New creates a We Work Remotely client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "weworkremotely" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weworkremotely: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("weworkremotely: read: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("weworkremotely: xml parse: %w", err)
	}

	var jobs []provider.Job
	for _, item := range feed.Items {
		title := strings.TrimSpace(item.Title)
		jobURL := strings.TrimSpace(item.Link)
		if title == "" || jobURL == "" {
			continue
		}

		// Title often "Company Name: Job Title" — split on ": " to extract company
		company := "We Work Remotely"
		jobTitle := title
		if idx := strings.Index(title, ": "); idx != -1 {
			company = title[:idx]
			jobTitle = title[idx+2:]
		}

		if len(criteria.Titles) > 0 && !matchesTitle(jobTitle, criteria.Titles) {
			continue
		}
		// All jobs are remote; apply location filter
		if !matchesLocation("Remote", true, criteria) {
			continue
		}
		jobs = append(jobs, provider.Job{
			Title:       jobTitle,
			Company:     company,
			Board:       "weworkremotely",
			Location:    "Remote",
			Remote:      true,
			URL:         jobURL,
			Provider:    "weworkremotely",
			Description: item.Description,
		})
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
