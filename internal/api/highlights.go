package api

import (
	"net/http"

	"github.com/manthan8219/nexus-job-assistant/internal/inbox"
)

// handleGetHighlights returns the hiring-email signals discovered by the
// inbox scan (internal/inbox), newest first.
func (s *Server) handleGetHighlights(w http.ResponseWriter, r *http.Request) {
	p, err := inbox.HighlightsPath()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "highlights path: "+err.Error())
		return
	}
	hs, err := inbox.LoadAll(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load highlights: "+err.Error())
		return
	}
	out := make([]inbox.Highlight, 0, len(hs))
	out = append(out, hs...)
	writeJSON(w, http.StatusOK, out)
}
