package api

import (
	"context"
	"net/http"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// handlePostJobTitlesSuggest generates AI job title suggestions from a job intent.
func (s *Server) handlePostJobTitlesSuggest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Intent string   `json:"intent"`
		Years  string   `json:"years,omitempty"`
		Hints  []string `json:"hints,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Intent == "" {
		writeError(w, http.StatusBadRequest, "intent is required")
		return
	}

	ai := resume.AIOptionsFromConfig(s.cfg)
	if !ai.Enabled {
		// Offline catalog — suggestions work for ANY profession without AI keys.
		titles := resume.SuggestTitlesOffline(body.Intent, body.Years, body.Hints)
		writeJSON(w, http.StatusOK, map[string]any{
			"titles":     titles,
			"intent":     body.Intent,
			"profession": resume.SuggestProfession(body.Intent),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	titles, err := resume.SuggestJobTitles(ctx, ai, body.Intent, body.Years, body.Hints)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "suggest titles: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"titles": titles,
		"intent": body.Intent,
	})
}
