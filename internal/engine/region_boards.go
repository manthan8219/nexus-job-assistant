package engine

import (
	"fmt"
	"strings"

	"github.com/manthanmanthan/nexus/internal/companies"
	"github.com/manthanmanthan/nexus/internal/config"
	"github.com/manthanmanthan/nexus/internal/geo"
	"github.com/manthanmanthan/nexus/internal/provider"
)

// countriesFromConfig extracts ISO2 codes from Config target locations
// ("Bengaluru, India" → IN, "India" → IN, "IN" → IN).
func countriesFromConfig(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	tags := geo.ParseLocationTags(cfg.TargetLocations)
	isos := geo.CountriesFromTags(tags)
	if len(isos) > 0 {
		return isos
	}
	// Fallback: NormalizeCountry on raw tags / country suffixes when geo index misses.
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		_, iso, ok := companies.NormalizeCountry(raw)
		if !ok || iso == "" {
			return
		}
		k := strings.ToLower(iso)
		if _, dup := seen[k]; dup {
			return
		}
		seen[k] = struct{}{}
		out = append(out, iso)
	}
	for _, tag := range tags {
		add(tag)
		if i := strings.LastIndex(tag, ","); i > 0 {
			add(strings.TrimSpace(tag[i+1:]))
		}
	}
	return out
}

// expandBoardsFromCompanyDB merges companies.db boards for the user's countries
// into every ATS provider that supports BoardMerger. Safe to call every run:
// MergeBoards rebuilds from each client's embedded base list.
func (e *Engine) expandBoardsFromCompanyDB(countries []string) {
	// Always reset mergers to base (empty extra) when no country — keeps lists clean.
	if len(countries) == 0 {
		for _, p := range e.providers {
			if m, ok := p.(provider.BoardMerger); ok {
				m.MergeBoards(nil)
			}
		}
		e.log("Company DB: no country in Target Locations — using embedded ATS lists only")
		return
	}

	byATS, err := companies.BoardsByATS(countries)
	if err != nil {
		e.log("Company DB: %v — falling back to embedded ATS lists", err)
		for _, p := range e.providers {
			if m, ok := p.(provider.BoardMerger); ok {
				m.MergeBoards(nil)
			}
		}
		return
	}

	totalExtra := 0
	for ats, list := range byATS {
		totalExtra += len(list)
		_ = ats
	}
	e.log("Company DB: expanding ATS boards for countries %v (%d scannable employers)", countries, totalExtra)

	for _, p := range e.providers {
		m, ok := p.(provider.BoardMerger)
		if !ok {
			continue
		}
		list := byATS[strings.ToLower(p.Name())]
		extra := make([]provider.NamedBoard, 0, len(list))
		for _, c := range list {
			extra = append(extra, provider.NamedBoard{
				Name:  c.Name,
				Board: companies.BoardToken(c),
			})
		}
		m.MergeBoards(extra)
		if len(extra) > 0 {
			e.log("  [%s] +%d boards from companies.db", p.Name(), len(extra))
		}
	}
}

// regionSummary is a short human line for the run log / notifier.
func regionSummary(countries []string) string {
	if len(countries) == 0 {
		return "global (no country in locations)"
	}
	return fmt.Sprintf("countries=%s", strings.Join(countries, ","))
}
