package hackernews

// API response types for the Algolia HN search and items APIs.

type hnSearchResponse struct {
	Hits []hnSearchHit `json:"hits"`
}

type hnSearchHit struct {
	ObjectID string `json:"objectID"`
	Title    string `json:"title"`
}

type hnItemResponse struct {
	ID       int64     `json:"id"`
	Title    string    `json:"title"`
	Children []hnChild `json:"children"`
}

type hnChild struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
	Text      string `json:"text"`
	Deleted   bool   `json:"deleted"`
	Dead      bool   `json:"dead"`
}
