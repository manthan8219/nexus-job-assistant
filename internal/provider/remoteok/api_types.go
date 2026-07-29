package remoteok

// API response types for the RemoteOK public feed (https://remoteok.com/api).
// The feed is a flat JSON array; index 0 contains {last_updated, legal}
// metadata and is naturally filtered out by the missing position/url fields.

type rokJob struct {
	ID       string `json:"id"`
	Position string `json:"position"`
	URL      string `json:"url"`
	Company  string `json:"company"`
	Location string `json:"location"`
	Date        string `json:"date"` // ISO 8601
	Description string `json:"description"`
}
