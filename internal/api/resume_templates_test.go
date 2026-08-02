package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

func TestHandleGetResumeTemplatesIncludesLaTeX(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/resume/templates", nil)
	rr := httptest.NewRecorder()
	srv.handleGetResumeTemplates(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body %s", rr.Code, rr.Body.String())
	}
	var list []resume.Template
	if err := json.Unmarshal([]byte(rr.Body.String()), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("no templates in response")
	}
	for _, tpl := range list {
		if tpl.ID == "" || tpl.Name == "" {
			t.Errorf("template with empty id/name: %+v", tpl)
		}
		// Every template must ship the real LaTeX source (sample persona) so
		// the web UI can render a faithful live preview.
		if !strings.Contains(tpl.LaTeX, `\documentclass`) {
			t.Errorf("template %s latex missing documentclass", tpl.ID)
		}
		if !strings.Contains(tpl.LaTeX, "Maya Okonkwo") {
			t.Errorf("template %s latex missing sample persona", tpl.ID)
		}
	}
}
