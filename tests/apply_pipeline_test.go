// Package tests — black-box integration tests for the apply pipeline
// (engine + store + config through public APIs only, hermetic: temp-dir
// stores, no network, no real submissions). The guards asserted here are the
// non-negotiable apply-safety invariants from AGENTS.md section 14.
package tests

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/engine"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// newTestStore opens a hermetic store in a temp dir.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.OpenPath(filepath.Join(t.TempDir(), "apply.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// newTestEngine wires a real Engine (all registered providers) around a
// hermetic store and config. Provider constructors make no network calls, so
// this is safe to build in a test.
func newTestEngine(t *testing.T, cfg *config.Config, st *store.Store) *engine.Engine {
	t.Helper()
	e, err := engine.New(cfg, st, "")
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	return e
}

// insertQueued inserts one queued application and returns its id.
func insertQueued(t *testing.T, st *store.Store, providerName string) int64 {
	t.Helper()
	url := "https://example.com/job-" + providerName
	if err := st.Insert(store.Application{
		Provider: providerName, Company: "Acme", Role: "Engineer",
		URL: url, Status: store.StatusQueued,
		AppliedAt: time.Now(), Location: "Remote", Remote: true,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	apps, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, a := range apps {
		if a.URL == url {
			return a.ID
		}
	}
	t.Fatalf("inserted application %q not found in List", url)
	return 0
}

// appStatus reads the stored status for one application id.
func appStatus(t *testing.T, st *store.Store, id int64) store.Status {
	t.Helper()
	apps, err := st.GetByIDs([]int64{id})
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("GetByIDs returned %d apps, want 1", len(apps))
	}
	return apps[0].Status
}

// Consent is mandatory: without it the apply pipeline must block before any
// provider is reached and the stored job must remain queued.
func TestApplyPipeline_BlocksWithoutConsent(t *testing.T) {
	st := newTestStore(t)
	e := newTestEngine(t, &config.Config{ApplyConsent: false}, st)
	id := insertQueued(t, st, "greenhouse")

	if err := e.ApplySelected(context.Background(), []int64{id}); err == nil {
		t.Fatal("ApplySelected must block without consent")
	}
	if got := appStatus(t, st, id); got != store.StatusQueued {
		t.Errorf("status = %q, want %q (no state change without consent)", got, store.StatusQueued)
	}
}

// Dry run guarantees zero submissions: ApplySelected must refuse to run.
func TestApplyPipeline_BlocksWhileDryRun(t *testing.T) {
	st := newTestStore(t)
	e := newTestEngine(t, &config.Config{ApplyConsent: true}, st)
	e.DryRun = true
	id := insertQueued(t, st, "greenhouse")

	if err := e.ApplySelected(context.Background(), []int64{id}); err == nil {
		t.Fatal("ApplySelected must block while dry run is active")
	}
	if got := appStatus(t, st, id); got != store.StatusQueued {
		t.Errorf("status = %q, want %q", got, store.StatusQueued)
	}
}

// Idempotency: an already-applied job is never applied twice — the status
// must not change and the call must succeed (a no-op, not an error).
func TestApplyPipeline_SkipsAlreadyApplied(t *testing.T) {
	st := newTestStore(t)
	e := newTestEngine(t, &config.Config{ApplyConsent: true}, st)

	if err := st.Insert(store.Application{
		Provider: "greenhouse", Company: "Acme", Role: "Engineer",
		URL: "https://example.com/already", Status: store.StatusApplied,
		AppliedAt: time.Now(),
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	apps, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	id := apps[0].ID

	if err := e.ApplySelected(context.Background(), []int64{id}); err != nil {
		t.Fatalf("ApplySelected: %v", err)
	}
	if got := appStatus(t, st, id); got != store.StatusApplied {
		t.Errorf("status = %q, want %q (already-applied job must stay untouched)", got, store.StatusApplied)
	}
}

// The daily cap is a hard limit: once today's quota is spent, additional
// queued jobs must stay queued.
func TestApplyPipeline_RespectsDailyCap(t *testing.T) {
	st := newTestStore(t)
	e := newTestEngine(t, &config.Config{ApplyConsent: true, MaxAppsPerDay: 1}, st)

	// One application already today fills the cap.
	if err := st.Insert(store.Application{
		Provider: "greenhouse", Company: "Acme", Role: "Existing",
		URL: "https://example.com/today", Status: store.StatusApplied,
		AppliedAt: time.Now(),
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	id := insertQueued(t, st, "greenhouse")

	if err := e.ApplySelected(context.Background(), []int64{id}); err != nil {
		t.Fatalf("ApplySelected: %v", err)
	}
	if got := appStatus(t, st, id); got != store.StatusQueued {
		t.Errorf("status = %q, want %q (daily cap must block the apply)", got, store.StatusQueued)
	}
}

// A queued job whose provider is not registered must fail safely: no panic,
// no submission, and the store records the failure so the user can see why.
func TestApplyPipeline_UnknownProviderFailsSafely(t *testing.T) {
	st := newTestStore(t)
	e := newTestEngine(t, &config.Config{ApplyConsent: true}, st)
	id := insertQueued(t, st, "nobody")

	if err := e.ApplySelected(context.Background(), []int64{id}); err != nil {
		t.Fatalf("ApplySelected: %v", err)
	}
	if got := appStatus(t, st, id); got != store.StatusFailed {
		t.Errorf("status = %q, want %q (unknown provider must fail safely)", got, store.StatusFailed)
	}
}

// recordingNotifier records every event delivered to it.
type recordingNotifier struct {
	events []notifier.Event
}

func (n *recordingNotifier) Name() string { return "recorder" }

func (n *recordingNotifier) Send(_ context.Context, ev notifier.Event) error {
	n.events = append(n.events, ev)
	return nil
}

// The pipeline notifies on outcomes: an unknown provider failure must emit a
// job_failed event carrying the provider name.
func TestApplyPipeline_UnknownProviderEmitsFailureEvent(t *testing.T) {
	st := newTestStore(t)
	e := newTestEngine(t, &config.Config{ApplyConsent: true}, st)
	rec := &recordingNotifier{}
	e.Notifier = notifier.MultiNotifier{rec}
	id := insertQueued(t, st, "nobody")

	if err := e.ApplySelected(context.Background(), []int64{id}); err != nil {
		t.Fatalf("ApplySelected: %v", err)
	}
	for _, ev := range rec.events {
		if ev.Kind == notifier.EventJobFailed && ev.Provider == "nobody" {
			return
		}
	}
	t.Errorf("expected a job_failed notifier event for provider %q; got %+v", "nobody", rec.events)
}

// The engine must register every always-on provider (including the new
// board-wide job finders) and must NOT register key-gated providers without
// credentials (graceful degradation).
func TestEngineNew_RegistersProviders(t *testing.T) {
	st := newTestStore(t)
	e := newTestEngine(t, &config.Config{}, st)

	registered := map[string]bool{}
	for _, name := range e.ProviderNames() {
		registered[name] = true
	}

	alwaysOn := []string{
		// Apply-capable ATS providers.
		"greenhouse", "lever", "ashby", "workable", "smartrecruiters", "recruitee",
		// New board-wide job finders.
		"remoteco", "dynamitejobs", "euroremotejobs",
	}
	for _, want := range alwaysOn {
		if !registered[want] {
			t.Errorf("provider %q must be registered", want)
		}
	}

	keyGated := []string{"adzuna", "usajobs", "jooble", "careerjet"}
	for _, absent := range keyGated {
		if registered[absent] {
			t.Errorf("key-gated provider %q must not register without credentials", absent)
		}
	}
}
