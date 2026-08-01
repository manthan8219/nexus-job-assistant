package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// newEngineWithProviders builds a hermetic Engine with the given providers and
// returns it alongside its temp-dir store.
func newEngineWithProviders(t *testing.T, cfg *config.Config, provs []provider.Provider) (*Engine, *store.Store) {
	t.Helper()
	st, err := store.OpenAt(filepath.Join(t.TempDir(), "apps.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if cfg.MaxAppsPerRun <= 0 {
		cfg.MaxAppsPerRun = 10
	}
	if cfg.MaxAppsPerDay <= 0 {
		cfg.MaxAppsPerDay = 25
	}
	if cfg.ApplyDelaySec < 0 {
		cfg.ApplyDelaySec = 0
	}
	return &Engine{
		cfg:        cfg,
		store:      st,
		providers:  provs,
		MaxPerRun:  10,
		MinDelay:   0,
		LogCh:      make(chan string, 100),
		ResultCh:   make(chan Result, 100),
		ProgressCh: make(chan ProviderProgress, 100),
		// No real pause between applies in tests.
		applyDelay: func(int) time.Duration { return 0 },
	}, st
}

func seedQueued(t *testing.T, st *store.Store, providerName string) int64 {
	t.Helper()
	now := time.Now().UTC()
	app := store.Application{
		Provider: providerName, Company: "Acme", Role: "Engineer",
		URL: "https://example.com/job-" + providerName, Status: store.StatusQueued,
		AppliedAt: now, Location: "Remote", Remote: true,
	}
	if err := st.Insert(app); err != nil {
		t.Fatal(err)
	}
	apps, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	return apps[0].ID
}

func TestApplySelectedGuards(t *testing.T) {
	cfg := &config.Config{}
	fake := &fakeProvider{name: "greenhouse", result: provider.ApplyResult{Status: "applied"}}

	t.Run("blocks without consent", func(t *testing.T) {
		e, st := newEngineWithProviders(t, cfg, []provider.Provider{fake})
		id := seedQueued(t, st, "greenhouse")
		err := e.ApplySelected(context.Background(), []int64{id})
		if err == nil {
			t.Fatal("expected consent error")
		}
		if len(fake.applied) != 0 {
			t.Errorf("Apply called %d times; want 0", len(fake.applied))
		}
	})

	t.Run("blocks while dry run is active", func(t *testing.T) {
		withConsent := &config.Config{ApplyConsent: true}
		e, st := newEngineWithProviders(t, withConsent, []provider.Provider{fake})
		e.DryRun = true
		id := seedQueued(t, st, "greenhouse")
		if err := e.ApplySelected(context.Background(), []int64{id}); err == nil {
			t.Fatal("expected dry-run error")
		}
		if len(fake.applied) != 0 {
			t.Errorf("Apply called %d times; want 0", len(fake.applied))
		}
	})

	t.Run("skips already-applied jobs (idempotent)", func(t *testing.T) {
		withConsent := &config.Config{ApplyConsent: true}
		e, st := newEngineWithProviders(t, withConsent, []provider.Provider{fake})
		now := time.Now().UTC()
		if err := st.Insert(store.Application{
			Provider: "greenhouse", Company: "Acme", Role: "Engineer",
			URL: "https://example.com/already", Status: store.StatusApplied,
			AppliedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		apps, _ := st.List()
		if err := e.ApplySelected(context.Background(), []int64{apps[0].ID}); err != nil {
			t.Fatalf("ApplySelected: %v", err)
		}
		if len(fake.applied) != 0 {
			t.Errorf("Apply called %d times; want 0 (already applied)", len(fake.applied))
		}
	})

	t.Run("respects the daily cap", func(t *testing.T) {
		withConsent := &config.Config{ApplyConsent: true, MaxAppsPerDay: 1}
		e, st := newEngineWithProviders(t, withConsent, []provider.Provider{fake})
		// One application already today fills the cap.
		now := time.Now().UTC()
		if err := st.Insert(store.Application{
			Provider: "greenhouse", Company: "Acme", Role: "Existing",
			URL: "https://example.com/today", Status: store.StatusApplied,
			AppliedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		id := seedQueued(t, st, "greenhouse")
		if err := e.ApplySelected(context.Background(), []int64{id}); err != nil {
			t.Fatalf("ApplySelected: %v", err)
		}
		if len(fake.applied) != 0 {
			t.Errorf("Apply called %d times; want 0 (daily cap)", len(fake.applied))
		}
	})
}

func TestApplySelectedSubmits(t *testing.T) {
	cfg := &config.Config{ApplyConsent: true}
	fake := &fakeProvider{name: "lever", result: provider.ApplyResult{Status: "applied"}}
	e, st := newEngineWithProviders(t, cfg, []provider.Provider{fake})
	id := seedQueued(t, st, "lever")

	if err := e.ApplySelected(context.Background(), []int64{id}); err != nil {
		t.Fatalf("ApplySelected: %v", err)
	}
	if len(fake.applied) != 1 {
		t.Fatalf("Apply called %d times; want 1", len(fake.applied))
	}
	if fake.applied[0].Company != "Acme" || fake.applied[0].Title != "Engineer" {
		t.Errorf("applied job = %+v; want Acme/Engineer", fake.applied[0])
	}

	got, err := st.GetByIDs([]int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != store.StatusApplied {
		t.Errorf("status = %q; want applied", got[0].Status)
	}
	if got[0].Approved {
		t.Error("approved flag should be cleared after apply")
	}
}

func TestApplySelectedUnknownProviderFailsSafely(t *testing.T) {
	cfg := &config.Config{ApplyConsent: true}
	e, st := newEngineWithProviders(t, cfg, nil) // no providers registered
	id := seedQueued(t, st, "nobody")

	if err := e.ApplySelected(context.Background(), []int64{id}); err != nil {
		t.Fatalf("ApplySelected: %v", err)
	}
	got, err := st.GetByIDs([]int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != store.StatusFailed {
		t.Errorf("status = %q; want failed", got[0].Status)
	}
}
