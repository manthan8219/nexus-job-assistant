package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestResumeImproveRequiresAIAssist(t *testing.T) {
	cfg := &config.Config{} // AI Assist off by default
	srv := &Server{cfg: cfg}

	body, _ := json.Marshal(map[string]any{
		"targetRole": "Cardiologist",
		"formats":    []string{"markdown"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/resume/improve", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handlePostResumeImprove(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 with AI off; body %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("AI Assist")) {
		t.Errorf("expected an honest AI-Assist message, got %s", rr.Body.String())
	}
}

func TestResumeImproveRequiresResumePath(t *testing.T) {
	cfg := &config.Config{AIAssist: true} // AI on but no resume path
	srv := &Server{cfg: cfg}

	body, _ := json.Marshal(map[string]any{
		"targetRole": "Cardiologist",
		"formats":    []string{"markdown"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/resume/improve", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handlePostResumeImprove(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 without a resume; body %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("resume path")) {
		t.Errorf("expected an honest resume-path message, got %s", rr.Body.String())
	}
}
