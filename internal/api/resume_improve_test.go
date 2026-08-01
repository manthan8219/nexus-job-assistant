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

func TestResumeImproveRejectsUnknownTemplate(t *testing.T) {
	cfg := &config.Config{AIAssist: true, ResumePath: "/nonexistent/resume.pdf"}
	srv := &Server{cfg: cfg}

	body, _ := json.Marshal(map[string]any{
		"targetRole": "Cardiologist",
		"formats":    []string{"markdown"},
		"templateId": "nope",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/resume/improve", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handlePostResumeImprove(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for an unknown template; body %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("template")) {
		t.Errorf("expected a template error message, got %s", rr.Body.String())
	}
}

func TestGetResumeTemplates(t *testing.T) {
	srv := &Server{cfg: &config.Config{}}

	req := httptest.NewRequest(http.MethodGet, "/api/resume/templates", nil)
	rr := httptest.NewRecorder()
	srv.handleGetResumeTemplates(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body %s", rr.Code, rr.Body.String())
	}

	var tmpls []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &tmpls); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(tmpls) != 4 {
		t.Fatalf("got %d templates; want 4", len(tmpls))
	}
	found := false
	for _, m := range tmpls {
		if m["id"] == "classic" {
			found = true
			if m["name"] == nil || m["layout"] == nil || m["sections"] == nil {
				t.Errorf("classic template missing manifest fields: %v", m)
			}
		}
	}
	if !found {
		t.Error("template registry should include classic")
	}
}

