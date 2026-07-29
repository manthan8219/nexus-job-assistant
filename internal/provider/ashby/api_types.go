package ashby

// API response types for Ashby GraphQL job board API

type ashbyJobBoard struct {
	JobPostings []ashbyJobPosting `json:"jobPostings"`
}

type ashbyJobPosting struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	LocationName   string `json:"locationName"`
	IsRemote       bool   `json:"isRemote"`
	EmploymentType string `json:"employmentType"`
	ExternalLink   string `json:"externalLink"`
}

type ashbyCompany struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// graphQL response envelope
type ashbyGraphQLResponse struct {
	Data struct {
		JobBoard ashbyJobBoard `json:"jobBoard"`
	} `json:"data"`
}
