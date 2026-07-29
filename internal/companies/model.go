package companies

import "time"

// Company is one employer we can scan / filter by country footprint.
type Company struct {
	ID               int64
	Name             string
	Website          string
	ATS              string   // greenhouse, lever, ashby, workable, … or empty
	Board            string   // ATS board slug / token
	BoardURL         string   // canonical careers/ATS URL
	HireCountries    []string // display names e.g. "India"
	HireCountryCodes []string // ISO2 e.g. "IN"
	HQCountry        string   // optional; empty until enriched
	HQCountryCode    string
	Kind             string // "", "startup", "tech", "gaming", …
	Industry         string
	Source           string // openjobs, manual, observed, …
	UpdatedAt        time.Time
}
