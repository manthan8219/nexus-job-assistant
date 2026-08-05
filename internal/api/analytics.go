package api

import "net/http"

// handleGetAnalytics returns the analytics aggregation snapshot for the
// analytics dashboard (funnel, per-provider yield, applied-over-time).
// GET /api/analytics
func (s *Server) handleGetAnalytics(w http.ResponseWriter, r *http.Request) {
	st := s.storeFor(r)
	if st == nil {
		writeError(w, http.StatusInternalServerError, "store not available")
		return
	}
	snap, err := st.AnalyticsSnapshot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "analytics: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}
