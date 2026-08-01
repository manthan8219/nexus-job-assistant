package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
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
	if len(tmpls) != 12 {
		t.Fatalf("got %d templates; want 12", len(tmpls))
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

func TestImproveResponseIncludesFit(t *testing.T) {
	out := &resume.ImproveOutput{
		PreviewMD:    "# Ada",
		Dir:          "~/.nexus/resumes/improved-x",
		TemplateID:   "compact",
		TemplateName: "Compact",
		Review:       resume.PolishReview{Summary: "ok", ATSScore: 88, QualityScore: 84},
		VersionID:    "20260101-120000",
		Fit: resume.FitPlan{
			TemplateID:     "compact",
			Layout:         resume.LayoutSingle,
			Budget:         resume.SpaceBudget{TargetPages: 1, MaxRoles: 5, CharsPerLine: 100},
			PlannedLines:   42,
			TargetLines:    54,
			EstimatedPages: 1,
			Pages:          1,
			FitScore:       100,
		},
	}
	resp := improveResponse(out)
	if resp["templateId"] != "compact" {
		t.Errorf("templateId = %v; want compact", resp["templateId"])
	}
	if resp["pdfId"] != "20260101-120000" {
		t.Errorf("pdfId = %v; want 20260101-120000", resp["pdfId"])
	}
	fit, ok := resp["fit"].(resume.FitPlan)
	if !ok {
		t.Fatalf("fit = %T; want resume.FitPlan", resp["fit"])
	}
	if fit.TemplateID != "compact" || fit.Pages != 1 || fit.FitScore != 100 {
		t.Errorf("unexpected fit payload: %+v", fit)
	}
	// The fit object must survive JSON serialization (what the web client sees).
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"fit"`)) {
		t.Error("fit key missing from improve JSON")
	}
	if !bytes.Contains(b, []byte(`"fitScore":100`)) {
		t.Errorf("fitScore missing from improve JSON: %s", b)
	}
}
