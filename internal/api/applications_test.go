package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func TestHandlePostApplicationApproved(t *testing.T) {
	st, err := store.OpenAt(filepath.Join(t.TempDir(), "apps.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.Insert(store.Application{
		Provider: "greenhouse", Company: "Acme", Role: "Engineer",
		URL: "https://example.com/1", Status: store.StatusQueued,
		AppliedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	apps, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	id := apps[0].ID

	srv := &Server{store: st}

	body, _ := json.Marshal(map[string]bool{"approved": true})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/applications/%d/approved", id), bytes.NewReader(body))
	req.SetPathValue("id", fmt.Sprintf("%d", id))
	rr := httptest.NewRecorder()
	srv.handlePostApplicationApproved(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}

	got, err := st.GetByIDs([]int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].Approved {
		t.Error("approved should be true after POST")
	}

	// Bad id → 400.
	bad := httptest.NewRequest(http.MethodPost, "/api/applications/abc/approved", bytes.NewReader(body))
	bad.SetPathValue("id", "abc")
	badRR := httptest.NewRecorder()
	srv.handlePostApplicationApproved(badRR, bad)
	if badRR.Code != http.StatusBadRequest {
		t.Errorf("bad id status = %d; want 400", badRR.Code)
	}
}
