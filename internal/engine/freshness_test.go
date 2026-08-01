package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func TestStaleJob(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		posted     time.Time
		cutoffDays int
		want       bool
	}{
		{"cutoff disabled never stale", now.AddDate(0, 0, -60), 0, false},
		{"unknown posting date never stale", time.Time{}, 7, false},
		{"younger than cutoff", now.AddDate(0, 0, -6), 7, false},
		{"exactly at cutoff not stale", now.AddDate(0, 0, -7), 7, false},
		{"one day past cutoff stale", now.AddDate(0, 0, -8), 7, true},
		{"months old stale", now.AddDate(0, -6, 0), 30, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := provider.Job{PostedAt: tt.posted}
			if got := staleJob(job, now, tt.cutoffDays); got != tt.want {
				t.Errorf("staleJob(posted=%v, cutoff=%d) = %v; want %v", tt.posted, tt.cutoffDays, got, tt.want)
			}
		})
	}
}

func TestSortFreshFirst(t *testing.T) {
	base := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   []provider.Job
		want []string // expected URL order after sorting
	}{
		{"empty", nil, nil},
		{"single", []provider.Job{{URL: "1", PostedAt: base}}, []string{"1"}},
		{
			"newest first",
			[]provider.Job{
				{URL: "old", PostedAt: base.AddDate(0, 0, -10)},
				{URL: "new", PostedAt: base},
				{URL: "mid", PostedAt: base.AddDate(0, 0, -3)},
			},
			[]string{"new", "mid", "old"},
		},
		{
			"unknown posting date last",
			[]provider.Job{
				{URL: "unknown", PostedAt: time.Time{}},
				{URL: "new", PostedAt: base},
			},
			[]string{"new", "unknown"},
		},
		{
			"equal dates keep arrival order",
			[]provider.Job{
				{URL: "a", PostedAt: base},
				{URL: "b", PostedAt: base},
			},
			[]string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortFreshFirst(tt.in)
			var got []string
			for _, j := range tt.in {
				got = append(got, j.URL)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("sortFreshFirst = %v; want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("sortFreshFirst order = %v; want %v", got, tt.want)
					break
				}
			}
		})
	}
}

// Stale cutoff: a job posted older than the configured cutoff is skipped
// without ever calling Apply and is recorded with an honest reason.
func TestProcessJob_StaleJobSkipped(t *testing.T) {
	fp := &fakeProvider{name: "fake"}
	e := newTestEngine(t, &config.Config{ApplyConsent: true, AIAssist: false, StaleJobCutoffDays: 14})
	e.AutoApply = true
	e.DryRun = false
	e.providers = []provider.Provider{fp}
	e.syncApplySafety()
	job := jobFor("fake")
	job.PostedAt = time.Now().AddDate(0, 0, -30)
	_, _ = runProcessJob(t, context.Background(), e, job)
	if fp.applyN != 0 {
		t.Errorf("stale job must not be applied; got %d Apply calls", fp.applyN)
	}
	apps, err := e.store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(apps) != 1 || apps[0].Status != store.StatusSkipped {
		t.Fatalf("stale job should be recorded as skipped; got %+v", apps)
	}
	if !strings.Contains(apps[0].Reason, "stale") {
		t.Errorf("skip reason should mention stale; got %q", apps[0].Reason)
	}
}

// Fresh-cutoff fail-open: a job with no posting date is never skipped even when
// the cutoff is configured.
func TestProcessJob_UnknownPostedAtNotStale(t *testing.T) {
	fp := &fakeProvider{name: "fake"}
	e := newTestEngine(t, &config.Config{ApplyConsent: true, AIAssist: false, StaleJobCutoffDays: 7})
	e.AutoApply = true
	e.DryRun = false
	e.providers = []provider.Provider{fp}
	e.syncApplySafety()
	job := jobFor("fake") // jobFor sets no PostedAt (zero value)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = runProcessJob(t, ctx, e, job)
	if fp.applyN != 1 {
		t.Errorf("unknown posting date must still apply; got %d Apply calls", fp.applyN)
	}
}

// Fresh-job priority: with FreshJobPriority on, discovered jobs are surfaced
// newest-first, which is the order the apply loop then processes them in.
func TestRunOnce_FreshFirstOrder(t *testing.T) {
	base := time.Now()
	cfg := &config.Config{ApplyConsent: true, AIAssist: false, FreshJobPriority: true}
	e := newTestEngine(t, cfg)
	e.providers = []provider.Provider{&fakeProvider{
		name: "fake",
		jobs: []provider.Job{
			{ID: "old", Title: "Backend Engineer", Company: "Acme", URL: "https://example.com/old", Provider: "fake", PostedAt: base.AddDate(0, 0, -9)},
			{ID: "new", Title: "Backend Engineer", Company: "Acme", URL: "https://example.com/new", Provider: "fake", PostedAt: base.AddDate(0, 0, -1)},
		},
	}}
	e.OnlyProvider = "fake"

	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	// RunOnce closes ResultCh on exit, so ranging over it drains every buffered
	// result and terminates (drainResults would spin on the closed channel).
	var found []string
	for r := range e.ResultCh {
		if r.Status == "found" {
			found = append(found, r.Job.URL)
		}
	}
	if len(found) != 2 || found[0] != "https://example.com/new" || found[1] != "https://example.com/old" {
		t.Errorf("found feed order = %v; want [new old]", found)
	}
}
