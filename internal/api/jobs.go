package api

import (
	"net/http"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// Application mirrors the frontend Application type.
type Application struct {
	ID          int    `json:"id"`
	Provider    string `json:"provider"`
	Company     string `json:"company"`
	Role        string `json:"role"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	AppliedAt   string `json:"appliedAt"`
	Location    string `json:"location"`
	Remote      bool   `json:"remote"`
	PostedAt    string `json:"postedAt"`
	FitScore    int    `json:"fitScore"`
	FitSummary  string `json:"fitSummary"`
	Description string `json:"description,omitempty"`
	Outcome     string `json:"outcome"`
	OutcomeAt   string `json:"outcomeAt"`
}

// handleGetJobs returns all applications, optionally filtered by query.
func (s *Server) handleGetJobs(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusInternalServerError, "store not available")
		return
	}

	apps, err := s.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list applications: "+err.Error())
		return
	}

	query := r.URL.Query().Get("q")
	var results []store.Application
	if query != "" {
		for _, a := range apps {
			if contains(a.Company, query) || contains(a.Role, query) ||
				contains(a.Provider, query) || contains(string(a.Status), query) ||
				contains(a.Location, query) {
				results = append(results, a)
			}
		}
	} else {
		results = apps
	}

	frontend := make([]Application, len(results))
	for i, a := range results {
		frontend[i] = storeAppToFrontend(a)
	}
	writeJSON(w, http.StatusOK, frontend)
}

// handlePatchJobOutcome cycles the outcome of an application.
func (s *Server) handlePatchJobOutcome(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusInternalServerError, "store not available")
		return
	}

	idStr := r.PathValue("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing job id")
		return
	}

	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	var id int64
	for _, c := range idStr {
		if c < '0' || c > '9' {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		id = id*10 + int64(c-'0')
	}

	if err := s.store.SetOutcome(id, store.Outcome(body.Outcome)); err != nil {
		writeError(w, http.StatusInternalServerError, "set outcome: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func storeAppToFrontend(a store.Application) Application {
	return Application{
		ID:         int(a.ID),
		Provider:   a.Provider,
		Company:    a.Company,
		Role:       a.Role,
		URL:        a.URL,
		Status:     string(a.Status),
		Reason:     a.Reason,
		AppliedAt:  a.AppliedAt.Format(time.RFC3339),
		Location:   a.Location,
		Remote:     a.Remote,
		PostedAt:   a.PostedAt.Format(time.RFC3339),
		FitScore:   a.FitScore,
		FitSummary: a.FitSummary,
		Outcome:    string(a.Outcome),
		OutcomeAt:  a.OutcomeAt.Format(time.RFC3339),
	}
}
