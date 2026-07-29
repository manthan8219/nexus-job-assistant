package justjoin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

type jjJob struct {
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	CompanyName   string `json:"companyName"`
	City          string `json:"city"`
	WorkplaceType string `json:"workplaceType"`
}

type jjMeta struct {
	TotalPages int `json:"totalPages"`
}

type jjResponse struct {
	Data []jjJob `json:"data"`
	Meta jjMeta  `json:"meta"`
}

// Client implements provider.Provider for JustJoin.it.
type Client struct {
	http *http.Client
}

// New creates a JustJoin client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "justjoin" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	var jobs []provider.Job

	for page := 1; page <= 3; page++ {
		u := fmt.Sprintf("https://justjoin.it/api/candidate-api/offers?page=%d&perPage=100", page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return jobs, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

		resp, err := c.http.Do(req)
		if err != nil {
			return jobs, err
		}

		var result jjResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return jobs, fmt.Errorf("justjoin: page %d decode: %w", page, err)
		}
		resp.Body.Close()

		for _, j := range result.Data {
			title := strings.TrimSpace(j.Title)
			if title == "" || j.Slug == "" {
				continue
			}
			jobURL := "https://justjoin.it/job-offer/" + j.Slug
			remote := j.WorkplaceType == "remote" || j.WorkplaceType == "hybrid"

			if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
				continue
			}
			if !matchesLocation(j.City, remote, criteria) {
				continue
			}
			jobs = append(jobs, provider.Job{
				Title:    title,
				Company:  j.CompanyName,
				Board:    "justjoin",
				Location: j.City,
				Remote:   remote,
				URL:      jobURL,
				Provider: "justjoin",
			})
		}

		if page >= result.Meta.TotalPages {
			break
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
