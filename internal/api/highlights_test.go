package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/inbox"
)

func TestHandleGetHighlights(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NEXUS_HOME", filepath.Join(home, ".nexus"))

	p, err := inbox.HighlightsPath()
	if err != nil {
		t.Fatalf("HighlightsPath: %v", err)
	}
	h := inbox.Highlight{
		ID: "id1", MessageID: "m1", From: "recruiter@acme.com", Subject: "Interview invitation",
		Date: time.Now(), Signal: inbox.SignalInterview, Confidence: 95, Company: "Acme",
	}
	if err := inbox.Upsert(p, h); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/highlights", nil)
	rr := httptest.NewRecorder()
	srv.handleGetHighlights(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rr.Code)
	}
	var hs []inbox.Highlight
	if err := json.Unmarshal(rr.Body.Bytes(), &hs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(hs) != 1 || hs[0].Signal != inbox.SignalInterview || hs[0].Company != "Acme" {
		t.Errorf("got %+v; want 1 interview highlight for Acme", hs)
	}
}

func TestHandleGetHighlightsEmptyWhenMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NEXUS_HOME", filepath.Join(home, ".nexus"))

	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/highlights", nil)
	rr := httptest.NewRecorder()
	srv.handleGetHighlights(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rr.Code)
	}
	var hs []inbox.Highlight
	if err := json.Unmarshal(rr.Body.Bytes(), &hs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(hs) != 0 {
		t.Errorf("expected empty highlights, got %d", len(hs))
	}
}
