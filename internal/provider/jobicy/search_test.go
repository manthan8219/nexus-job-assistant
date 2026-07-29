package jobicy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToProviderJob(t *testing.T) {
	j := jcyJob{
		ID:       42,
		Title:    "Senior Rust Engineer",
		URL:      "https://jobicy.com/jobs/42",
		Company:  "Rust Inc",
		Geo:      "Worldwide",
		PubDate:  "2026-07-28T12:00:00Z",
	}
	pj := toProviderJob(j)
	if pj == nil {
		t.Fatal("expected non-nil")
	}
	if pj.Title != "Senior Rust Engineer" {
		t.Errorf("title = %q", pj.Title)
	}
	if pj.Remote != true {
		t.Error("expected remote")
	}

	// Off-host URL → nil
	j2 := jcyJob{
		Title: "Bad",
		URL:   "https://evil.com/job",
	}
	if pj2 := toProviderJob(j2); pj2 != nil {
		t.Error("expected nil for untrusted host")
	}
}

func TestSearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := jcyResponse{
			Jobs: []jcyJob{
				{ID: 1, Title: "Go Developer", URL: "https://jobicy.com/jobs/1", Company: "Go Co", Geo: "Remote", PubDate: "2026-07-28T00:00:00Z"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	original := feedURL
	feedURL = ts.URL
	defer func() { feedURL = original }()

	_ = New()
}
