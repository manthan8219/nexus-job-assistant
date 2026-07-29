package getonbrd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

// JSON:API response types for Get on Board.
type gobCompanyData struct {
	Attributes struct {
		Name string `json:"name"`
	} `json:"attributes"`
}

type gobCompanyWrapper struct {
	Data gobCompanyData `json:"data"`
}

type gobAttributes struct {
	Title       string            `json:"title"`
	Remote      bool              `json:"remote"`
	Countries   []string          `json:"countries"`
	CompanyName string            `json:"company_name"`
	Company     gobCompanyWrapper `json:"company"`
}

type gobLinks struct {
	PublicURL string `json:"public_url"`
}

type gobItem struct {
	Attributes gobAttributes `json:"attributes"`
	Links      gobLinks      `json:"links"`
}

type gobResponse struct {
	Data []gobItem `json:"data"`
}

// Client implements provider.Provider for Get on Board.
type Client struct {
	http *http.Client
}

// New creates a Get on Board client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "getonbrd" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	var jobs []provider.Job

	for page := 1; page <= 3; page++ {
		u := fmt.Sprintf("https://www.getonbrd.com/api/v0/categories/programming/jobs?expand[]=company&page=%d&per_page=100", page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return jobs, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

		resp, err := c.http.Do(req)
		if err != nil {
			return jobs, err
		}

		var result gobResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return jobs, fmt.Errorf("getonbrd: page %d decode: %w", page, err)
		}
		resp.Body.Close()

		if len(result.Data) == 0 {
			break
		}

		for _, item := range result.Data {
			attr := item.Attributes
			title := strings.TrimSpace(attr.Title)
			jobURL := strings.TrimSpace(item.Links.PublicURL)
			if title == "" || jobURL == "" {
				continue
			}

			// Resolve company name: try nested company data, then company_name, then fallback
			companyName := attr.Company.Data.Attributes.Name
			if companyName == "" {
				companyName = attr.CompanyName
			}
			if companyName == "" {
				companyName = "Get on Board"
			}

			location := strings.Join(attr.Countries, ", ")

			if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
				continue
			}
			if !matchesLocation(location, attr.Remote, criteria) {
				continue
			}
			jobs = append(jobs, provider.Job{
				Title:    title,
				Company:  companyName,
				Board:    "getonbrd",
				Location: location,
				Remote:   attr.Remote,
				URL:      jobURL,
				Provider: "getonbrd",
			})
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
