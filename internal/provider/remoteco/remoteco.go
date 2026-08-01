// Package remoteco implements provider.Provider for the Remote.co remote
// jobs board via its WordPress RSS feed.
//
// Remote.co is a search-only aggregator: postings link to the employer's
// own apply page, so Apply always returns "skipped".
package remoteco

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

// feedURL is the board's RSS feed.
const feedURL = "https://remote.co/remote-jobs/feed/"

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
}

type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

// Client implements provider.Provider for Remote.co.
type Client struct {
	http *http.Client
	// feedURL overrides the feed for tests.
	feedURL string
}

// New creates a Remote.co client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, feedURL: feedURL}
}

func (c *Client) Name() string { return "remoteco" }

// Search fetches the RSS feed and filters jobs by the search criteria.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.feedURL, nil)
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
		return nil, fmt.Errorf("remoteco: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("remoteco: read: %w", err)
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("remoteco: xml parse: %w", err)
	}

	var jobs []provider.Job
	for _, item := range feed.Items {
		title := strings.TrimSpace(item.Title)
		link := strings.TrimSpace(item.Link)
		if title == "" || link == "" {
			continue
		}

		// Remote.co titles look like "Job Title at Company".
		company := "Remote.co"
		jobTitle := title
		if idx := strings.LastIndex(title, " at "); idx != -1 {
			company = strings.TrimSpace(title[idx+4:])
			jobTitle = strings.TrimSpace(title[:idx])
		}

		if len(criteria.Titles) > 0 && !provider.MatchesTitle(jobTitle, criteria.Titles) {
			continue
		}
		if !provider.MatchesLocation("Remote", true, criteria) {
			continue
		}
		jobs = append(jobs, provider.Job{
			Title:       jobTitle,
			Company:     company,
			Location:    "Remote",
			Remote:      true,
			URL:         link,
			Provider:    "remoteco",
			Board:       "remoteco",
			Description: item.Description,
		})
	}
	return jobs, nil
}

// Apply marks as skipped — Remote.co postings link to the employer's own
// apply page.
func (c *Client) Apply(_ context.Context, job provider.Job, _ provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{Status: "skipped", Reason: "apply manually: " + job.URL}, nil
}
