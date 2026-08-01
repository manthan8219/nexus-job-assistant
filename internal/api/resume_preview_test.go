package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestGetResumeTemplatePreviewPDF(t *testing.T) {
	srv := &Server{cfg: &config.Config{}}

	req := httptest.NewRequest(http.MethodGet, "/api/resume/templates/classic/preview.pdf", nil)
	req.SetPathValue("id", "classic")
	rr := httptest.NewRecorder()
	srv.handleGetResumeTemplatePreviewPDF(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("content-type = %q; want application/pdf", ct)
	}
	if !bytes.HasPrefix(rr.Body.Bytes(), []byte("%PDF")) {
		t.Errorf("body does not start with %%PDF (%d bytes)", rr.Body.Len())
	}
}

func TestGetResumeTemplatePreviewPDFRejectsUnknown(t *testing.T) {
	srv := &Server{cfg: &config.Config{}}

	req := httptest.NewRequest(http.MethodGet, "/api/resume/templates/nope/preview.pdf", nil)
	req.SetPathValue("id", "nope")
	rr := httptest.NewRecorder()
	srv.handleGetResumeTemplatePreviewPDF(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400; body %s", rr.Code, rr.Body.String())
	}
}
