package companies

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// YCombinatorDefaultURL is Y Combinator's public, unauthenticated company
// directory API. It returns {name, website, ...} for YC-backed startups —
// no job listings, just a directory. Job listings for these companies are
// found separately by the careerscraper provider, which auto-discovers
// and scrapes each company's own /careers page once it has a Website and
// no known ATS.
const YCombinatorDefaultURL = "https://api.ycombinator.com/v0.1/companies"

// ycMaxPages is a safety cap, not the expected trip count — the loop
// terminates on the first empty page. At research time (2026-07-29) YC's
// directory spans ~200-225 pages of 25 companies each (~5,000-5,600
// companies); this cap leaves headroom for portfolio growth.
const ycMaxPages = 400

type ycResponse struct {
	Companies []ycCompany `json:"companies"`
}

type ycCompany struct {
	Name    string `json:"name"`
	Website string `json:"website"`
	Batch   string `json:"batch"`
	Status  string `json:"status"`
}

// RefreshFromYCombinator imports the YC company directory (name + website)
// as startup-tagged companies with no ATS/board — the careerscraper
// provider picks these up automatically via its companies-DB auto-discovery.
func (s *DB) RefreshFromYCombinator(url string) (int, error) {
	if strings.TrimSpace(url) == "" {
		url = YCombinatorDefaultURL
	}
	client := &http.Client{Timeout: 30 * time.Second}
	now := time.Now().UTC()
	n := 0

	for page := 1; page <= ycMaxPages; page++ {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s?page=%d", url, page), nil)
		if err != nil {
			return n, err
		}
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return n, fmt.Errorf("ycombinator page %d: %w", page, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return n, fmt.Errorf("ycombinator page %d: HTTP %d", page, resp.StatusCode)
		}
		var body ycResponse
		decErr := json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if decErr != nil {
			return n, fmt.Errorf("ycombinator page %d: decode: %w", page, decErr)
		}
		if len(body.Companies) == 0 {
			break
		}

		for _, c := range body.Companies {
			name := strings.TrimSpace(c.Name)
			website := strings.TrimSpace(c.Website)
			if name == "" || website == "" {
				continue
			}
			if err := s.Upsert(Company{
				Name:      name,
				Website:   website,
				Kind:      "startup",
				Industry:  "startup",
				Source:    "ycombinator",
				UpdatedAt: now,
			}); err != nil {
				return n, err
			}
			n++
		}

		// Polite delay between pages — ~200+ pages, don't hammer the API.
		time.Sleep(150 * time.Millisecond)
	}
	return n, nil
}
