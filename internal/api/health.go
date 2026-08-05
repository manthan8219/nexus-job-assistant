package api

import (
	"net/http"
	"time"
)

// handleHealth returns a simple health check for AWS load balancers.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"version":   "0.2.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"uptime":    time.Since(startTime).String(),
	})
}

var startTime = time.Now()
