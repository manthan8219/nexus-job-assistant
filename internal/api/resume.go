package api

import (
	"net/http"
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

// handlePostResumeImprove triggers resume improvement (stub).
func (s *Server) handlePostResumeImprove(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"analysis": nil,
		"formats":  []string{},
		"pdfNote":  nil,
	})
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
