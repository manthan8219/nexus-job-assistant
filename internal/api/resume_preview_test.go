package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
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

func TestPostResumeTemplatePreview(t *testing.T) {
	srv := &Server{cfg: &config.Config{}}
	doc := map[string]any{
		"fullName": "Ada Lovelace",
		"headline": "Senior Engineer",
		"summary":  "Backend engineer shipping distributed systems at scale.",
		"skills":   []string{"Go", "PostgreSQL"},
		"experience": []map[string]any{
			{"title": "Senior Engineer", "org": "Acme", "period": "2021—present",
				"bullets": []string{"Cut p99 checkout latency by 40%."}},
		},
		"education": []string{"B.Tech, University, 2015"},
	}
	body, _ := json.Marshal(doc)
	req := httptest.NewRequest(http.MethodPost, "/api/resume/templates/classic/preview", bytes.NewReader(body))
	req.SetPathValue("id", "classic")
	rr := httptest.NewRecorder()
	srv.handlePostResumeTemplatePreview(rr, req)

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

func TestPostResumeTemplatePreviewRejectsUnknownAndEmpty(t *testing.T) {
	srv := &Server{cfg: &config.Config{}}

	// Unknown template → 400.
	body, _ := json.Marshal(map[string]any{"fullName": "Ada"})
	req := httptest.NewRequest(http.MethodPost, "/api/resume/templates/nope/preview", bytes.NewReader(body))
	req.SetPathValue("id", "nope")
	rr := httptest.NewRecorder()
	srv.handlePostResumeTemplatePreview(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for an unknown template; body %s", rr.Code, rr.Body.String())
	}

	// Empty document → 400.
	body, _ = json.Marshal(map[string]any{})
	req = httptest.NewRequest(http.MethodPost, "/api/resume/templates/classic/preview", bytes.NewReader(body))
	req.SetPathValue("id", "classic")
	rr = httptest.NewRecorder()
	srv.handlePostResumeTemplatePreview(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400 for an empty doc; body %s", rr.Code, rr.Body.String())
	}
}

func TestGetResumeLibraryPDF(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NEXUS_HOME", "")

	dir := filepath.Join(home, ".nexus", "resumes")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Register a REAL rendered PDF so the stream is a valid document.
	tpl, err := resume.GetTemplate(resume.TemplateClassic)
	if err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(dir, "improved-20260101-120000.pdf")
	if err := resume.RenderNativePDFFor(resume.SampleResume(), tpl, pdfPath); err != nil {
		t.Fatal(err)
	}
	if err := resume.RegisterVersion(resume.Version{
		ID:        "20260101-120000",
		CreatedAt: time.Now(),
		Label:     "test",
		Template:  "classic",
		PDFPath:   pdfPath,
	}); err != nil {
		t.Fatal(err)
	}

	srv := &Server{cfg: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/api/resume/library/20260101-120000/pdf", nil)
	req.SetPathValue("id", "20260101-120000")
	rr := httptest.NewRecorder()
	srv.handleGetResumeLibraryPDF(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("content-type = %q; want application/pdf", ct)
	}
	if !bytes.HasPrefix(rr.Body.Bytes(), []byte("%PDF")) {
		t.Errorf("body does not start with %%PDF")
	}

	// Unknown version → 404.
	req = httptest.NewRequest(http.MethodGet, "/api/resume/library/missing/pdf", nil)
	req.SetPathValue("id", "missing")
	rr = httptest.NewRecorder()
	srv.handleGetResumeLibraryPDF(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404 for a missing version", rr.Code)
	}
}
