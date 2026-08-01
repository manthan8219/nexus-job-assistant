package api

import (
	"net/http"
	"strings"
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
	Approved    bool   `json:"approved"`
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

// handlePostJobs records a manually-added job into the review queue
// (the frontend "Add a job" flow).
func (s *Server) handlePostJobs(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusInternalServerError, "store not available")
		return
	}

	var input struct {
		Role     string `json:"role"`
		Company  string `json:"company"`
		URL      string `json:"url"`
		Location string `json:"location"`
		Remote   bool   `json:"remote"`
		Provider string `json:"provider"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	input.Role = strings.TrimSpace(input.Role)
	input.Company = strings.TrimSpace(input.Company)
	input.URL = strings.TrimSpace(input.URL)
	if input.Role == "" || input.Company == "" || input.URL == "" {
		writeError(w, http.StatusBadRequest, "role, company, and url are required")
		return
	}

	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = "manual"
	}

	now := time.Now().UTC()
	if err := s.store.Insert(store.Application{
		Provider:  provider,
		Company:   input.Company,
		Role:      input.Role,
		URL:       input.URL,
		Status:    store.StatusQueued,
		Reason:    "added manually — awaiting your approval",
		AppliedAt: now,
		Location:  strings.TrimSpace(input.Location),
		Remote:    input.Remote,
		PostedAt:  now,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "save application: "+err.Error())
		return
	}

	// Reload to pick up the generated id, then return the frontend shape.
	apps, err := s.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list applications: "+err.Error())
		return
	}
	for _, a := range apps {
		if a.URL == input.URL {
			writeJSON(w, http.StatusCreated, storeAppToFrontend(a))
			return
		}
	}
	writeError(w, http.StatusInternalServerError, "saved application not found after insert")
}

// parsePathID parses a numeric path value ("{id}") into an int64.
func parsePathID(r *http.Request) (int64, bool) {
	idStr := r.PathValue("id")
	if idStr == "" {
		return 0, false
	}
	var id int64
	for _, c := range idStr {
		if c < '0' || c > '9' {
			return 0, false
		}
		id = id*10 + int64(c-'0')
	}
	return id, true
}

// handlePatchJobOutcome cycles the outcome of an application.
func (s *Server) handlePatchJobOutcome(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusInternalServerError, "store not available")
		return
	}

	id, ok := parsePathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := s.store.SetOutcome(id, store.Outcome(body.Outcome)); err != nil {
		writeError(w, http.StatusInternalServerError, "set outcome: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handlePostApplicationApproved marks/unmarks an application for a real apply
// (the review-queue approve → apply flow).
func (s *Server) handlePostApplicationApproved(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusInternalServerError, "store not available")
		return
	}

	id, ok := parsePathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	var body struct {
		Approved bool `json:"approved"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := s.store.SetApproved(id, body.Approved); err != nil {
		writeError(w, http.StatusInternalServerError, "set approved: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handlePostJobDismiss marks a queued job as dismissed (skipped) so it leaves
// the review queue (the frontend "Dismiss" action).
func (s *Server) handlePostJobDismiss(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusInternalServerError, "store not available")
		return
	}

	id, ok := parsePathID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	if err := s.store.SetStatus(id, store.StatusSkipped, "dismissed by user"); err != nil {
		if strings.Contains(err.Error(), "no application") {
			writeError(w, http.StatusNotFound, "application not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "dismiss job: "+err.Error())
		return
	}
	// Clear any pending approval so the job can never be applied later.
	_ = s.store.SetApproved(id, false)
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
		Approved:   a.Approved,
	}
}
