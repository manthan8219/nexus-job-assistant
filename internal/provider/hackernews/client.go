package hackernews

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

// Client implements provider.Provider for the HN "Who is hiring?" thread.
type Client struct {
	http *http.Client
}

// New creates a HackerNews client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "hackernews" }

// Search fetches the latest monthly "Who is hiring?" thread and parses
// top-level comments into jobs.
func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	threadID, threadURL, err := searchThread(ctx, c.http)
	if err != nil {
		return nil, fmt.Errorf("hackernews: %w", err)
	}

	item, err := fetchThread(ctx, c.http, threadID)
	if err != nil {
		return nil, fmt.Errorf("hackernews: %w", err)
	}

	var results []provider.Job
	for _, child := range item.Children {
		if child.Deleted || child.Dead || strings.TrimSpace(child.Text) == "" {
			continue
		}

		parsed := parseComment(child.Text, threadURL)
		if parsed == nil {
			continue
		}

		if len(criteria.Titles) > 0 && !provider.MatchesTitle(parsed.Title, criteria.Titles) {
			continue
		}

		isRemote := strings.Contains(strings.ToLower(parsed.Location), "remote")
		if !provider.MatchesLocation(parsed.Location, isRemote, criteria) {
			continue
		}

		var postedAt time.Time
		if t, err := time.Parse(time.RFC3339, child.CreatedAt); err == nil {
			postedAt = t
		} else if t, err := time.Parse("2006-01-02T15:04:05", child.CreatedAt); err == nil {
			postedAt = t
		}

		results = append(results, provider.Job{
			ID:       fmt.Sprintf("%d", child.ID),
			Title:    parsed.Title,
			Company:  parsed.Company,
			Board:    threadID,
			Location: parsed.Location,
			Remote:   isRemote,
			URL:      parsed.URL,
			PostedAt: postedAt,
			Provider: "hackernews",
		})
	}

	return results, nil
}

// Apply marks as skipped — HN threads link to company career pages.
func (c *Client) Apply(ctx context.Context, job provider.Job, profile provider.Profile) (provider.ApplyResult, error) {
	return provider.ApplyResult{
		Status: "skipped",
		Reason: "apply manually at " + job.URL,
	}, nil
}
