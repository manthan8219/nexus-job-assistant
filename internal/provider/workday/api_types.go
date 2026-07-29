package workday

// API response types for the Workday CXS public jobs endpoint.

// wdayRequestBody is the POST body sent to the CXS API.
type wdayRequestBody struct {
	AppliedFacets struct{} `json:"appliedFacets"`
	Limit         int      `json:"limit"`
	Offset        int      `json:"offset"`
	SearchText    string   `json:"searchText"`
}

// wdayResponse is the paginated response from the CXS API.
type wdayResponse struct {
	Total       int            `json:"total"`
	JobPostings []wdayPosting  `json:"jobPostings"`
}

type wdayPosting struct {
	Title         string `json:"title"`
	ExternalPath  string `json:"externalPath"`
	LocationsText string `json:"locationsText"`
	PostedOn      string `json:"postedOn"` // e.g. "Posted Today", "Posted 5 Days Ago"
	BulletFields  []string `json:"bulletFields,omitempty"`
}

// wdayCompany is an entry in the companies JSON list.
type wdayCompany struct {
	Name string `json:"name"`
	URL  string `json:"url"` // e.g. "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite"
}
