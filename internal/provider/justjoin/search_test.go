package justjoin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// testSearchServer serves a single page of offers with two entries: one
// external (applyUrl to the employer site) and one internal (JustJoin form).
func testSearchServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jjResponse{
			Data: []jjJob{
				{
					Slug:          "acme-senior-backend-engineer-warszawa-backend",
					Title:         "Senior Backend Engineer",
					CompanyName:   "Acme",
					City:          "Warszawa",
					WorkplaceType: "remote",
					ApplyMethod:   "external",
					ApplyURL:      "https://acme.example.com/jobs/42?utm_source=justjoinit",
				},
				{
					Slug:          "globex-frontend-engineer-warszawa-frontend",
					Title:         "Frontend Engineer",
					CompanyName:   "Globex",
					City:          "Warszawa",
					WorkplaceType: "hybrid",
					ApplyMethod:   "internal",
					ApplyURL:      "",
				},
			},
			Meta: jjMeta{TotalPages: 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSearchUsesExternalApplyURL(t *testing.T) {
	srv := testSearchServer(t)
	c := &Client{http: srv.Client(), baseURL: srv.URL}

	jobs, err := c.Search(context.Background(), provider.SearchCriteria{WorkType: "Remote"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// Both offers are treated as remote (hybrid counts as remote here), so
	// both survive filtering — the test asserts the URL mapping per offer.
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
	if want := "https://acme.example.com/jobs/42?utm_source=justjoinit"; jobs[0].URL != want {
		t.Errorf("external offer URL = %q, want %q", jobs[0].URL, want)
	}
	if want := "https://justjoin.it/job-offer/globex-frontend-engineer-warszawa-frontend"; jobs[1].URL != want {
		t.Errorf("internal offer URL = %q, want %q", jobs[1].URL, want)
	}
}

func TestOfferApplyURL(t *testing.T) {
	tests := []struct {
		name string
		in   jjJob
		want string
	}{
		{
			"external with url",
			jjJob{Slug: "acme-senior-engineer", ApplyMethod: "external", ApplyURL: "https://acme.example.com/jobs/1"},
			"https://acme.example.com/jobs/1",
		},
		{
			"external uppercase method",
			jjJob{Slug: "acme-senior-engineer", ApplyMethod: "External", ApplyURL: "https://acme.example.com/jobs/1"},
			"https://acme.example.com/jobs/1",
		},
		{
			"external without url falls back to offer page",
			jjJob{Slug: "acme-senior-engineer", ApplyMethod: "external", ApplyURL: ""},
			"https://justjoin.it/job-offer/acme-senior-engineer",
		},
		{
			"internal uses offer page",
			jjJob{Slug: "globex-frontend-engineer", ApplyMethod: "internal", ApplyURL: "https://unused.example.com"},
			"https://justjoin.it/job-offer/globex-frontend-engineer",
		},
		{
			"empty method uses offer page",
			jjJob{Slug: "acme-senior-engineer"},
			"https://justjoin.it/job-offer/acme-senior-engineer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := offerApplyURL(tt.in)
			if got != tt.want {
				t.Errorf("offerApplyURL(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
