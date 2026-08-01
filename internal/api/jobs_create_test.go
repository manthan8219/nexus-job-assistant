package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func TestHandlePostJobs(t *testing.T) {
	st, err := store.OpenAt(filepath.Join(t.TempDir(), "apps.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := &Server{store: st}

	body, _ := json.Marshal(map[string]any{
		"role":     "Cardiologist",
		"company":  "Acme Health",
		"url":      "https://acme.health/careers/cardio",
		"location": "Remote",
		"remote":   true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handlePostJobs(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}

	var created Application
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Role != "Cardiologist" || created.Company != "Acme Health" {
		t.Errorf("unexpected created app: %+v", created)
	}
	if created.Status != "queued" {
		t.Errorf("status = %q; want queued", created.Status)
	}
	if created.Provider != "manual" {
		t.Errorf("provider = %q; want manual", created.Provider)
	}
	if created.ID == 0 {
		t.Error("expected a generated id")
	}

	// The job lands in the review queue.
	apps, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].URL != "https://acme.health/careers/cardio" {
		t.Errorf("store should contain the manually-added job, got %+v", apps)
	}

	// Missing required fields → 400.
	bad := httptest.NewRequest(http.MethodPost, "/api/jobs",
		bytes.NewReader([]byte(`{"role":"","company":"","url":""}`)))
	badRR := httptest.NewRecorder()
	srv.handlePostJobs(badRR, bad)
	if badRR.Code != http.StatusBadRequest {
		t.Errorf("bad input status = %d; want 400", badRR.Code)
	}

	// Invalid JSON → 400.
	notJSON := httptest.NewRequest(http.MethodPost, "/api/jobs",
		bytes.NewReader([]byte(`{`)))
	notJSONRR := httptest.NewRecorder()
	srv.handlePostJobs(notJSONRR, notJSON)
	if notJSONRR.Code != http.StatusBadRequest {
		t.Errorf("invalid json status = %d; want 400", notJSONRR.Code)
	}
}
