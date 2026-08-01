package api

import (
	"net/http"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// handleGetConfig returns the current Nexus config.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, configToNexusConfig(cfg))
}

// handlePutConfig replaces the entire config and persists it to disk.
func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var incoming NexusConfig
	if err := readJSON(r, &incoming); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.mu.Lock()
	applyNexusConfig(s.cfg, &incoming)
	if err := config.Save(s.cfg); err != nil {
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}
	s.mu.Unlock()
	s.changed()

	writeJSON(w, http.StatusOK, incoming)
}

// handlePatchConfig merges partial fields into the existing config.
func (s *Server) handlePatchConfig(w http.ResponseWriter, r *http.Request) {
	var patch patchRequest
	if err := readJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.mu.Lock()
	if patch.DryRun != nil {
		s.dryRun = *patch.DryRun
	}
	if patch.AutoApply != nil {
		s.autoApply = *patch.AutoApply
	}
	if err := config.Save(s.cfg); err != nil {
		s.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}
	s.mu.Unlock()
	s.changed()

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleGetConfigComplete returns profile completion status.
func (s *Server) handleGetConfigComplete(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	var missing []string
	if cfg.FirstName == "" {
		missing = append(missing, "First Name")
	}
	if cfg.LastName == "" {
		missing = append(missing, "Last Name")
	}
	if cfg.Email == "" {
		missing = append(missing, "Email")
	}
	if cfg.Phone == "" {
		missing = append(missing, "Phone")
	}
	if cfg.ResumePath == "" {
		missing = append(missing, "Resume Path")
	}
	if cfg.TargetJobTitles == "" {
		missing = append(missing, "Target Job Titles")
	}
	if cfg.TargetLocations == "" {
		missing = append(missing, "Target Locations")
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"complete": len(missing) == 0,
		"missing":  missing,
	})
}

type patchRequest struct {
	DryRun    *bool `json:"dry_run,omitempty"`
	AutoApply *bool `json:"auto_apply,omitempty"`
}
