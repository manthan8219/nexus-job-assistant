package justjoin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

type jjJob struct {
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	CompanyName   string `json:"companyName"`
	City          string `json:"city"`
	WorkplaceType string `json:"workplaceType"`
	// ApplyMethod is "external" when the offer redirects candidates to the
	// employer's own apply page ("applyUrl") instead of the JustJoin form;
	// internal offers are applied through a JustJoin account (login +
	// reCAPTCHA) and cannot be automated (AGENTS.md §14).
	ApplyMethod string `json:"applyMethod"`
	ApplyURL    string `json:"applyUrl"`
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
	// baseURL overrides the API host for tests; when empty the default
	// https://justjoin.it host is used.
	baseURL string
}

// New creates a JustJoin client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, baseURL: "https://justjoin.it"}
}

func (c *Client) Name() string { return "justjoin" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	var jobs []provider.Job

	for page := 1; page <= 3; page++ {
		u := fmt.Sprintf("%s/api/candidate-api/offers?page=%d&perPage=100", c.baseURL, page)
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
			jobURL := offerApplyURL(j)
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

// offerApplyURL returns the canonical URL a candidate should use to apply:
// external offers link to the employer's own apply page; everything else
// uses the JustJoin offer page.
func offerApplyURL(j jjJob) string {
	if strings.EqualFold(strings.TrimSpace(j.ApplyMethod), "external") {
		if u := strings.TrimSpace(j.ApplyURL); u != "" {
			return u
		}
	}
	return "https://justjoin.it/job-offer/" + j.Slug
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
