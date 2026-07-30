package workday

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

const browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

var postedOnRe = regexp.MustCompile(`(?i)^Posted\s+(\d+)\s+Days?\s+Ago$`)

// parseCareersURL extracts tenant, instance, and site from a Workday careers
// URL. Expected format:
//
//	https://<tenant>.<instance>.myworkdayjobs.com[/<locale>]/<site>
//
// e.g. "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite"
//
// Returns tenant, instance (short host), site, and error.
func parseCareersURL(raw string) (tenant, instance, site string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", err
	}
	if u.Scheme != "https" {
		return "", "", "", fmt.Errorf("only https is supported: %s", raw)
	}

	hostParts := strings.Split(u.Host, ".")
	if len(hostParts) < 4 || !strings.HasSuffix(u.Host, ".myworkdayjobs.com") {
		return "", "", "", fmt.Errorf("unexpected host: %s (expected <tenant>.<instance>.myworkdayjobs.com)", u.Host)
	}
	tenant = hostParts[0]
	instance = hostParts[1] // e.g. "wd5"

	// Path: [/<locale>]/<site>
	path := strings.Trim(u.Path, "/")
	segments := strings.Split(path, "/")
	if len(segments) == 0 || segments[0] == "" {
		return "", "", "", fmt.Errorf("no site segment in path: %s", raw)
	}

	// Detect locale: e.g. "en-US", "en", "fr-CA"
	localeRe := regexp.MustCompile(`^[a-z]{2}(?:-[A-Z]{2})?$`)
	if localeRe.MatchString(segments[0]) && len(segments) > 1 {
		site = segments[1]
	} else {
		site = segments[0]
	}

	return tenant, instance, site, nil
}

// fetchTenantJobs fetches all pages of job postings for a Workday tenant,
// returning the newest maxPages × pageSize postings.
func fetchTenantJobs(ctx context.Context, client *http.Client, name, tenant, instance, site string) ([]provider.Job, error) {
	apiURL := fmt.Sprintf("https://%s.%s.myworkdayjobs.com/wday/cxs/%s/%s/jobs", tenant, instance, tenant, site)
	baseURL := fmt.Sprintf("https://%s.%s.myworkdayjobs.com/%s", tenant, instance, site)

	// Page 0.
	firstResp, err := postCXS(ctx, client, apiURL, 0)
	if err != nil {
		return nil, fmt.Errorf("page 0: %w", err)
	}

	total := firstResp.Total
	var jobs []provider.Job
	jobs = append(jobs, parsePostings(firstResp.JobPostings, baseURL, name)...)

	// Pages 1+.
	for page := 1; page < maxPages; page++ {
		select {
		case <-ctx.Done():
			return jobs, ctx.Err()
		default:
		}

		offset := page * pageSize
		if total > 0 && offset >= total {
			break
		}

		time.Sleep(300 * time.Millisecond) // inter-page delay

		resp, err := postCXS(ctx, client, apiURL, offset)
		if err != nil {
			break
		}
		jobs = append(jobs, parsePostings(resp.JobPostings, baseURL, name)...)

		if len(resp.JobPostings) < pageSize {
			break // short page → last page
		}
	}

	return jobs, nil
}

// postCXS sends a paginated POST request to the Workday CXS endpoint.
func postCXS(ctx context.Context, client *http.Client, apiURL string, offset int) (*wdayResponse, error) {
	body := wdayRequestBody{
		Limit:      pageSize,
		Offset:     offset,
		SearchText: "",
	}
	body.AppliedFacets = struct{}{}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CXS HTTP %d", resp.StatusCode)
	}

	var wresp wdayResponse
	if err := json.NewDecoder(resp.Body).Decode(&wresp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &wresp, nil
}

// parsePostings converts Workday CXS postings to shared Jobs.
func parsePostings(postings []wdayPosting, baseURL, companyName string) []provider.Job {
	var out []provider.Job
	now := time.Now()

	for _, p := range postings {
		title := strings.TrimSpace(p.Title)
		if title == "" {
			continue
		}

		jobURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(p.ExternalPath, "/")

		location := strings.TrimSpace(p.LocationsText)
		isRemote := strings.Contains(strings.ToLower(location), "remote") ||
			strings.Contains(strings.ToLower(title), "remote")

		postedAt := parsePostedOn(p.PostedOn, now)

		out = append(out, provider.Job{
			ID:       strings.TrimRight(baseURL, "/") + p.ExternalPath, // unique enough for dedup
			Title:    title,
			Company:  companyName,
			Board:    strings.TrimRight(baseURL, "/"),
			Location: location,
			Remote:   isRemote,
			URL:      jobURL,
			PostedAt: postedAt,
			Provider: "workday",
		})
	}
	return out
}

// parsePostedOn converts a Workday "postedOn" label to a timestamp.
// "Posted Today"       → now
// "Posted Yesterday"   → now - 24h
// "Posted N Days Ago"  → now - N*24h
// "Posted 30+ Days Ago" → zero time (unbounded)
func parsePostedOn(label string, now time.Time) time.Time {
	if label == "" {
		return time.Time{}
	}
	lower := strings.ToLower(strings.TrimSpace(label))
	switch lower {
	case "posted today":
		return now
	case "posted yesterday":
		return now.Add(-24 * time.Hour)
	}

	m := postedOnRe.FindStringSubmatch(lower)
	if len(m) == 2 {
		if n := parseInt(m[1]); n > 0 && n < 30 {
			return now.Add(-time.Duration(n) * 24 * time.Hour)
		}
	}
	return time.Time{} // "30+ Days Ago" or unrecognized
}

func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			return 0
		}
	}
	return n
}
