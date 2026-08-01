package api

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/engine"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func openTempStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.OpenPath(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// KAN-15: the API server must own an always-on outreach worker and wire the
// engine's OnApplied hook to it.
func TestWireOutreachWorker(t *testing.T) {
	st := openTempStore(t)
	eng := &engine.Engine{}
	wk := wireOutreachWorker(&config.Config{}, st, eng)
	if wk == nil {
		t.Fatal("wireOutreachWorker must create a worker when a store is present")
	}
	if eng.OnApplied == nil {
		t.Fatal("engine.OnApplied must be wired to the worker's auto-queue")
	}
	if wk.Events == nil {
		t.Fatal("worker events channel must be usable")
	}
}

// Without a store there is no worker to run.
func TestWireOutreachWorker_NilStore(t *testing.T) {
	eng := &engine.Engine{}
	if wk := wireOutreachWorker(&config.Config{}, nil, eng); wk != nil {
		t.Fatal("wireOutreachWorker must return nil without a store")
	}
	if eng.OnApplied != nil {
		t.Fatal("engine.OnApplied must stay untouched without a store")
	}
}

// Consent off → the auto-queue is a no-op: nothing is enqueued or processed,
// so no network work happens and nothing appears on the events channel.
func TestWireOutreachWorker_NoConsentNoop(t *testing.T) {
	st := openTempStore(t)
	cfg := &config.Config{OutreachConsent: false, OutreachAutoQueue: true}
	eng := &engine.Engine{}
	wk := wireOutreachWorker(cfg, st, eng)

	eng.OnApplied(store.Application{URL: "https://example.com/j", Company: "Acme", Role: "Engineer"})

	select {
	case <-wk.Events:
		t.Fatal("consent-off auto-queue must not emit events")
	case <-time.After(100 * time.Millisecond):
	}
}
