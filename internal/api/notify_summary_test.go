package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
)

type fakeNotifier struct {
	name string
	err  error
	got  *notifier.Event
}

func (f *fakeNotifier) Name() string { return f.name }

func (f *fakeNotifier) Send(_ context.Context, ev notifier.Event) error {
	f.got = &ev
	return f.err
}

func newTestServer(notifiers notifier.MultiNotifier) *Server {
	return &Server{notifier: notifiers, mu: sync.RWMutex{}}
}

func TestNotifySummarySendsToChannels(t *testing.T) {
	fake := &fakeNotifier{name: "test"}
	srv := newTestServer(notifier.MultiNotifier{fake})
	req := httptest.NewRequest(http.MethodPost, "/api/notify/summary", nil)
	rr := httptest.NewRecorder()
	srv.handlePostNotifySummary(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}
	if fake.got == nil {
		t.Fatal("expected the summary event to be dispatched")
	}
	if fake.got.Kind != notifier.EventDailySummary {
		t.Errorf("kind = %q; want daily_summary", fake.got.Kind)
	}
}

func TestNotifySummaryRequiresChannels(t *testing.T) {
	srv := newTestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/api/notify/summary", nil)
	rr := httptest.NewRecorder()
	srv.handlePostNotifySummary(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 without channels", rr.Code)
	}
}

func TestNotifySummaryReportsSendErrors(t *testing.T) {
	fake := &fakeNotifier{name: "broken", err: context.DeadlineExceeded}
	srv := newTestServer(notifier.MultiNotifier{fake})
	req := httptest.NewRequest(http.MethodPost, "/api/notify/summary", nil)
	rr := httptest.NewRecorder()
	srv.handlePostNotifySummary(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500 when every channel fails", rr.Code)
	}
}
