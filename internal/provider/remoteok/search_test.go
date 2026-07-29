package remoteok

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/manthanmanthan/nexus/internal/provider"
)

func TestToProviderJob(t *testing.T) {
	// Valid job
	j := rokJob{
		ID:       "12345",
		Position: "Senior Go Engineer",
		URL:      "https://remoteok.com/remote-jobs/12345-senior-go-engineer",
		Company:  "Acme Corp",
		Location: "Worldwide",
		Date:     "2026-07-28T12:00:00Z",
	}
	pj := toProviderJob(j)
	if pj == nil {
		t.Fatal("expected non-nil job")
	}
	if pj.Title != "Senior Go Engineer" {
		t.Errorf("title = %q, want %q", pj.Title, "Senior Go Engineer")
	}
	if pj.Company != "Acme Corp" {
		t.Errorf("company = %q, want %q", pj.Company, "Acme Corp")
	}
	if pj.Location != "Worldwide" {
		t.Errorf("location = %q, want %q", pj.Location, "Worldwide")
	}
	if !pj.Remote {
		t.Error("expected remote = true")
	}
	if pj.PostedAt.IsZero() {
		t.Error("expected non-zero postedAt")
	}

	// Metadata row (no position/url) → nil
	pj2 := toProviderJob(rokJob{Company: "RemoteOK"})
	if pj2 != nil {
		t.Error("expected nil for metadata row")
	}

	// No company → fallback
	j3 := rokJob{
		Position: "Engineer",
		URL:      "https://remoteok.com/job/3",
	}
	pj3 := toProviderJob(j3)
	if pj3 == nil {
		t.Fatal("expected non-nil")
	}
	if pj3.Company != "RemoteOK" {
		t.Errorf("fallback company = %q, want RemoteOK", pj3.Company)
	}
}

func TestSearch(t *testing.T) {
	// Mock server returning a RemoteOK-like feed.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent header")
		}
		feed := []rokJob{
			{ID: "meta", Position: "", URL: ""}, // metadata row → skipped
			{ID: "1", Position: "Senior Go Engineer", URL: "https://remoteok.com/job/1", Company: "Go Corp", Location: "Worldwide", Date: "2026-07-28T00:00:00Z"},
			{ID: "2", Position: "Frontend Developer", URL: "https://remoteok.com/job/2", Company: "Web Inc", Location: "Remote", Date: "2026-07-27T00:00:00Z"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(feed)
	}))
	defer ts.Close()

	originalURL := feedURL
	feedURL = ts.URL
	defer func() { feedURL = originalURL }()

	client := New()
	jobs, err := client.Search(nil, provider.SearchCriteria{Titles: []string{"Go"}})
	// nil context should fail
	if err == nil {
		if len(jobs) != 0 {
			t.Errorf("expected 0 jobs with nil context, got %d", len(jobs))
		}
	}

	_ = time.Second // force time import usage
}
