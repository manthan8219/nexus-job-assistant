// Package tests holds black-box integration tests that exercise multiple
// packages together through their public APIs only (httptest fakes, temp-dir
// stores). See AGENTS.md section 13 / 14 for the invariants covered here.
package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
)

// Graceful degradation (AGENTS.md section 6-L / 10): one notification channel
// failing must never block or abort delivery to the others. Built entirely from
// the public notifier API against httptest fakes — no real network, no secrets.
func TestNotifierFanOut_OneChannelFailsOthersStillDeliver(t *testing.T) {
	var received strings.Builder
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		received.Write(buf)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ok.Close()

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fail.Close()

	mn := notifier.MultiNotifier{
		notifier.NewDiscordNotifier(ok.URL),
		notifier.NewDiscordNotifier(fail.URL),
	}
	errs := mn.Send(context.Background(), notifier.Event{
		Kind:     notifier.EventJobApplied,
		JobTitle: "Backend Engineer",
		Company:  "Acme Corp",
	})

	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error (the failing channel); got %d: %v", len(errs), errs)
	}
	body := received.String()
	if !strings.Contains(body, "Acme Corp") {
		t.Errorf("healthy channel did not receive the event payload; got: %s", body)
	}
}

// Empty MultiNotifier is a safe no-op (a run with no configured channels must
// not panic and must report no errors).
func TestNotifierFanOut_EmptyIsNoop(t *testing.T) {
	mn := notifier.MultiNotifier{}
	if errs := mn.Send(context.Background(), notifier.Event{Kind: notifier.EventCustom}); len(errs) != 0 {
		t.Fatalf("empty MultiNotifier must be a no-op; got errors: %v", errs)
	}
}
