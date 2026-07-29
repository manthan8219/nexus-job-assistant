package companies

import (
	"fmt"
	"strings"
)

// BoardToken returns the token an ATS client needs (slug, id, or Workday URL).
func BoardToken(c Company) string {
	if strings.TrimSpace(c.Board) != "" {
		return strings.TrimSpace(c.Board)
	}
	if strings.EqualFold(c.ATS, "workday") && strings.TrimSpace(c.BoardURL) != "" {
		return strings.TrimSpace(c.BoardURL)
	}
	if c.BoardURL != "" {
		if _, b := ParseATSURL(c.BoardURL); b != "" {
			return b
		}
		return strings.TrimSpace(c.BoardURL)
	}
	return ""
}

// HasATSBoard is true when we can scan this employer via a public ATS client.
func HasATSBoard(c Company) bool {
	return strings.TrimSpace(c.ATS) != "" && BoardToken(c) != ""
}

// BoardsForATS returns name+board pairs for one ATS vendor in a country.
func (s *DB) BoardsForATS(country, ats string) ([]Company, error) {
	all, err := s.FindByCountry(country)
	if err != nil {
		return nil, err
	}
	var out []Company
	for _, c := range all {
		if strings.EqualFold(c.ATS, ats) && BoardToken(c) != "" {
			out = append(out, c)
		}
	}
	return out, nil
}

// BoardsByATS loads scannable boards for one or more countries, grouped by ATS.
// Dedupes by (ats, board token). countries may be ISO2 ("IN") or names ("India").
func BoardsByATS(countries []string) (map[string][]Company, error) {
	db, err := OpenDefault()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	n, err := db.Count()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("company database is empty — run: go run ./cmd/companies-seed")
	}

	out := map[string][]Company{}
	seen := map[string]struct{}{} // ats|board
	for _, country := range countries {
		country = strings.TrimSpace(country)
		if country == "" {
			continue
		}
		list, err := db.FindByCountry(country)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			if !HasATSBoard(c) {
				continue
			}
			ats := strings.ToLower(strings.TrimSpace(c.ATS))
			tok := BoardToken(c)
			key := ats + "|" + strings.ToLower(tok)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			c.Board = tok
			out[ats] = append(out[ats], c)
		}
	}
	return out, nil
}
