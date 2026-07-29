package jobicy

// API response types for the Jobicy public feed
// (https://jobicy.com/api/v2/remote-jobs).

type jcyResponse struct {
	Jobs []jcyJob `json:"jobs"`
}

type jcyJob struct {
	ID       int      `json:"id"`
	Title    string   `json:"jobTitle"`
	URL      string   `json:"url"`
	Company  string   `json:"companyName"`
	Geo      string   `json:"jobGeo"`
	PubDate  string   `json:"pubDate"` // ISO 8601
	Industry     []string `json:"jobIndustry,omitempty"`
	Description string   `json:"jobDescription"`
}
