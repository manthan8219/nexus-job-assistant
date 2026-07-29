package scraper

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// careerSlugs are tried in order against the company's root domain.
// First URL that returns 200 and looks like a job listing page wins.
var careerSlugs = []string{
	"/careers",
	"/jobs",
	"/about/careers",
	"/about/jobs",
	"/company/careers",
	"/company/jobs",
	"/work-with-us",
	"/join-us",
	"/join",
	"/hiring",
	"/openings",
	"/positions",
	"/careers/open-positions",
	"/careers/jobs",
	"/en/careers",
	"/en/jobs",
}

var httpClient = &http.Client{
	Timeout: 8 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// DiscoverCareersURL takes a company website (e.g. "https://stripe.com") and
// returns the first URL that responds 200 and matches career page heuristics.
// Returns ("", nil) when nothing is found.
func DiscoverCareersURL(website string) (string, error) {
	root, err := rootURL(website)
	if err != nil {
		return "", err
	}

	for _, slug := range careerSlugs {
		candidate := root + slug
		if ok, _ := looksLikeCareersPage(candidate); ok {
			return candidate, nil
		}
	}
	return "", nil
}

// looksLikeCareersPage fetches the URL and checks heuristics:
//   - HTTP 200
//   - Content-Type text/html
//   - Body contains job-signal keywords
func looksLikeCareersPage(rawURL string) (bool, error) {
	resp, err := httpClient.Get(rawURL)
	if err != nil {
		return false, nil // unreachable — not an error, just skip
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		return false, nil
	}

	// Read first 32 KB — enough to spot job signals without loading whole page
	buf := make([]byte, 32*1024)
	n, _ := resp.Body.Read(buf)
	body := strings.ToLower(string(buf[:n]))

	signals := []string{
		"open position", "open role", "job opening",
		"we're hiring", "we are hiring", "join our team",
		"job listing", "current opening", "view all jobs",
		"apply now", "full-time", "part-time", "engineering",
	}
	for _, sig := range signals {
		if strings.Contains(body, sig) {
			return true, nil
		}
	}
	return false, nil
}

// rootURL normalises a website string to scheme+host with no trailing slash.
func rootURL(website string) (string, error) {
	s := strings.TrimSpace(website)
	if s == "" {
		return "", fmt.Errorf("empty website")
	}
	if !strings.HasPrefix(s, "http") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host), nil
}
