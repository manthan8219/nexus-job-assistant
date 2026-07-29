package hackernews

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

const browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

const searchURL = "https://hn.algolia.com/api/v1/search_by_date?tags=story&query=Ask%20HN%20Who%20is%20hiring&hitsPerPage=5"

// searchThread finds the latest "Ask HN: Who is hiring?" thread ID.
func searchThread(ctx context.Context, client *http.Client) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", browserUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HN search: HTTP %d", resp.StatusCode)
	}

	var body hnSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", fmt.Errorf("HN search decode: %w", err)
	}

	const matchRE = `(?i)ask\s+hn[:\s]+who\s+is\s+hiring`
	re := regexp.MustCompile(matchRE)
	for _, hit := range body.Hits {
		if hit.ObjectID != "" && re.MatchString(hit.Title) {
			return hit.ObjectID, "https://news.ycombinator.com/item?id=" + hit.ObjectID, nil
		}
	}
	return "", "", fmt.Errorf("no 'Who is hiring?' thread found in recent HN stories")
}

// fetchThread retrieves the HN item with top-level children (job comments).
func fetchThread(ctx context.Context, client *http.Client, threadID string) (*hnItemResponse, error) {
	url := fmt.Sprintf("https://hn.algolia.com/api/v1/items/%s", threadID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HN item %s: HTTP %d", threadID, resp.StatusCode)
	}

	var item hnItemResponse
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("HN item %s decode: %w", threadID, err)
	}
	return &item, nil
}
