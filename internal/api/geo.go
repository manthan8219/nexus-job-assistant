package api

import (
	"net/http"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/geo"
)

// handleGeoSearch provides city autocomplete for the Target Locations field.
func (s *Server) handleGeoSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	hits := geo.Search(q, 8)
	type result struct {
		Label   string `json:"label"`
		Country string `json:"country"`
		ISO2    string `json:"iso2"`
	}
	out := make([]result, 0, len(hits))
	for _, c := range hits {
		out = append(out, result{
			Label:   c.Display(),
			Country: c.Country,
			ISO2:    c.ISO2,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
