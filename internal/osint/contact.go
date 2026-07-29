package osint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const scraperBaseURL = "http://localhost:8765"

// Contact represents a found HR/recruiter contact.
type Contact struct {
	ID         int64
	Company    string
	Domain     string
	Name       string
	Title      string
	Email      string
	EmailType  string // "work" | "personal" | "pattern"
	LinkedIn   string
	Source     string // "hunter", "apollo", "github", "pattern"
	Confidence int    // 0-100
	FoundAt    time.Time
	Notes      string
}

// Finder searches for HR contacts at a given company/domain.
type Finder struct {
	hunterKey string
	apolloKey string
	http      *http.Client
}

// NewFinder creates a Finder with optional API keys.
func NewFinder(hunterKey, apolloKey string) *Finder {
	return &Finder{
		hunterKey: hunterKey,
		apolloKey: apolloKey,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

// SearchResult holds results from a single search operation.
type SearchResult struct {
	Contacts []Contact
	Sources  []string // which sources were queried
	Errors   []string // non-fatal errors from sources
}

// Search runs all available OSINT sources and merges results.
// company is the human-readable name; domain is e.g. "linear.app".
func (f *Finder) Search(ctx context.Context, company, domain string) SearchResult {
	var result SearchResult

	seen := map[string]bool{}
	add := func(contacts []Contact, source string, err error) {
		if err != nil {
			result.Errors = append(result.Errors, source+": "+err.Error())
			return
		}
		result.Sources = append(result.Sources, source)
		for _, c := range contacts {
			key := c.Email
			if key == "" {
				key = c.LinkedIn
			}
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			result.Contacts = append(result.Contacts, c)
		}
	}

	if f.hunterKey != "" && domain != "" {
		contacts, err := f.hunterSearch(ctx, company, domain)
		add(contacts, "hunter", err)
	}

	if f.apolloKey != "" {
		contacts, err := f.apolloSearch(ctx, company)
		add(contacts, "apollo", err)
	}

	// GitHub org members — works for most tech companies, no key needed
	if company != "" || domain != "" {
		contacts, err := f.githubSearch(ctx, company, domain)
		add(contacts, "github", err)
	}

	// Free OSINT tools via scraper service (crosslinked, emailfinder, theHarvester, linkedin dork)
	if company != "" {
		contacts, err := f.scraperSearch(ctx, company, domain)
		add(contacts, "osint", err)
	}

	// Pattern emails — always generated as fallback
	if domain != "" {
		patterns := generatePatterns(company, domain)
		// SMTP-verify patterns to filter down to real addresses
		verified := VerifyPatterns(patterns)
		add(verified, "pattern", nil)
	}

	return result
}

// scraperSearch calls the local Python scraper service's /osint/contacts endpoint.
// It uses crosslinked, emailfinder, theHarvester, and Playwright LinkedIn dorking.
func (f *Finder) scraperSearch(ctx context.Context, company, domain string) ([]Contact, error) {
	type reqBody struct {
		Company string `json:"company"`
		Domain  string `json:"domain"`
	}
	type scraperContact struct {
		Name       string `json:"name"`
		Title      string `json:"title"`
		Email      string `json:"email"`
		LinkedIn   string `json:"linkedin_url"`
		Source     string `json:"source"`
		Confidence int    `json:"confidence"`
	}
	type respBody struct {
		Contacts []scraperContact `json:"contacts"`
		Error    string           `json:"error"`
	}

	body, _ := json.Marshal(reqBody{Company: company, Domain: domain})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		scraperBaseURL+"/osint/contacts", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use a longer timeout — OSINT tools can be slow
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	var r respBody
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if r.Error != "" {
		return nil, fmt.Errorf("%s", r.Error)
	}

	now := time.Now()
	contacts := make([]Contact, 0, len(r.Contacts))
	for _, c := range r.Contacts {
		contacts = append(contacts, Contact{
			Company:    company,
			Domain:     domain,
			Name:       c.Name,
			Title:      c.Title,
			Email:      c.Email,
			LinkedIn:   c.LinkedIn,
			Source:     c.Source,
			Confidence: c.Confidence,
			FoundAt:    now,
		})
	}
	return contacts, nil
}
