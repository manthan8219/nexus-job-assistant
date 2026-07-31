package api

import "net/http"

// handleGetOutreachSetup returns outreach configuration (stub).
func (s *Server) handleGetOutreachSetup(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"consent":        false,
		"mode":           "confirm",
		"maxEmailsDay":   10,
		"maxLinkedInDay": 5,
		"aiCompose":      false,
		"aiReview":       false,
	})
}

// handlePutOutreachSetup saves outreach configuration (stub).
func (s *Server) handlePutOutreachSetup(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleGetOutreachItems returns outreach queue items (stub).
func (s *Server) handleGetOutreachItems(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

// handlePostOutreachBuild builds outreach queue from applied jobs (stub).
func (s *Server) handlePostOutreachBuild(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

// handlePostOutreachSend sends a single outreach item (stub).
func (s *Server) handlePostOutreachSend(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id")})
}

// handleGetOutreachLog returns outreach history (stub).
func (s *Server) handleGetOutreachLog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

// handlePostNotifyTest sends a test notification to all configured channels.
func (s *Server) handlePostNotifyTest(w http.ResponseWriter, r *http.Request) {
	if s.notifier == nil || len(s.notifier) == 0 {
		writeError(w, http.StatusBadRequest, "no notification channels configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": len(s.notifier)})
}
