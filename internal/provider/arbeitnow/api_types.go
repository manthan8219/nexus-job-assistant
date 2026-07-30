package arbeitnow

// API response types for the Arbeitnow public API
// (https://www.arbeitnow.com/api/job-board-api).

type arbResponse struct {
	Data []arbJob `json:"data"`
}

type arbJob struct {
	Slug        string `json:"slug"`
	Company     string `json:"company_name"`
	Title       string `json:"title"`
	Remote      bool   `json:"remote"`
	URL         string `json:"url"`
	Location    string `json:"location"`
	CreatedAt   int64  `json:"created_at"` // epoch seconds
	Description string `json:"description"`
}
