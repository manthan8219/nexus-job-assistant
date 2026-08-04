package workatastartup

import (
	"context"
	"errors"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

func TestName(t *testing.T) {
	c := New()
	if c.Name() != "workatastartup" {
		t.Errorf("Name() = %q; want \"workatastartup\"", c.Name())
	}
}

func TestSearch(t *testing.T) {
	cases := []struct {
		name     string
		criteria provider.SearchCriteria
		scrape   scrapeFn
		wantJobs int
	}{
		{
			name:     "happy path returns matching remote jobs",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Remote"},
			scrape: func(_ context.Context, _, _ string, _ []string, _ bool) ([]scraper.BoardJob, error) {
				return []scraper.BoardJob{
					{Title: "Founding Engineer", Company: "YC Startup", Location: "Remote", ApplyURL: "https://example.com/jobs/1", Remote: true},
					{Title: "Engineer", Company: "AnotherCo", Location: "Remote", ApplyURL: "https://example.com/jobs/2", Remote: true},
				}, nil
			},
			wantJobs: 2,
		},
		{
			name:     "title filter excludes non-matching",
			criteria: provider.SearchCriteria{Titles: []string{"Backend"}, WorkType: "Remote"},
			scrape: func(_ context.Context, _, _ string, _ []string, _ bool) ([]scraper.BoardJob, error) {
				return []scraper.BoardJob{
					{Title: "Backend Engineer", Company: "A", Location: "Remote", ApplyURL: "https://example.com/jobs/1"},
					{Title: "Designer", Company: "B", Location: "Remote", ApplyURL: "https://example.com/jobs/2"},
				}, nil
			},
			wantJobs: 1,
		},
		{
			name:     "scrape error does not abort run",
			criteria: provider.SearchCriteria{Titles: []string{"Engineer"}, WorkType: "Remote"},
			scrape: func(_ context.Context, _, _ string, _ []string, _ bool) ([]scraper.BoardJob, error) {
				return nil, errors.New("CDP not available")
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

func TestApply(t *testing.T) {
	c := New()
	res, err := c.Apply(context.Background(), provider.Job{URL: "https://example.com/job/1"}, provider.Profile{})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if res.Status != "skipped" {
		t.Errorf("Apply status = %q; want \"skipped\"", res.Status)
	}
}
