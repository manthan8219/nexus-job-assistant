package remotive

// API response types for the Remotive public feed
// (https://remotive.com/api/remote-jobs).

type remResponse struct {
	Jobs []remJob `json:"jobs"`
}

type remJob struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Company     string `json:"company_name"`
	Location    string `json:"candidate_required_location"`
	Category    string `json:"category"`
	PubDate     string `json:"publication_date"` // e.g. "2026-07-28T12:00:00"
	Description string `json:"description"`
}
