package store

import (
	"path/filepath"
	"testing"
	"time"
)

func seedAnalyticsApp(t *testing.T, st *Store, provider, url, status string, outcome Outcome, at time.Time) {
	t.Helper()
	if err := st.Insert(Application{
		Provider: provider, Company: "Acme", Role: "Engineer",
		URL: url, Status: Status(status), AppliedAt: at, Outcome: outcome,
	}); err != nil {
		t.Fatalf("seed insert %s: %v", url, err)
	}
}

func openAnalyticsStore(t *testing.T) *Store {
	t.Helper()
	st, err := OpenPath(filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestAnalyticsSnapshot_EmptyDB(t *testing.T) {
	st := openAnalyticsStore(t)
	snap, err := st.AnalyticsSnapshot()
	if err != nil {
		t.Fatalf("AnalyticsSnapshot: %v", err)
	}
	if len(snap.StatusTotals) != 0 {
		t.Errorf("StatusTotals = %v; want empty", snap.StatusTotals)
	}
	if len(snap.PerProvider) != 0 {
		t.Errorf("PerProvider = %v; want empty", snap.PerProvider)
	}
	if len(snap.AppliedLast7Days) != 7 || len(snap.AppliedLast30Days) != 30 {
		t.Errorf("bucket lengths = %d/%d; want 7/30", len(snap.AppliedLast7Days), len(snap.AppliedLast30Days))
	}
	if snap.StatusTotals == nil || snap.PerProvider == nil {
		t.Error("maps/slices must be non-nil for stable JSON")
	}
}

func TestAnalyticsSnapshot_Aggregates(t *testing.T) {
	st := openAnalyticsStore(t)
	now := time.Now()
	seedAnalyticsApp(t, st, "greenhouse", "g1", "applied", OutcomeReplied, now)
	seedAnalyticsApp(t, st, "greenhouse", "g2", "applied", OutcomeNone, now)
	seedAnalyticsApp(t, st, "lever", "l1", "applied", OutcomeInterview, now.AddDate(0, 0, -6))
	seedAnalyticsApp(t, st, "lever", "l2", "skipped", OutcomeNone, now.AddDate(0, 0, -6))
	seedAnalyticsApp(t, st, "remoteok", "r1", "failed", OutcomeNone, now)
	seedAnalyticsApp(t, st, "greenhouse", "g3", "queued", OutcomeNone, now)

	snap, err := st.AnalyticsSnapshot()
	if err != nil {
		t.Fatalf("AnalyticsSnapshot: %v", err)
	}

	if snap.StatusTotals["applied"] != 3 || snap.StatusTotals["skipped"] != 1 ||
		snap.StatusTotals["failed"] != 1 || snap.StatusTotals["queued"] != 1 {
		t.Errorf("StatusTotals = %v; want applied=3 skipped=1 failed=1 queued=1", snap.StatusTotals)
	}

	f := snap.Funnel
	if f.Applied != 3 || f.Replied != 1 || f.Interview != 1 || f.Offer != 0 || f.Rejected != 0 || f.Ghosted != 0 {
		t.Errorf("Funnel = %+v; want applied=3 replied=1 interview=1 rest 0", f)
	}

	if len(snap.PerProvider) != 3 {
		t.Fatalf("PerProvider = %+v; want 3 providers", snap.PerProvider)
	}
	got := map[string]ProviderYield{}
	for _, py := range snap.PerProvider {
		got[py.Provider] = py
	}
	if gh := got["greenhouse"]; gh.Applied != 2 || gh.Replied != 1 {
		t.Errorf("greenhouse yield = %+v; want applied=2 replied=1", gh)
	}
	if lv := got["lever"]; lv.Applied != 1 || lv.Interview != 1 {
		t.Errorf("lever yield = %+v; want applied=1 interview=1", lv)
	}
	if rk := got["remoteok"]; rk.Applied != 0 {
		t.Errorf("remoteok yield = %+v; want applied=0", rk)
	}

	sum7 := 0
	for _, d := range snap.AppliedLast7Days {
		sum7 += d.Count
	}
	if sum7 != 3 { // g1, g2 today + l1 six days ago
		t.Errorf("7-day total = %d; want 3 (buckets %+v)", sum7, snap.AppliedLast7Days)
	}
	sum30 := 0
	for _, d := range snap.AppliedLast30Days {
		sum30 += d.Count
	}
	if sum30 != 3 {
		t.Errorf("30-day total = %d; want 3", sum30)
	}
	if last := snap.AppliedLast7Days[len(snap.AppliedLast7Days)-1]; last.Count != 2 {
		t.Errorf("today bucket = %+v; want count 2", last)
	}
}

func TestAnalyticsSnapshot_ClosedDB(t *testing.T) {
	st, err := OpenPath(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	st.Close()
	if _, err := st.AnalyticsSnapshot(); err == nil {
		t.Error("AnalyticsSnapshot on a closed store must return an error")
	}
}
