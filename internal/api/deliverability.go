package api

import (
	"net/http"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/deliverability"
)

// handleGetDeliverabilityAudit audits a domain's SPF/DMARC/DKIM posture and
// returns a report with actionable guidance.
// GET /api/deliverability/audit?domain=example.com
func (s *Server) handleGetDeliverabilityAudit(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if domain == "" {
		writeError(w, http.StatusBadRequest, "missing domain query parameter")
		return
	}
	report, err := deliverability.Audit(r.Context(), domain, s.txtResolver)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
