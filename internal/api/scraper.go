package api

import (
	"net/http"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

// ScraperStatus contains info about the career scraper service.
type ScraperStatus struct {
	Installed bool     `json:"installed"`
	Running   bool     `json:"running"`
	Backends  []string `json:"backends,omitempty"`
}

// handleScraperStatus returns the status of the career scraper service.
func (s *Server) handleScraperStatus(w http.ResponseWriter, r *http.Request) {
	status := ScraperStatus{
		Installed: scraper.Installed(),
		Running:   scraper.Running(),
	}
	if status.Installed {
		status.Backends = scraper.InstalledBackends()
	}
	writeJSON(w, http.StatusOK, status)
}

// handleScraperInstall installs the scraper Python venv.
func (s *Server) handleScraperInstall(w http.ResponseWriter, r *http.Request) {
	if err := scraper.Install(r.Context(), nil); err != nil {
		writeError(w, http.StatusInternalServerError, "install failed: "+err.Error())
		return
	}
	_ = scraper.Start("", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleScraperStart starts the scraper service.
func (s *Server) handleScraperStart(w http.ResponseWriter, r *http.Request) {
	_ = scraper.Start("", "")
	_ = scraper.WaitReady(15 * time.Second)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"running": scraper.Running(),
	})
}
