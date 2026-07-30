package lever

// API response types for Lever REST job board API

type leverPosting struct {
	ID               string          `json:"id"`
	Text             string          `json:"text"` // job title
	Categories       leverCategories `json:"categories"`
	ApplyURL         string          `json:"applyUrl"`
	HostedURL        string          `json:"hostedUrl"`
	DescriptionPlain string          `json:"descriptionPlain"`
	DescriptionHTML  string          `json:"description"`
	AdditionalPlain  string          `json:"additionalPlain"`
}

type leverCategories struct {
	Location   string `json:"location"`
	Commitment string `json:"commitment"` // e.g. "Full-time"
}

type leverCompany struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}
