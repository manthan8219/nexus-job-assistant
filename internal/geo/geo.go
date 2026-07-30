// Package geo provides a slim city→country index for Target Locations.
//
// Data is derived from dr5hn/countries-states-cities-database (Open Database License).
package geo

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/manthan8219/nexus-job-assistant/data"
)

// City is one row from the slim index.
type City struct {
	Name    string
	Country string
	ISO2    string
}

// Display returns the canonical tag form "City, Country".
func (c City) Display() string {
	if c.Country == "" {
		return c.Name
	}
	return c.Name + ", " + c.Country
}

type slimRow struct {
	N string `json:"n"`
	C string `json:"c"`
	I string `json:"i"`
}

var (
	loadOnce sync.Once
	loadErr  error
	cities   []City
	byKey    map[string]City   // lower(display)
	byName   map[string][]City // lower(name) → all matches
)

func ensureLoaded() error {
	loadOnce.Do(func() {
		loadErr = loadIndex(data.CitiesIndexGZ)
	})
	return loadErr
}

func loadIndex(gz []byte) error {
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return fmt.Errorf("geo: gzip: %w", err)
	}
	defer r.Close()
	raw, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("geo: read: %w", err)
	}
	var rows []slimRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return fmt.Errorf("geo: json: %w", err)
	}
	cities = make([]City, 0, len(rows))
	byKey = make(map[string]City, len(rows))
	byName = make(map[string][]City, len(rows)/2)
	for _, row := range rows {
		c := City{Name: row.N, Country: row.C, ISO2: row.I}
		if c.Name == "" || c.Country == "" {
			continue
		}
		cities = append(cities, c)
		byKey[strings.ToLower(c.Display())] = c
		ln := strings.ToLower(c.Name)
		byName[ln] = append(byName[ln], c)
	}
	return nil
}

// Search returns cities matching query (aliases → name prefix → name substring).
// Limit ≤ 0 means 8. Alias keys match on prefix too ("bangl" → bangalore → Bengaluru).
func Search(query string, limit int) []City {
	if err := ensureLoaded(); err != nil || query == "" {
		return nil
	}
	if limit <= 0 {
		limit = 8
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}

	var aliased, prefix, rest []City
	seen := make(map[string]struct{}, limit*2)
	add := func(c City, dest *[]City) {
		k := strings.ToLower(c.Display())
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		*dest = append(*dest, c)
	}

	// Exact alias match first (deterministic, not subject to map iteration order).
	if display, ok := cityAliases[q]; ok {
		if c, ok := Resolve(display); ok {
			add(c, &aliased)
		}
	}
	// Fuzzy alias hits — prefix / 1-edit typos ("bangl" → bangalore → Bengaluru).
	for alias, display := range cityAliases {
		if alias == q {
			continue // already handled above
		}
		if !aliasMatches(alias, q) {
			continue
		}
		if c, ok := Resolve(display); ok {
			add(c, &aliased)
		}
		if len(aliased) >= limit {
			return aliased[:limit]
		}
	}

	for _, c := range cities {
		ln := strings.ToLower(c.Name)
		ld := strings.ToLower(c.Display())
		switch {
		case strings.HasPrefix(ln, q) || strings.HasPrefix(ld, q):
			add(c, &prefix)
		case strings.Contains(ln, q):
			add(c, &rest)
		}
		if len(aliased)+len(prefix) >= limit {
			break
		}
	}
	out := append(aliased, prefix...)
	out = append(out, rest...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Resolve maps a display string or unique city name to a City.
func Resolve(s string) (City, bool) {
	if err := ensureLoaded(); err != nil {
		return City{}, false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return City{}, false
	}
	if c, ok := byKey[strings.ToLower(s)]; ok {
		return c, true
	}
	// "City, Country" with flexible spacing
	if i := strings.LastIndex(s, ","); i > 0 {
		name := strings.TrimSpace(s[:i])
		country := strings.TrimSpace(s[i+1:])
		if name != "" && country != "" {
			key := strings.ToLower(name + ", " + country)
			if c, ok := byKey[key]; ok {
				return c, true
			}
			for _, c := range byName[strings.ToLower(name)] {
				if strings.EqualFold(c.Country, country) {
					return c, true
				}
			}
		}
	}
	if display, ok := lookupAlias(s); ok {
		if c, ok := byKey[strings.ToLower(display)]; ok {
			return c, true
		}
		if i := strings.LastIndex(display, ","); i > 0 {
			name := strings.TrimSpace(display[:i])
			country := strings.TrimSpace(display[i+1:])
			for _, c := range byName[strings.ToLower(name)] {
				if strings.EqualFold(c.Country, country) {
					return c, true
				}
			}
		}
	}
	matches := byName[strings.ToLower(s)]
	if len(matches) == 1 {
		return matches[0], true
	}
	return City{}, false
}

// CountriesFromTags extracts unique ISO2 country codes from location tags
// like "Bengaluru, India", "India", or "IN". Unresolved free-text is ignored.
func CountriesFromTags(tags []string) []string {
	_ = ensureLoaded()
	seen := map[string]struct{}{}
	var out []string
	add := func(iso string) {
		iso = strings.ToUpper(strings.TrimSpace(iso))
		if len(iso) != 2 {
			return
		}
		if _, ok := seen[iso]; ok {
			return
		}
		seen[iso] = struct{}{}
		out = append(out, iso)
	}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if c, ok := Resolve(tag); ok {
			if c.ISO2 != "" {
				add(c.ISO2)
				continue
			}
		}
		// Country-only tag: match against known country names in the index.
		upper := strings.ToUpper(tag)
		if len(upper) == 2 {
			for _, c := range cities {
				if strings.EqualFold(c.ISO2, upper) {
					add(c.ISO2)
					break
				}
			}
			continue
		}
		for _, c := range cities {
			if strings.EqualFold(c.Country, tag) {
				add(c.ISO2)
				break
			}
		}
		// "City, Country" where city is unknown but country suffix is known.
		if i := strings.LastIndex(tag, ","); i > 0 {
			country := strings.TrimSpace(tag[i+1:])
			for _, c := range cities {
				if strings.EqualFold(c.Country, country) {
					add(c.ISO2)
					break
				}
			}
			if len(country) == 2 {
				add(country)
			}
		}
	}
	return out
}

// ExpandLocations turns saved tags into city + country match terms (deduped).
// Unresolved legacy free-text tags are passed through unchanged.
func ExpandLocations(tags []string) []string {
	_ = ensureLoaded()
	seen := make(map[string]struct{}, len(tags)*2)
	var out []string
	add := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" {
			return
		}
		k := strings.ToLower(term)
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, term)
	}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if c, ok := Resolve(tag); ok {
			add(c.Name)
			add(c.Country)
			for _, alt := range reverseAliases[strings.ToLower(c.Name)] {
				add(alt)
			}
			continue
		}
		add(tag)
	}
	return out
}

// Len returns how many cities are loaded (0 if not yet / failed).
func Len() int {
	_ = ensureLoaded()
	return len(cities)
}

// ParseLocationTags splits saved target_locations.
// New format uses "; " between "City, Country" tags.
// Legacy comma-separated free-text (no semicolons) is still accepted.
func ParseLocationTags(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	sep := ","
	if strings.Contains(s, ";") {
		sep = ";"
	}
	var tags []string
	for _, part := range strings.Split(s, sep) {
		part = strings.TrimSpace(part)
		if part != "" {
			tags = append(tags, part)
		}
	}
	return tags
}

// JoinLocationTags encodes tags for config storage (semicolon-separated).
func JoinLocationTags(tags []string) string {
	var clean []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			clean = append(clean, t)
		}
	}
	return strings.Join(clean, "; ")
}
