package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/workcontext"
)

// handleGetResumeAnalyze returns current resume analysis.
// It checks the on-disk cache first, then falls back to a descriptive stub.
func (s *Server) handleGetResumeAnalyze(w http.ResponseWriter, r *http.Request) {
	path := ""
	aiEnabled := false
	if s.cfg != nil {
		path = s.cfg.ResumePath
		aiEnabled = s.cfg.AIAssist
	}
	if path != "" {
		if cached, ok := resume.LoadFreshCache(path, aiEnabled); ok {
			writeJSON(w, http.StatusOK, map[string]any{
				"valid":      cached.Result.Valid,
				"fileType":   cached.Result.FileType,
				"message":    cached.Result.Message,
				"err":        cached.Result.Err,
				"profile":    cached.Result.Profile,
				"contact":    cached.Result.Contact,
				"analyzedAt": cached.AnalyzedAt.Format(time.RFC3339),
			})
			return
		}
		// Resume exists but no cache — tell the user.
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":      false,
			"fileType":   "",
			"message":    "Resume file found — hit Re-analyze to generate an AI profile.",
			"err":        "",
			"profile":    nil,
			"analyzedAt": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":      false,
		"fileType":   "",
		"message":    "Configure a resume path in Settings first.",
		"err":        "",
		"profile":    nil,
		"analyzedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

// handlePostResumeAnalyze triggers a new resume analysis and caches the result.
func (s *Server) handlePostResumeAnalyze(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &body); err != nil {
		body.Path = ""
	}

	path := body.Path
	if path == "" && s.cfg != nil {
		path = s.cfg.ResumePath
	}
	if path == "" {
		writeError(w, http.StatusBadRequest, "no resume path configured")
		return
	}

	ai := resume.AIOptionsFromConfig(s.cfg)
	result := resume.AnalyzeFull(path, ai)

	// Cache the analysis result so subsequent GETs return it.
	if result.Valid || result.Profile != nil {
		_ = resume.SaveCache(path, ai.Enabled, result)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":      result.Valid,
		"fileType":   result.FileType,
		"message":    result.Message,
		"err":        result.Err,
		"profile":    result.Profile,
		"contact":    result.Contact,
		"analyzedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

// handleGetResumeTemplates returns the curated resume template registry. Each
// manifest declares the sections/layout/constraints the backend understands,
// which is what lets the AI fit generated content into the chosen design.
func (s *Server) handleGetResumeTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, resume.Templates())
}

// handleGetResumeTemplatePreviewPDF renders the sample persona into the named
// template with the real PDF renderer, so the gallery's "view the actual
// document" action shows exactly what that template produces — same engine,
// same fonts, same layout that real resumes are written with.
func (s *Server) handleGetResumeTemplatePreviewPDF(w http.ResponseWriter, r *http.Request) {
	templateID := r.PathValue("id")
	if templateID == "" {
		writeError(w, http.StatusBadRequest, "missing template id")
		return
	}
	pdfBytes, err := resume.RenderTemplatePreviewPDF(templateID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\""+templateID+"-preview.pdf\"")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

// handlePostResumeTemplatePreview renders a user-supplied resume document into
// the named template with the real PDF engine — no AI, fully deterministic.
// This is the "preview with my data" action: pick a template and see YOUR
// resume in it before spending AI credits on generation.
func (s *Server) handlePostResumeTemplatePreview(w http.ResponseWriter, r *http.Request) {
	templateID := r.PathValue("id")
	if templateID == "" {
		writeError(w, http.StatusBadRequest, "missing template id")
		return
	}
	// The web client sends camelCase keys (mirrors the frontend PreviewResumeDoc);
	// map them onto the resume model explicitly since ImprovedDoc uses snake_case.
	var body struct {
		FullName   string   `json:"fullName"`
		Headline   string   `json:"headline"`
		Summary    string   `json:"summary"`
		Skills     []string `json:"skills"`
		Experience []struct {
			Title   string   `json:"title"`
			Org     string   `json:"org"`
			Period  string   `json:"period"`
			Bullets []string `json:"bullets"`
		} `json:"experience"`
		Education []string `json:"education"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid resume document: "+err.Error())
		return
	}
	if strings.TrimSpace(body.FullName) == "" && strings.TrimSpace(body.Summary) == "" && len(body.Experience) == 0 {
		writeError(w, http.StatusBadRequest, "send at least a name, summary, or experience entry")
		return
	}
	tpl, err := resume.GetTemplate(templateID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	doc := resume.ImprovedDoc{
		FullName:  body.FullName,
		Headline:  body.Headline,
		Summary:   body.Summary,
		Skills:    body.Skills,
		Education: body.Education,
	}
	for _, e := range body.Experience {
		doc.Experience = append(doc.Experience, resume.ImprovedRole{
			Title:   e.Title,
			Org:     e.Org,
			Period:  e.Period,
			Bullets: e.Bullets,
		})
	}
	pdfBytes, err := resume.RenderTemplatePreviewPDFFor(doc, tpl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\""+templateID+"-with-my-data.pdf\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

// handleGetResumeLibraryPDF streams the PDF of one generated resume version so
// the web UI can render it inline (the ResultPanel's PDF pane).
func (s *Server) handleGetResumeLibraryPDF(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing version id")
		return
	}
	v, err := resume.GetVersion(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	data, err := os.ReadFile(v.PDFPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "pdf missing for version "+id)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\""+id+".pdf\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleGetResumeProjects returns work history projects from the workcontext store.
func (s *Server) handleGetResumeProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := workcontext.Load()
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

// handlePutResumeProjects saves a work project to the workcontext store.
func (s *Server) handlePutResumeProjects(w http.ResponseWriter, r *http.Request) {
	var proj workcontext.Project
	if err := readJSON(r, &proj); err != nil {
		writeError(w, http.StatusBadRequest, "invalid project payload")
		return
	}
	if err := workcontext.Upsert(proj); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, proj)
}

// handleDeleteResumeProjects deletes a work project from the workcontext store.
func (s *Server) handleDeleteResumeProjects(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing project id")
		return
	}
	if err := workcontext.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleGetResumeSkills returns the skills list from the config or cached profile.
func (s *Server) handleGetResumeSkills(w http.ResponseWriter, r *http.Request) {
	// First try config's explicit skills list.
	if s.cfg != nil && len(s.cfg.Skills) > 0 {
		writeJSON(w, http.StatusOK, s.cfg.Skills)
		return
	}
	// Fall back to skills from the cached analysis profile.
	if s.cfg != nil && s.cfg.ResumePath != "" {
		if cached, ok := resume.LoadFreshCache(s.cfg.ResumePath, s.cfg.AIAssist); ok {
			if cached.Result.Profile != nil && len(cached.Result.Profile.Skills) > 0 {
				writeJSON(w, http.StatusOK, cached.Result.Profile.Skills)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, []string{})
}

// handlePutResumeSkills saves the skills list to the config.
func (s *Server) handlePutResumeSkills(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Skills []string `json:"skills"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid skills payload")
		return
	}
	if s.cfg != nil {
		cfg := s.cfg
		cfg.Skills = body.Skills
		// Persist the config change so it survives restarts.
		_ = config.Save(cfg)
	}
	writeJSON(w, http.StatusOK, body.Skills)
}

// handlePostResumeImprove generates a stronger resume from analysis + work
// context and exports the requested formats. It fails honestly (400) when AI
// Assist is off, no resume path is configured, or the template id is unknown.
func (s *Server) handlePostResumeImprove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetRole string   `json:"targetRole"`
		Formats    []string `json:"formats"`
		TemplateID string   `json:"templateId"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	ai := resume.AIOptionsFromConfig(s.cfg)
	if !ai.Enabled {
		writeError(w, http.StatusBadRequest, "turn on AI Assist in Config first")
		return
	}

	resumePath := strings.TrimSpace(s.cfg.ResumePath)
	if resumePath == "" {
		writeError(w, http.StatusBadRequest, "set a resume path in Config first")
		return
	}

	tpl, err := resume.GetTemplate(body.TemplateID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	resumeText, err := resume.ExtractText(resumePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read resume: "+err.Error())
		return
	}

	var profile *resume.Profile
	if res := resume.AnalyzeFull(resumePath, ai); res.Valid && res.Profile != nil {
		profile = res.Profile
	}

	projects, _ := workcontext.Load()

	formats := make([]resume.Format, 0, len(body.Formats))
	for _, f := range body.Formats {
		switch resume.Format(f) {
		case resume.FormatMarkdown, resume.FormatLaTeX, resume.FormatPDF:
			formats = append(formats, resume.Format(f))
		}
	}
	if len(formats) == 0 {
		formats = []resume.Format{resume.FormatMarkdown}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
	defer cancel()

	out, err := resume.GenerateImproved(ctx, ai, resume.ImproveInput{
		ResumePath: resumePath,
		ResumeText: resumeText,
		Profile:    profile,
		Projects:   projects,
		Skills:     s.cfg.Skills,
		TargetRole: body.TargetRole,
		Formats:    formats,
		TemplateID: tpl.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "generate improved resume: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, improveResponse(out))
}

// improveResponse builds the JSON payload for a successful /resume/improve.
// Extracted so tests can assert the shape without running the AI pipeline.
func improveResponse(out *resume.ImproveOutput) map[string]any {
	return map[string]any{
		"previewMD":    out.PreviewMD,
		"dir":          out.Dir,
		"templateId":   out.TemplateID,
		"templateName": out.TemplateName,
		"review": map[string]any{
			"summary":      out.Review.Summary,
			"atsScore":     out.Review.ATSScore,
			"qualityScore": out.Review.QualityScore,
		},
		"pdfNote": out.PDFNote,
		"fit":     out.Fit,
		"pdfId":   out.VersionID,
	}
}

// handleGetResumeLibrary returns the list of generated resumes.
func (s *Server) handleGetResumeLibrary(w http.ResponseWriter, r *http.Request) {
	versions, err := resume.ListVersions()
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	if versions == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, versions)
}
