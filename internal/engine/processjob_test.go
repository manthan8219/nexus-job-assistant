package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// runProcessJob calls processJob once with ctx and waits for the background
// scoring goroutine it spawns (so -race sees no late writes), then drains channels.
func runProcessJob(t *testing.T, ctx context.Context, e *Engine, job provider.Job) (counted, stop bool) {
	t.Helper()
	var applied int
	var wg sync.WaitGroup
	c, s := e.processJob(ctx, job, provider.Profile{}, &applied, &wg)
	wg.Wait()
	_ = drainResults(e.ResultCh)
	return c, s
}

// Dry-run honesty (AGENTS.md section 14): --dry-run must guarantee zero submissions.
func TestProcessJob_DryRunNeverSubmits(t *testing.T) {
	fp := &fakeProvider{name: "fake"}
	e := newTestEngine(t, &config.Config{ApplyConsent: true, AIAssist: false})
	e.AutoApply = true
	e.DryRun = true
	e.providers = []provider.Provider{fp}
	e.syncApplySafety()
	_, _ = runProcessJob(t, context.Background(), e, jobFor("fake"))
	if fp.applyN != 0 {
		t.Errorf("dry-run must not call Apply; got %d calls", fp.applyN)
	}
}

// Consent is untouchable: without ApplyConsent, no Apply is ever called.
func TestProcessJob_NoConsentNeverSubmits(t *testing.T) {
	fp := &fakeProvider{name: "fake"}
	e := newTestEngine(t, &config.Config{ApplyConsent: false, AIAssist: false})
	e.AutoApply = true
	e.DryRun = false
	e.providers = []provider.Provider{fp}
	e.syncApplySafety()
	if e.AutoApply {
		t.Fatal("AutoApply must be false without consent")
	}
	_, _ = runProcessJob(t, context.Background(), e, jobFor("fake"))
	if fp.applyN != 0 {
		t.Errorf("without consent Apply must not be called; got %d calls", fp.applyN)
	}
}

// Idempotency: an already-applied URL is never re-applied (store checked first).
func TestProcessJob_AlreadyAppliedIsSkipped(t *testing.T) {
	fp := &fakeProvider{name: "fake"}
	e := newTestEngine(t, &config.Config{ApplyConsent: true, AIAssist: false})
	e.AutoApply = true
	e.DryRun = false
	e.providers = []provider.Provider{fp}
	e.syncApplySafety()
	job := jobFor("fake")
	if err := e.store.Insert(store.Application{
		Provider: job.Provider, Company: job.Company, Role: job.Title,
		URL: job.URL, Status: store.StatusApplied, AppliedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed insert: %v", err)
	}
	counted, _ := runProcessJob(t, context.Background(), e, job)
	if fp.applyN != 0 {
		t.Errorf("already-applied URL must not be re-applied; got %d calls", fp.applyN)
	}
	if counted {
		t.Error("skipped (already applied) job must not count toward the run limit")
	}
}

// Daily cap: once MaxAppsPerDay is reached, Apply is not called and the run stops.
// Seed AppliedAt = now (today) so the row is unambiguously within processJob's
// "since local-midnight" window.
func TestProcessJob_DailyCapStopsRun(t *testing.T) {
	fp := &fakeProvider{name: "fake"}
	e := newTestEngine(t, &config.Config{ApplyConsent: true, AIAssist: false, MaxAppsPerDay: 1})
	e.AutoApply = true
	e.DryRun = false
	e.providers = []provider.Provider{fp}
	e.syncApplySafety()
	if err := e.store.Insert(store.Application{
		Provider: "fake", Company: "Acme", Role: "X",
		URL: "https://example.com/fake/earlier", Status: store.StatusApplied, AppliedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed insert: %v", err)
	}
	_, stop := runProcessJob(t, context.Background(), e, jobFor("fake"))
	if fp.applyN != 0 {
		t.Errorf("at daily cap Apply must not be called; got %d calls", fp.applyN)
	}
	if !stop {
		t.Error("hitting the daily cap must signal stopRun=true")
	}
}

// Happy path: consent + not-yet-applied -> exactly one Apply, recorded in store.
// A pre-cancelled context makes processJob skip the post-apply humanDelay sleep
// (its select takes ctx.Done), keeping the test fast and deterministic while
// still exercising the real apply + record path.
func TestProcessJob_HappyApply(t *testing.T) {
	fp := &fakeProvider{name: "fake"}
	e := newTestEngine(t, &config.Config{ApplyConsent: true, AIAssist: false})
	e.AutoApply = true
	e.DryRun = false
	e.providers = []provider.Provider{fp}
	e.syncApplySafety()
	job := jobFor("fake")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	counted, _ := runProcessJob(t, ctx, e, job)
	if fp.applyN != 1 {
		t.Errorf("happy path must call Apply exactly once; got %d", fp.applyN)
	}
	if !counted {
		t.Error("a successful apply must count toward the run limit")
	}
	exists, err := e.store.Exists(job.URL)
	if err != nil || !exists {
		t.Errorf("applied URL should be recorded; exists=%v err=%v", exists, err)
	}
}
