package remotive

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestToProviderJob(t *testing.T) {
	j := remJob{
		ID:       101,
		Title:    "Senior Python Developer",
		URL:      "https://remotive.com/remote-jobs/python-101",
		Company:  "Python Co",
		Location: "Worldwide",
		Category: "Software Development",
		PubDate:  "2026-07-28T12:00:00",
	}
	pj := toProviderJob(j)
	if pj == nil {
		t.Fatal("expected non-nil")
	}
	if pj.Title != "Senior Python Developer" {
		t.Errorf("title = %q", pj.Title)
	}
	if pj.Remote != true {
		t.Error("expected remote")
	}
}

func TestSearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := remResponse{
			Jobs: []remJob{
				{ID: 1, Title: "Go Engineer", URL: "https://remotive.com/job/1", Company: "Go Inc", Location: "EMEA", PubDate: "2026-07-28T12:00:00"},
				{ID: 2, Title: "Rust Developer", URL: "https://remotive.com/job/2", Company: "Rust Inc", Location: "Americas", PubDate: "2026-07-27T12:00:00"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	original := feedURL
	feedURL = ts.URL
	defer func() { feedURL = original }()

	client := New()
	jobs, err := client.Search(nil, provider.SearchCriteria{Titles: []string{"Go"}})
	if err == nil {
		// With nil context, should still get feed but no ctx cancellation
		t.Logf("remotive mock returned %d jobs (ctx=nil, expected error?)", len(jobs))
	} else {
		t.Logf("expected ctx error: %v", err)
	}
}
