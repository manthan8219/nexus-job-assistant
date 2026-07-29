package workable

// API response types for Workable job board API

type workableJobsResponse struct {
	Results []workableJob `json:"results"`
}

type workableJob struct {
	ID             string           `json:"id"`
	Title          string           `json:"title"`
	Shortcode      string           `json:"shortcode"`
	Location       workableLocation `json:"location"`
	EmploymentType string           `json:"employment_type"`
	URL            string           `json:"url"`
}

type workableLocation struct {
	City    string `json:"city"`
	Country string `json:"country"`
	Remote  bool   `json:"remote"`
}

type workableCompany struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}
