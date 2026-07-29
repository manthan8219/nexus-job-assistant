package companies

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/data"
)

type indiaEmployer struct {
	Name     string `json:"name"`
	Website  string `json:"website"`
	BoardURL string `json:"board_url"`
	ATS      string `json:"ats"`
	Board    string `json:"board"`
}

// ImportIndiaEmployers tags curated employers as hiring in India.
// Prefer merging into an existing row by name; insert only when unknown.
func (s *DB) ImportIndiaEmployers() (int, error) {
	var rows []indiaEmployer
	if err := json.Unmarshal(data.IndiaEmployersJSON, &rows); err != nil {
		return 0, fmt.Errorf("india employers: %w", err)
	}
	now := time.Now().UTC()
	n := 0
	for _, r := range rows {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		updated, err := s.ensureHireCountryByName(name, "India", Company{
			Name:             name,
			Website:          strings.TrimSpace(r.Website),
			ATS:              strings.TrimSpace(r.ATS),
			Board:            strings.TrimSpace(r.Board),
			BoardURL:         strings.TrimSpace(r.BoardURL),
			HireCountries:    []string{"India"},
			HireCountryCodes: []string{"IN"},
			Kind:             "tech",
			Industry:         "tech",
			Source:           "india-priority",
			UpdatedAt:        now,
		})
		if err != nil {
			return n, err
		}
		if updated {
			n++
		}
	}
	return n, nil
}

// ensureHireCountryByName merges country into every existing row with this name.
// If none exist, upserts fallback.
func (s *DB) ensureHireCountryByName(name, country string, fallback Company) (bool, error) {
	rows, err := s.db.Query(`
SELECT id, name, website, ats, board, board_url,
       hire_countries, hire_country_codes, hq_country, hq_country_code,
       kind, industry, source, updated_at
FROM companies WHERE lower(name) = lower(?)`, name)
	if err != nil {
		return false, err
	}
	list, err := scanCompanies(rows)
	rows.Close()
	if err != nil {
		return false, err
	}
	if len(list) == 0 {
		if err := s.Upsert(fallback); err != nil {
			return false, err
		}
		return true, nil
	}
	_, iso, ok := NormalizeCountry(country)
	if !ok || iso == "" {
		return false, fmt.Errorf("unknown country %q", country)
	}
	canon, _, _ := NormalizeCountry(country)
	for _, c := range list {
		c.HireCountries = uniqueFold(append(c.HireCountries, canon))
		c.HireCountryCodes = uniqueFold(append(c.HireCountryCodes, iso))
		if c.Website == "" {
			c.Website = fallback.Website
		}
		if c.ATS == "" {
			c.ATS = fallback.ATS
		}
		if c.Board == "" {
			c.Board = fallback.Board
		}
		if c.BoardURL == "" {
			c.BoardURL = fallback.BoardURL
		}
		if c.Source == "" {
			c.Source = fallback.Source
		}
		c.UpdatedAt = fallback.UpdatedAt
		if err := s.Upsert(c); err != nil {
			return false, err
		}
	}
	return true, nil
}
