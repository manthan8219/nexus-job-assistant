package companies

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenJobsDefaultURL is the public MIT company dataset (hire countries + ATS links).
const OpenJobsDefaultURL = "https://raw.githubusercontent.com/Santosh-Goyal/OpenJobs/main/data/companies_v2.json"

type openJobsRow struct {
	Name             string   `json:"name"`
	Website          string   `json:"website"`
	IndustryCategory string   `json:"industry_category"`
	Type             string   `json:"type"`
	ATSLinks         []string `json:"ats_links"`
	ListURLs         []string `json:"list_urls"`
	Countries        []string `json:"countries"`
}

// ImportOpenJobsJSON imports companies from an OpenJobs-style JSON array.
func (s *DB) ImportOpenJobsJSON(r io.Reader) (inserted int, err error) {
	var rows []openJobsRow
	dec := json.NewDecoder(r)
	if err := dec.Decode(&rows); err != nil {
		return 0, fmt.Errorf("openjobs json: %w", err)
	}
	now := time.Now().UTC()
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		if name == "" {
			continue
		}
		urls := row.ATSLinks
		if len(urls) == 0 {
			urls = row.ListURLs
		}
		if len(urls) == 0 {
			urls = []string{""}
		}
		codes := make([]string, 0, len(row.Countries))
		for _, c := range row.Countries {
			_, iso, ok := NormalizeCountry(c)
			if ok && iso != "" {
				codes = append(codes, iso)
			}
		}
		kind := strings.TrimSpace(row.IndustryCategory)
		if kind == "" {
			kind = strings.TrimSpace(strings.ToLower(row.Type))
		}
		for _, rawURL := range urls {
			ats, board := ParseATSURL(rawURL)
			c := Company{
				Name:             name,
				Website:          strings.TrimSpace(row.Website),
				ATS:              ats,
				Board:            board,
				BoardURL:         strings.TrimSpace(rawURL),
				HireCountries:    append([]string(nil), row.Countries...),
				HireCountryCodes: uniqueFold(codes),
				Kind:             kind,
				Industry:         kind,
				Source:           "openjobs",
				UpdatedAt:        now,
			}
			if err := s.Upsert(c); err != nil {
				return inserted, err
			}
			inserted++
		}
	}
	return inserted, nil
}

// RefreshFromOpenJobs downloads the public dataset and merges into the DB.
func (s *DB) RefreshFromOpenJobs(url string) (int, error) {
	if strings.TrimSpace(url) == "" {
		url = OpenJobsDefaultURL
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("openjobs HTTP %d", resp.StatusCode)
	}
	return s.ImportOpenJobsJSON(resp.Body)
}
