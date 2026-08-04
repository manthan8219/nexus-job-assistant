package engine

import (
	"context"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// recordingNotifier captures every event delivered to it, so tests can assert
// exactly which notifications (and therefore which emails) a run produces.
type recordingNotifier struct {
	events []notifier.Event
}

func (n *recordingNotifier) Name() string { return "recorder" }
func (n *recordingNotifier) Send(_ context.Context, ev notifier.Event) error {
	n.events = append(n.events, ev)
	return nil
}

func kindsOf(evs []notifier.Event) []notifier.EventKind {
	out := make([]notifier.EventKind, 0, len(evs))
	for _, e := range evs {
		out = append(out, e.Kind)
	}
	return out
}

func hasKind(evs []notifier.Event, k notifier.EventKind) bool {
	for _, e := range evs {
		if e.Kind == k {
			return true
		}
	}
	return false
}

// The daily scheduler fires a DRY RUN (startEngineRun(true, false, nil)).
// This test proves what notification events — and therefore what emails —
// that run produces: run-level events only, never per-job "applied" emails,
// because nothing is actually submitted in a dry run.
func TestRunOnce_ScheduledDryRunEmitsRunEventsOnly(t *testing.T) {
	eng := newTestEngine(t, &config.Config{})
	eng.DryRun = true
	rec := &recordingNotifier{}
	eng.Notifier = notifier.MultiNotifier{rec}
	eng.providers = []provider.Provider{
		&fakeProvider{name: "a", jobs: []provider.Job{jobFor("a")}},
		&fakeProvider{name: "b", jobs: []provider.Job{jobFor("b")}},
	}

	if err := eng.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	// The email notifier renders EventRunStarted as no email ("") and
	// EventJobApplied/Failed as per-job emails — so a dry run must NOT emit
	// any per-job events.
	if hasKind(rec.events, notifier.EventJobApplied) {
		t.Errorf("dry run emitted a job_applied event; got %v", kindsOf(rec.events))
	}
	if hasKind(rec.events, notifier.EventJobFailed) {
		t.Errorf("dry run emitted a job_failed event; got %v", kindsOf(rec.events))
	}
	if !hasKind(rec.events, notifier.EventRunStarted) {
		t.Errorf("dry run must emit run_started; got %v", kindsOf(rec.events))
	}

	var complete *notifier.Event
	for i := range rec.events {
		if rec.events[i].Kind == notifier.EventRunComplete {
			complete = &rec.events[i]
			break
		}
	}
	if complete == nil {
		t.Fatalf("dry run must emit run_complete; got %v", kindsOf(rec.events))
	}
	// The dry-run run-complete event must be honest: nothing was submitted.
	// Scanned counts the processed jobs (the "Scanned: 12 · Submitted: 0 (dry
	// run)" email), while TotalApplied stays 0.
	if !complete.DryRun {
		t.Error("run_complete.DryRun = false; want true for a dry run")
	}
	if complete.Scanned != 2 {
		t.Errorf("run_complete.Scanned = %d; want 2 (dry-run processed)", complete.Scanned)
	}
	if complete.TotalApplied != 0 {
		t.Errorf("run_complete.TotalApplied = %d; want 0 (dry run submits nothing)", complete.TotalApplied)
	}
	// The digest must carry the matched jobs (so the email lists them).
	if len(complete.Jobs) != 2 {
		t.Fatalf("run_complete.Jobs = %d entries; want 2 matched jobs", len(complete.Jobs))
	}
	for _, j := range complete.Jobs {
		if j.Status != "found" || j.URL == "" {
			t.Errorf("run_complete.Jobs entry = %+v; want status 'found' with URL", j)
		}
	}
}

// A real auto-apply run (manual "Run" with consent) DOES emit per-job applied
// events that become "✅ Applied: X @ Y" emails.
func TestRunOnce_AutoApplyEmitsJobAppliedEvents(t *testing.T) {
	eng := newTestEngine(t, &config.Config{ApplyConsent: true})
	eng.DryRun = false
	eng.AutoApply = true
	rec := &recordingNotifier{}
	eng.Notifier = notifier.MultiNotifier{rec}
	eng.providers = []provider.Provider{
		&fakeProvider{name: "a", jobs: []provider.Job{jobFor("a")}},
	}

	if err := eng.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if !hasKind(rec.events, notifier.EventJobApplied) {
		t.Errorf("real run must emit job_applied; got %v", kindsOf(rec.events))
	}
	var complete *notifier.Event
	for i := range rec.events {
		if rec.events[i].Kind == notifier.EventRunComplete {
			complete = &rec.events[i]
			break
		}
	}
	if complete == nil {
		t.Fatalf("real run must emit run_complete; got %v", kindsOf(rec.events))
	}
	if complete.TotalApplied != 1 {
		t.Errorf("run_complete.TotalApplied = %d; want 1 real application", complete.TotalApplied)
	}
	if complete.TotalFailed != 0 || complete.TotalSkipped != 0 {
		t.Errorf("run_complete counts = applied:%d failed:%d skipped:%d; want 1/0/0",
			complete.TotalApplied, complete.TotalFailed, complete.TotalSkipped)
	}
	// The digest must carry the applied job (title/company/url for the list).
	if len(complete.Jobs) != 1 || complete.Jobs[0].Status != "applied" || complete.Jobs[0].URL == "" {
		t.Errorf("run_complete.Jobs = %+v; want 1 applied job with URL", complete.Jobs)
	}
}
