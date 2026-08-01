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

// verifyBatchTimeout bounds the whole SMTP verification pass of one Search
// call. Politeness delays and greylisting retries make a large batch slow, so
// the pass is capped and every remaining address is reported inconclusive
// rather than blocking the search forever.
const verifyBatchTimeout = 2 * time.Minute

// Contact represents a found HR/recruiter contact.
type Contact struct {
	ID         int64     `json:"id"`
	Company    string    `json:"company"`
	Domain     string    `json:"domain"`
	Name       string    `json:"name"`
	Title      string    `json:"title"`
	Email      string    `json:"email"`
	EmailType  string    `json:"emailType"` // "work" | "personal" | "pattern"
	LinkedIn   string    `json:"linkedIn"`
	Source     string    `json:"source"`     // "hunter", "apollo", "github", "pattern"
	Confidence int       `json:"confidence"` // 0-100
	FoundAt    time.Time `json:"foundAt"`
	Notes      string    `json:"notes"`
}

// Finder searches for HR contacts at a given company/domain.
type Finder struct {
	hunterKey string
	apolloKey string
	http      *http.Client
	// scraperURL overrides the local OSINT scraper service base URL.
	// Empty means the default localhost:8765; tests inject an httptest URL.
	scraperURL string
	// Verify enables SMTP probing of the found addresses. It is slow (dials
	// port 25) and many networks block it, so callers opt in explicitly.
	Verify bool
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
		add(patterns, "pattern", nil)
	}

	// SMTP-verify every found address (from any source) so Confidence
	// reflects reality: a definitive accept upgrades it, a definitive reject
	// zeroes it, and unreachable/inconclusive results keep the source
	// confidence with a reason in Notes. Verification never drops a contact.
	if f.Verify && len(result.Contacts) > 0 {
		ctx2, cancel := context.WithTimeout(ctx, verifyBatchTimeout)
		defer cancel()
		result.Contacts = NewVerifier().VerifyContacts(ctx2, result.Contacts)
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
	base := scraperBaseURL
	if f.scraperURL != "" {
		base = f.scraperURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/osint/contacts", bytes.NewReader(body))
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scraper service HTTP %d", resp.StatusCode)
	}

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
