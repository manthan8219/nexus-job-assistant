package nodesk

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

var feedURL = "https://nodesk.co/remote-jobs/index.xml"

type rssItem struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

// Client implements provider.Provider for NoDesk.
type Client struct {
	http *http.Client
}

// New creates a NoDesk client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "nodesk" }

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
		return nil, fmt.Errorf("nodesk: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("nodesk: read: %w", err)
	}

	// Nodesk RSS contains HTML entities (&rsquo; etc) that strict XML rejects.
	// Replace common ones before parsing.
	sanitized := sanitizeXML(body)

	var feed rssFeed
	if err := xml.Unmarshal(sanitized, &feed); err != nil {
		return nil, fmt.Errorf("nodesk: xml parse: %w", err)
	}

	var jobs []provider.Job
	for _, item := range feed.Items {
		title := strings.TrimSpace(item.Title)
		jobURL := strings.TrimSpace(item.Link)
		if title == "" || jobURL == "" {
			continue
		}

		if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
			continue
		}
		if !matchesLocation("Remote", true, criteria) {
			continue
		}
		jobs = append(jobs, provider.Job{
			Title:    title,
			Company:  "NoDesk",
			Board:    "nodesk",
			Location: "Remote",
			Remote:   true,
			URL:      jobURL,
			Provider: "nodesk",
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

// sanitizeXML replaces HTML entities not defined in XML 1.0 so the strict
// Go XML parser can handle RSS feeds that mix HTML and XML.
func sanitizeXML(b []byte) []byte {
	s := string(b)
	replacer := strings.NewReplacer(
		"&rsquo;", "'",
		"&lsquo;", "'",
		"&rdquo;", "\u201d",
		"&ldquo;", "\u201c",
		"&ndash;", "-",
		"&mdash;", "-",
		"&amp;amp;", "&amp;",
		"&nbsp;", " ",
		"&hellip;", "...",
	)
	return []byte(replacer.Replace(s))
}
