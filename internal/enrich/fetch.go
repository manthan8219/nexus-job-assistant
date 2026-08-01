package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
	"github.com/manthan8219/nexus-job-assistant/internal/textutil"
)

var (
	ghAPIJob = regexp.MustCompile(`(?i)boards-api\.greenhouse\.io/v1/boards/([^/]+)/jobs/(\d+)`)
	ghHosted = regexp.MustCompile(`(?i)boards\.greenhouse\.io/([^/]+)/jobs/(\d+)`)
	leverURL = regexp.MustCompile(`(?i)jobs\.lever\.co/([^/]+)/([a-f0-9-]+)`)
)

var (
	ghAPIBase    = "https://boards-api.greenhouse.io/v1/boards"
	leverAPIBase = "https://api.lever.co/v0/postings"
)

// FetchDescription re-downloads a job description for an already-stored URL.
// Supports Greenhouse + Lever today; other providers return an error.
func FetchDescription(ctx context.Context, provider, jobURL string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	jobURL = strings.TrimSpace(jobURL)
	if jobURL == "" {
		return "", fmt.Errorf("empty job url")
	}
	client := &http.Client{Timeout: 20 * time.Second}

	switch provider {
	case "greenhouse":
		return fetchGreenhouse(ctx, client, jobURL)
	case "lever":
		return fetchLever(ctx, client, jobURL)
	case "linkedin", "careerscraper":
		return fetchViaPlaywright(ctx, client, jobURL)
	default:
		if ghAPIJob.MatchString(jobURL) || ghHosted.MatchString(jobURL) {
			return fetchGreenhouse(ctx, client, jobURL)
		}
		if leverURL.MatchString(jobURL) {
			return fetchLever(ctx, client, jobURL)
		}
		return "", fmt.Errorf("re-fetch not supported for provider %q", provider)
	}
}

func fetchGreenhouse(ctx context.Context, client *http.Client, jobURL string) (string, error) {
	board, id := "", ""
	if m := ghAPIJob.FindStringSubmatch(jobURL); len(m) == 3 {
		board, id = m[1], m[2]
	} else if m := ghHosted.FindStringSubmatch(jobURL); len(m) == 3 {
		board, id = m[1], m[2]
	} else {
		return "", fmt.Errorf("unrecognized greenhouse url")
	}
	api := fmt.Sprintf("%s/%s/jobs/%s", ghAPIBase, url.PathEscape(board), id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; nexus-enrich/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("greenhouse HTTP %d", resp.StatusCode)
	}
	var doc struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", err
	}
	content := textutil.HTMLToPlain(doc.Content)
	if content == "" {
		return "", fmt.Errorf("greenhouse returned empty content")
	}
	return content, nil
}

func fetchLever(ctx context.Context, client *http.Client, jobURL string) (string, error) {
	m := leverURL.FindStringSubmatch(jobURL)
	slug, id := "", ""
	if len(m) == 3 {
		slug, id = m[1], m[2]
	} else {
		u, err := url.Parse(jobURL)
		if err != nil {
			return "", fmt.Errorf("unrecognized lever url")
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 2 {
			return "", fmt.Errorf("unrecognized lever url")
		}
		slug, id = parts[0], parts[1]
	}
	api := fmt.Sprintf("%s/%s/%s", leverAPIBase, url.PathEscape(slug), url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; nexus-enrich/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return "", fmt.Errorf("lever HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var doc struct {
		DescriptionPlain string `json:"descriptionPlain"`
		Description      string `json:"description"`
		AdditionalPlain  string `json:"additionalPlain"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", err
	}
	desc := strings.TrimSpace(doc.DescriptionPlain)
	if desc == "" {
		desc = strings.TrimSpace(doc.Description)
	}
	if add := strings.TrimSpace(doc.AdditionalPlain); add != "" {
		if desc != "" {
			desc += "\n\n" + add
		} else {
			desc = add
		}
	}
	desc = textutil.HTMLToPlain(desc)
	if desc == "" {
		return "", fmt.Errorf("lever returned empty description")
	}
	return desc, nil
}

// fetchViaPlaywright calls the local Python scraper microservice /description endpoint.
// Used for LinkedIn and career-page jobs that require a real browser to render.
func fetchViaPlaywright(ctx context.Context, client *http.Client, jobURL string) (string, error) {
	if !scraper.Running() {
		return "", fmt.Errorf("scraper service not running — install via Settings › Career Scraper")
	}
	type reqBody struct {
		URL string `json:"url"`
	}
	type respBody struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	body, _ := json.Marshal(reqBody{URL: jobURL})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		scraper.BaseURL+"/description", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("scraper /description: %w", err)
	}
	defer resp.Body.Close()
	var r respBody
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("scraper /description decode: %w", err)
	}
	if r.Error != "" {
		return "", fmt.Errorf("scraper /description: %s", r.Error)
	}
	if strings.TrimSpace(r.Text) == "" {
		return "", fmt.Errorf("scraper /description: empty response")
	}
	return r.Text, nil
}
