package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func TestHandlePostJobDismiss(t *testing.T) {
	st, err := store.OpenAt(filepath.Join(t.TempDir(), "apps.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.Insert(store.Application{
		Provider: "manual", Company: "Acme Health", Role: "Cardiologist",
		URL: "https://acme.health/careers/cardio", Status: store.StatusQueued,
		AppliedAt: time.Now().UTC(), Approved: true,
	}); err != nil {
		t.Fatal(err)
	}
	apps, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	id := apps[0].ID

	srv := &Server{store: st}
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/1/dismiss", nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()
	srv.handlePostJobDismiss(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}

	got, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != store.StatusSkipped {
		t.Errorf("status = %q; want skipped", got[0].Status)
	}
	if got[0].Reason != "dismissed by user" {
		t.Errorf("reason = %q; want dismissed by user", got[0].Reason)
	}
	if got[0].Approved {
		t.Error("approved should be cleared on dismiss")
	}
	if got[0].ID != id {
		t.Errorf("dismissed the wrong id %d (want %d)", got[0].ID, id)
	}

	// Bad id -> 400.
	bad := httptest.NewRequest(http.MethodPost, "/api/jobs/abc/dismiss", nil)
	bad.SetPathValue("id", "abc")
	badRR := httptest.NewRecorder()
	srv.handlePostJobDismiss(badRR, bad)
	if badRR.Code != http.StatusBadRequest {
		t.Errorf("bad id status = %d; want 400", badRR.Code)
	}

	// Non-existent id -> 404.
	missing := httptest.NewRequest(http.MethodPost, "/api/jobs/999/dismiss", nil)
	missing.SetPathValue("id", "999")
	missingRR := httptest.NewRecorder()
	srv.handlePostJobDismiss(missingRR, missing)
	if missingRR.Code != http.StatusNotFound {
		t.Errorf("missing id status = %d; want 404", missingRR.Code)
	}
}
