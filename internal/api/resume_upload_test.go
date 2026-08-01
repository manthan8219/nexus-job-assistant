package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveResumeUpload(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		wantBase string // expected sanitized base of the saved name
	}{
		{name: "plain pdf", filename: "resume.pdf", content: "%PDF-1.4", wantBase: "resume"},
		{name: "spaces and unicode", filename: "my r\u00e9sum\u00e9 v2.pdf", content: "%PDF", wantBase: "my-r-sum--v2"},
		{name: "path traversal is neutralized", filename: "../../etc/evil.pdf", content: "x", wantBase: "evil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path, name, err := saveResumeUpload(strings.NewReader(tt.content), tt.filename, dir)
			if err != nil {
				t.Fatalf("saveResumeUpload() error = %v", err)
			}
			if !strings.HasPrefix(name, tt.wantBase+"-") || !strings.HasSuffix(name, ".pdf") {
				t.Errorf("name = %q; want prefix %q and .pdf suffix", name, tt.wantBase+"-")
			}
			if filepath.Dir(path) != dir {
				t.Errorf("path = %q; want inside dir %q", path, dir)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read saved file: %v", err)
			}
			if string(data) != tt.content {
				t.Errorf("content = %q; want %q", data, tt.content)
			}
		})
	}
}

func TestHandlePostResumeUploadRejectsInvalid(t *testing.T) {
	srv := &Server{}

	t.Run("missing file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/resume/upload", bytes.NewBufferString(""))
		req.Header.Set("Content-Type", "application/octet-stream")
		rr := httptest.NewRecorder()
		srv.handlePostResumeUpload(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want %d (body %s)", rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	})

	t.Run("non-pdf extension", func(t *testing.T) {
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		fw, err := w.CreateFormFile("file", "resume.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write([]byte("not a pdf"))
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/resume/upload", &b)
		req.Header.Set("Content-Type", w.FormDataContentType())
		rr := httptest.NewRecorder()
		srv.handlePostResumeUpload(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d; want %d (body %s)", rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	})
}
