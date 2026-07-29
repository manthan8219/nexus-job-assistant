package wttj

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

type wttjOffice struct {
	City    string `json:"city"`
	Country string `json:"country"`
}

type wttjHit struct {
	ObjectID         string       `json:"objectID"`
	JobTitle         string       `json:"job_title"`
	OrganizationName string       `json:"organization_name"`
	Remote           string       `json:"remote"`
	Offices          []wttjOffice `json:"offices"`
}

type wttjSearchResponse struct {
	Hits []wttjHit `json:"hits"`
}

type wttjEnv struct {
	PublicAlgoliaApplicationID  string `json:"PUBLIC_ALGOLIA_APPLICATION_ID"`
	PublicAlgoliaAPIKeyClient   string `json:"PUBLIC_ALGOLIA_API_KEY_CLIENT"`
}

// Client implements provider.Provider for Welcome to the Jungle.
type Client struct {
	http *http.Client
}

// New creates a WTTJ client.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "wttj" }

func (c *Client) Search(ctx context.Context, criteria provider.SearchCriteria) ([]provider.Job, error) {
	// Step 1: fetch env to get Algolia credentials
	envReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.welcometothejungle.com/api/env", nil)
	if err != nil {
		return nil, nil
	}
	envReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")

	envResp, err := c.http.Do(envReq)
	if err != nil {
		return nil, nil
	}
	defer envResp.Body.Close()

	if envResp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var env wttjEnv
	if err := json.NewDecoder(envResp.Body).Decode(&env); err != nil {
		return nil, nil
	}

	appID := strings.TrimSpace(env.PublicAlgoliaApplicationID)
	apiKey := strings.TrimSpace(env.PublicAlgoliaAPIKeyClient)
	if appID == "" || apiKey == "" {
		return nil, nil
	}

	// Step 2: query Algolia
	algoliaURL := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/wttj_jobs_production_en/query", appID)
	searchBody := map[string]interface{}{
		"query":       "",
		"hitsPerPage": 100,
		"page":        0,
	}
	bodyBytes, err := json.Marshal(searchBody)
	if err != nil {
		return nil, nil
	}

	algReq, err := http.NewRequestWithContext(ctx, http.MethodPost, algoliaURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, nil
	}
	algReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; job-search-bot/1.0)")
	algReq.Header.Set("X-Algolia-Application-Id", appID)
	algReq.Header.Set("X-Algolia-API-Key", apiKey)
	algReq.Header.Set("Referer", "https://www.welcometothejungle.com")
	algReq.Header.Set("Content-Type", "application/json")

	algResp, err := c.http.Do(algReq)
	if err != nil {
		return nil, nil
	}
	defer algResp.Body.Close()

	if algResp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var searchResp wttjSearchResponse
	if err := json.NewDecoder(algResp.Body).Decode(&searchResp); err != nil {
		return nil, nil
	}

	var jobs []provider.Job
	for _, hit := range searchResp.Hits {
		title := strings.TrimSpace(hit.JobTitle)
		if title == "" || hit.ObjectID == "" {
			continue
		}
		jobURL := "https://www.welcometothejungle.com/jobs/" + hit.ObjectID
		remote := hit.Remote == "fulltime" || hit.Remote == "partial"

		var location string
		if len(hit.Offices) > 0 {
			parts := []string{}
			if hit.Offices[0].City != "" {
				parts = append(parts, hit.Offices[0].City)
			}
			if hit.Offices[0].Country != "" {
				parts = append(parts, hit.Offices[0].Country)
			}
			location = strings.Join(parts, ", ")
		}

		if len(criteria.Titles) > 0 && !matchesTitle(title, criteria.Titles) {
			continue
		}
		if !matchesLocation(location, remote, criteria) {
			continue
		}
		jobs = append(jobs, provider.Job{
			ID:       hit.ObjectID,
			Title:    title,
			Company:  hit.OrganizationName,
			Board:    "wttj",
			Location: location,
			Remote:   remote,
			URL:      jobURL,
			Provider: "wttj",
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
