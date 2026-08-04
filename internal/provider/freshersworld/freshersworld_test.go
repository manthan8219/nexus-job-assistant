package freshersworld

import (
	"context"
	"errors"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

func TestSearch(t *testing.T) {
	cases := []struct {
		name     string
		criteria provider.SearchCriteria
		scrape   scrapeFn
		wantJobs int
	}{
		{
			name:     "happy path returns matching jobs",
			criteria: provider.SearchCriteria{Titles: []string{"Software Engineer"}, WorkType: "Remote"},
			scrape: func(_ context.Context, _, _ string, _ []string, _ bool) ([]scraper.BoardJob, error) {
				return []scraper.BoardJob{
					{Title: "Software Engineer", Company: "TCS", Location: "Remote", ApplyURL: "https://www.freshersworld.com/jobs/1", Remote: true},
					{Title: "Software Engineer II", Company: "Infosys", Location: "Remote, India", ApplyURL: "https://www.freshersworld.com/jobs/2", Remote: true},
				}, nil
			},
			wantJobs: 2,
		},
		{
			name:     "title filter excludes non-matching jobs",
			criteria: provider.SearchCriteria{Titles: []string{"Backend"}, WorkType: "Remote"},
			scrape: func(_ context.Context, _, _ string, _ []string, _ bool) ([]scraper.BoardJob, error) {
				return []scraper.BoardJob{
					{Title: "Backend Developer", Company: "A", Location: "Remote", ApplyURL: "https://fw.com/jobs/1"},
					{Title: "Marketing Manager", Company: "B", Location: "Remote", ApplyURL: "https://fw.com/jobs/2"},
				}, nil
			},
			wantJobs: 1,
		},
		{
			name:     "onsite location filter",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Onsite", Locations: []string{"Bangalore"}},
			scrape: func(_ context.Context, _, _ string, _ []string, _ bool) ([]scraper.BoardJob, error) {
				return []scraper.BoardJob{
					{Title: "Engineer", Company: "A", Location: "Bangalore", ApplyURL: "https://fw.com/jobs/1"},
					{Title: "Engineer", Company: "B", Location: "Mumbai", ApplyURL: "https://fw.com/jobs/2"},
				}, nil
			},
			wantJobs: 1,
		},
		{
			name:     "job without url is skipped",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Remote"},
			scrape: func(_ context.Context, _, _ string, _ []string, _ bool) ([]scraper.BoardJob, error) {
				return []scraper.BoardJob{{Title: "Engineer", Company: "A", Location: "Remote", ApplyURL: ""}}, nil
			},
			wantJobs: 0,
		},
		{
			name:     "scrape error does not abort run",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Remote"},
			scrape: func(_ context.Context, _, _ string, _ []string, _ bool) ([]scraper.BoardJob, error) {
				return nil, errors.New("scraper unavailable")
			},
			wantJobs: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{scrape: tc.scrape}
			jobs, err := c.Search(context.Background(), tc.criteria)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(jobs) != tc.wantJobs {
				t.Errorf("got %d jobs, want %d", len(jobs), tc.wantJobs)
			}
		})
	}
}

func TestSearchDedupAcrossKeywords(t *testing.T) {
	called := 0
	c := &Client{scrape: func(_ context.Context, _ string, _ string, _ []string, _ bool) ([]scraper.BoardJob, error) {
		called++
		return []scraper.BoardJob{
			{Title: "Engineer", Company: "A", Location: "Remote", ApplyURL: "https://fw.com/jobs/1"},
		}, nil
	}}
	jobs, err := c.Search(context.Background(), provider.SearchCriteria{Titles: []string{"Engineer", "Developer"}, WorkType: "Remote"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("got %d jobs, want 1 (deduped)", len(jobs))
	}
	if called != 2 {
		t.Errorf("scrape called %d times, want 2", called)
	}
}

func TestSearchContextCancel(t *testing.T) {
	c := &Client{scrape: func(ctx context.Context, _, _ string, _ []string, _ bool) ([]scraper.BoardJob, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Search(ctx, provider.SearchCriteria{Titles: []string{"Go"}, WorkType: "Remote"})
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

func TestApplySkipped(t *testing.T) {
	c := New()
	res, err := c.Apply(context.Background(), provider.Job{URL: "https://www.freshersworld.com/jobs/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != "skipped" {
		t.Errorf("status = %q, want skipped", res.Status)
	}
}

func TestToProviderJobFallbackCompany(t *testing.T) {
	pj := toProviderJob(scraper.BoardJob{Title: "Engineer", ApplyURL: "https://fw.com/1"}, "freshersworld")
	if pj == nil {
		t.Fatal("expected non-nil")
	}
	if pj.Company != "Freshersworld" {
		t.Errorf("company = %q, want Freshersworld", pj.Company)
	}
}
