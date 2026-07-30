package geo

import "strings"

// cityAliases maps common / legacy spellings → canonical "City, Country"
// as stored in the slim index (dr5hn names).
//
// Job boards often still use the old English names (Bangalore, Gurgaon),
// so ExpandLocations also emits these aliases as match terms.
var cityAliases = map[string]string{
	// India — IT hubs & renamed cities
	"bangalore":           "Bengaluru, India",
	"banglore":            "Bengaluru, India", // common misspelling
	"bangl":               "Bengaluru, India", // common truncation
	"bengalooru":          "Bengaluru, India",
	"gurgaon":             "Gurugram, India",
	"bombay":              "Mumbai, India",
	"calcutta":            "Kolkata, India",
	"madras":              "Chennai, India",
	"poona":               "Pune, India",
	"trivandrum":          "Thiruvananthapuram, India",
	"tvm":                 "Thiruvananthapuram, India",
	"kochi":               "Cochin, India", // index uses Cochin
	"cochi":               "Cochin, India",
	"pondicherry":         "Puducherry, India",
	"pondichery":          "Puducherry, India",
	"baroda":              "Vadodara, India",
	"benares":             "Varanasi, India",
	"banaras":             "Varanasi, India",
	"kashi":               "Varanasi, India",
	"mysore":              "Mysuru, India",
	"mangalore":           "Mangaluru, India",
	"hubli":               "Hubballi, India",
	"hubli-dharwad":       "Hubballi, India",
	"simla":               "Shimla, India",
	"prayagraj":           "Prayagraj (Allahabad), India",
	"allahabad":           "Prayagraj (Allahabad), India",
	"cawnpore":            "Kanpur, India",
	"belgaum":             "Belagavi, India",
	"belgaon":             "Belagavi, India",
	"tumkur":              "Tumakuru, India",
	"trichy":              "Tiruchirappalli, India",
	"tiruchi":             "Tiruchirappalli, India",
	"trichinopoly":        "Tiruchirappalli, India",
	"tuticorin":           "Thoothukudi, India",
	"ootacamund":          "Ooty, India",
	"udhagamandalam":      "Ooty, India",
	"vizag":               "Visakhapatnam, India",
	"vizagapatam":         "Visakhapatnam, India",
	"waltair":             "Visakhapatnam, India",
	"secunderabad":        "Hyderabad, India", // twin city; jobs usually say Hyderabad
	"navi mumbai":         "Mumbai, India",
	"new bombay":          "Mumbai, India",
	"gautam buddha nagar": "Noida, India",
	"greater noida":       "Noida, India",

	// Common non-India job-board aliases
	"sf":            "San Francisco, United States",
	"san fran":      "San Francisco, United States",
	"nyc":           "New York City, United States",
	"new york city": "New York City, United States",
	"new york":      "New York City, United States",
	"la":            "Los Angeles, United States",
	"l.a.":          "Los Angeles, United States",
}

// reverseAliases: lower(canonical city name) → alternate spellings for job matching.
var reverseAliases map[string][]string

func init() {
	reverseAliases = make(map[string][]string, len(cityAliases))
	for alias, display := range cityAliases {
		name := display
		if i := strings.Index(display, ","); i > 0 {
			name = strings.TrimSpace(display[:i])
		}
		key := strings.ToLower(name)
		// Keep human-facing alias casing: title-ish from map key
		label := aliasLabel(alias)
		if !containsFold(reverseAliases[key], label) {
			reverseAliases[key] = append(reverseAliases[key], label)
		}
	}
}

func aliasLabel(alias string) string {
	parts := strings.Fields(alias)
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		// Keep short all-caps tokens (nyc, sf, tvm, la)
		if len(p) <= 3 && strings.IndexFunc(p, func(r rune) bool { return r < 'a' || r > 'z' }) == -1 {
			parts[i] = strings.ToUpper(p)
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func containsFold(ss []string, want string) bool {
	for _, s := range ss {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

func lookupAlias(query string) (string, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return "", false
	}
	if display, ok := cityAliases[q]; ok {
		return display, true
	}
	// Fuzzy: only if all matching aliases point at the same city
	var hit string
	for alias, display := range cityAliases {
		if !aliasMatches(alias, q) {
			continue
		}
		if hit == "" {
			hit = display
			continue
		}
		if !strings.EqualFold(hit, display) {
			return "", false
		}
	}
	if hit != "" {
		return hit, true
	}
	return "", false
}

// aliasMatches reports whether typed query refers to this alias key.
// Allows exact, prefix, and 1-character prefix typos ("bangl" → "bangalore").
func aliasMatches(alias, q string) bool {
	if alias == q || strings.HasPrefix(alias, q) {
		return true
	}
	if len(q) < 3 {
		return false
	}
	if len(q) >= 4 && strings.Contains(alias, q) {
		return true
	}
	// Compare equal-length prefix with at most one substitution/insertion/deletion
	if len(alias) >= len(q) {
		return editDistAtMost1(alias[:len(q)], q)
	}
	return editDistAtMost1(alias, q)
}

func editDistAtMost1(a, b string) bool {
	if a == b {
		return true
	}
	la, lb := len(a), len(b)
	if la > lb {
		a, b = b, a
		la, lb = lb, la
	}
	if lb-la > 1 {
		return false
	}
	if la == lb {
		diffs := 0
		for i := 0; i < la; i++ {
			if a[i] != b[i] {
				diffs++
				if diffs > 1 {
					return false
				}
			}
		}
		return diffs <= 1
	}
	// lb == la+1 — one insertion in b
	i, j := 0, 0
	skipped := false
	for i < la && j < lb {
		if a[i] == b[j] {
			i++
			j++
			continue
		}
		if skipped {
			return false
		}
		skipped = true
		j++
	}
	return true
}
