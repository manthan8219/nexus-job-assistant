package companies

import (
	"strings"
	"sync"
)

// Common aliases → ISO2.
var countryAliases = map[string]string{
	"in": "IN", "india": "IN", "bharat": "IN", "hindustan": "IN",
	"us": "US", "usa": "US", "united states": "US", "united states of america": "US", "america": "US",
	"uk": "GB", "gb": "GB", "united kingdom": "GB", "great britain": "GB", "england": "GB",
	"uae": "AE", "united arab emirates": "AE",
	"korea": "KR", "south korea": "KR",
	"russia": "RU", "russian federation": "RU",
	"vietnam": "VN", "viet nam": "VN",
	"czech republic": "CZ", "czechia": "CZ",
	"holland": "NL", "netherlands": "NL",
	"deutschland": "DE", "germany": "DE",
}

var (
	countryOnce sync.Once
	isoToName   map[string]string
	nameToISO   map[string]string
)

func loadCountryMaps() {
	countryOnce.Do(func() {
		isoToName = make(map[string]string, 128)
		nameToISO = make(map[string]string, 256)
		for alias, iso := range countryAliases {
			nameToISO[alias] = iso
		}
		canon := map[string]string{
			"IN": "India", "US": "United States", "GB": "United Kingdom", "CA": "Canada",
			"DE": "Germany", "FR": "France", "AU": "Australia", "JP": "Japan", "SG": "Singapore",
			"NL": "Netherlands", "ES": "Spain", "BR": "Brazil", "MX": "Mexico", "PL": "Poland",
			"CN": "China", "KR": "South Korea", "AE": "United Arab Emirates", "IE": "Ireland",
			"IL": "Israel", "SE": "Sweden", "CH": "Switzerland", "IT": "Italy", "PT": "Portugal",
			"ID": "Indonesia", "PH": "Philippines", "MY": "Malaysia", "TH": "Thailand", "VN": "Vietnam",
			"NZ": "New Zealand", "ZA": "South Africa", "NG": "Nigeria", "EG": "Egypt", "PK": "Pakistan",
			"BD": "Bangladesh", "LK": "Sri Lanka", "NP": "Nepal", "AT": "Austria", "BE": "Belgium",
			"DK": "Denmark", "FI": "Finland", "NO": "Norway", "CZ": "Czechia", "RO": "Romania",
			"HU": "Hungary", "TR": "Turkey", "AR": "Argentina", "CL": "Chile", "CO": "Colombia",
			"TW": "Taiwan", "HK": "Hong Kong", "SA": "Saudi Arabia",
		}
		for iso, name := range canon {
			isoToName[iso] = name
			nameToISO[strings.ToLower(name)] = iso
		}
	})
}

// NormalizeCountry accepts "IN", "India", "india", "United States", "US".
// Returns canonical English name, ISO2 (may be empty for unknown names), and ok.
func NormalizeCountry(s string) (name, iso2 string, ok bool) {
	loadCountryMaps()
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	upper := strings.ToUpper(s)
	if len(upper) == 2 {
		if n, found := isoToName[upper]; found {
			return n, upper, true
		}
	}
	if iso, found := nameToISO[strings.ToLower(s)]; found {
		return isoToName[iso], iso, true
	}
	// Unknown but non-empty: keep as display name for string match.
	return s, "", true
}

// CountryKey is the lowercase lookup key stored for hire countries (prefer ISO2).
func CountryKey(nameOrCode string) string {
	name, iso, ok := NormalizeCountry(nameOrCode)
	if !ok {
		return strings.ToLower(strings.TrimSpace(nameOrCode))
	}
	if iso != "" {
		return strings.ToLower(iso)
	}
	return strings.ToLower(name)
}
