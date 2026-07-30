package companies

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/data"
)

type boardRow struct {
	Name  string `json:"name"`
	Board string `json:"board"`
	Slug  string `json:"slug"`
	URL   string `json:"url"`
}

// ImportNexusEmbeddedBoards loads Greenhouse/Lever/Ashby/… lists shipped with Nexus.
// These often lack hire-country tags; they are still searchable by name and usable as ATS boards.
func (s *DB) ImportNexusEmbeddedBoards() (int, error) {
	now := time.Now().UTC()
	batches := []struct {
		ats   string
		raw   []byte
		urlFn func(boardRow) string
	}{
		{"greenhouse", data.CompaniesJSON, func(r boardRow) string {
			b := first(r.Board, r.Slug)
			if b == "" {
				return ""
			}
			return "https://boards.greenhouse.io/" + b
		}},
		{"ashby", data.AshbyCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return "https://jobs.ashbyhq.com/" + b
		}},
		{"lever", data.LeverCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return "https://jobs.lever.co/" + b
		}},
		{"workable", data.WorkableCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return "https://apply.workable.com/" + b
		}},
		{"smartrecruiters", data.SmartRecruitersCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return "https://jobs.smartrecruiters.com/" + b
		}},
		{"workday", data.WorkdayCompaniesJSON, func(r boardRow) string {
			return strings.TrimSpace(r.URL)
		}},
		{"bamboohr", data.BambooHRCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return "https://" + b + ".bamboohr.com/careers"
		}},
		{"recruitee", data.RecruiteeCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return "https://" + b + ".recruitee.com"
		}},
		{"breezy", data.BreezyCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return "https://" + b + ".breezy.hr"
		}},
		{"jobvite", data.JobviteCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return "https://jobs.jobvite.com/" + b
		}},
		{"teamtailor", data.TeamtailorCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return "https://" + b + ".teamtailor.com"
		}},
		{"personio", data.PersonioCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return "https://" + b + ".jobs.personio.com"
		}},
		{"pinpoint", data.PinpointCompaniesJSON, func(r boardRow) string {
			b := first(r.Slug, r.Board)
			if b == "" {
				return ""
			}
			return b // may already be URL in some files
		}},
	}

	n := 0
	for _, b := range batches {
		var rows []boardRow
		if len(bytesTrim(b.raw)) == 0 {
			continue
		}
		if err := json.Unmarshal(b.raw, &rows); err != nil {
			return n, fmt.Errorf("%s boards: %w", b.ats, err)
		}
		for _, r := range rows {
			name := strings.TrimSpace(r.Name)
			if name == "" {
				continue
			}
			board := first(r.Board, r.Slug)
			boardURL := b.urlFn(r)
			if boardURL == "" && board != "" {
				boardURL = board
			}
			c := Company{
				Name:      name,
				ATS:       b.ats,
				Board:     board,
				BoardURL:  boardURL,
				Kind:      "tech",
				Industry:  "tech",
				Source:    "nexus-boards",
				UpdatedAt: now,
			}
			// Preserve countries if we already know them from OpenJobs — Upsert merges on conflict
			// but overwrites hire_countries. So only set empty countries here; Upsert SQL should
			// prefer non-empty hire data. Adjust Upsert to not wipe countries when incoming empty.
			if err := s.Upsert(c); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

func first(a, b string) string {
	a = strings.TrimSpace(a)
	if a != "" {
		return a
	}
	return strings.TrimSpace(b)
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
