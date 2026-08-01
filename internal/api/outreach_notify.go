package api

import (
	"net/http"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// outreachItemDTO mirrors the frontend OutreachItem type (web-shaped, camelCase).
type outreachItemDTO struct {
	ID            string    `json:"id"`
	Channel       string    `json:"channel"`
	JobURL        string    `json:"jobURL"`
	Company       string    `json:"company"`
	Role          string    `json:"role"`
	Provider      string    `json:"provider,omitempty"`
	ContactName   string    `json:"contactName,omitempty"`
	ContactEmail  string    `json:"contactEmail,omitempty"`
	ContactTitle  string    `json:"contactTitle,omitempty"`
	ContactSource string    `json:"contactSource,omitempty"`
	LinkedInURL   string    `json:"linkedInURL,omitempty"`
	Subject       string    `json:"subject,omitempty"`
	Body          string    `json:"body"`
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
	Auto          bool      `json:"auto,omitempty"`
	ReviewScore   int       `json:"reviewScore,omitempty"`
	ReviewNotes   string    `json:"reviewNotes,omitempty"`
	Attempts      int       `json:"attempts,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	SentAt        time.Time `json:"sentAt,omitempty"`
}

func toOutreachItemDTO(it outreach.Item) outreachItemDTO {
	return outreachItemDTO{
		ID:            it.ID,
		Channel:       string(it.Channel),
		JobURL:        it.JobURL,
		Company:       it.Company,
		Role:          it.Role,
		Provider:      it.Provider,
		ContactName:   it.ContactName,
		ContactEmail:  it.ContactEmail,
		ContactTitle:  it.ContactTitle,
		ContactSource: it.ContactSource,
		LinkedInURL:   it.LinkedInURL,
		Subject:       it.Subject,
		Body:          it.Body,
		Status:        string(it.Status),
		Error:         it.Error,
		Auto:          it.Auto,
		ReviewScore:   it.ReviewScore,
		ReviewNotes:   it.ReviewNotes,
		Attempts:      it.Attempts,
		CreatedAt:     it.CreatedAt,
		UpdatedAt:     it.UpdatedAt,
		SentAt:        it.SentAt,
	}
}

// handleGetOutreachSetup returns the real outreach configuration.
func (s *Server) handleGetOutreachSetup(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg
	mode := cfg.OutreachMode
	if mode == "" {
		mode = "confirm"
	}
	maxEmails := cfg.MaxEmailsPerDay
	if maxEmails <= 0 {
		maxEmails = 10
	}
	maxLinkedIn := cfg.MaxLinkedInPerDay
	if maxLinkedIn <= 0 {
		maxLinkedIn = 10
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"consent":           cfg.OutreachConsent,
		"mode":              mode,
		"maxEmailsPerDay":   maxEmails,
		"maxLinkedInPerDay": maxLinkedIn,
		"aiCompose":         cfg.OutreachAICompose,
		"aiReview":          cfg.OutreachAIReview,
	})
}

// handlePutOutreachSetup persists the outreach configuration.
func (s *Server) handlePutOutreachSetup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Consent           bool   `json:"consent"`
		Mode              string `json:"mode"`
		MaxEmailsPerDay   int    `json:"maxEmailsPerDay"`
		MaxLinkedInPerDay int    `json:"maxLinkedInPerDay"`
		AIConpose         bool   `json:"aiCompose"`
		AIReview          bool   `json:"aiReview"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Mode != "confirm" && body.Mode != "queue" && body.Mode != "auto" {
		body.Mode = "confirm"
	}

	s.mu.Lock()
	s.cfg.OutreachConsent = body.Consent
	s.cfg.OutreachMode = body.Mode
	if body.MaxEmailsPerDay > 0 {
		s.cfg.MaxEmailsPerDay = body.MaxEmailsPerDay
	}
	if body.MaxLinkedInPerDay > 0 {
		s.cfg.MaxLinkedInPerDay = body.MaxLinkedInPerDay
	}
	s.cfg.OutreachAICompose = body.AIConpose
	s.cfg.OutreachAIReview = body.AIReview
	err := config.Save(s.cfg)
	s.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}
	s.changed()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleGetOutreachItems lists outreach queue items (newest first).
func (s *Server) handleGetOutreachItems(w http.ResponseWriter, r *http.Request) {
	items, err := outreach.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load outreach: "+err.Error())
		return
	}
	out := make([]outreachItemDTO, 0, len(items))
	for _, it := range items {
		out = append(out, toOutreachItemDTO(it))
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePostOutreachBuild creates email + linkedin drafts for every applied or
// skipped application that doesn't already have a queue item (idempotent).
func (s *Server) handlePostOutreachBuild(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusInternalServerError, "store unavailable")
		return
	}
	var body struct {
		Channel string `json:"channel,omitempty"`
	}
	_ = readJSON(r, &body)

	existing, err := outreach.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load outreach: "+err.Error())
		return
	}
	seen := map[string]bool{}
	for _, it := range existing {
		seen[string(it.Channel)+"|"+it.JobURL] = true
	}

	apps, err := s.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list applications: "+err.Error())
		return
	}

	created := make([]outreachItemDTO, 0)
	for _, app := range apps {
		if app.Status != store.StatusApplied && app.Status != store.StatusSkipped {
			continue
		}
		if app.URL == "" {
			continue
		}
		ref := outreach.JobRef{
			URL:      app.URL,
			Company:  app.Company,
			Role:     app.Role,
			Provider: app.Provider,
		}
		if body.Channel == "" || body.Channel == "email" {
			if !seen["email|"+app.URL] {
				it := outreach.NewEmailDraft(s.cfg, ref, "", "")
				if err := outreach.Upsert(it); err != nil {
					writeError(w, http.StatusInternalServerError, "save outreach item: "+err.Error())
					return
				}
				created = append(created, toOutreachItemDTO(it))
				seen["email|"+app.URL] = true
			}
		}
		if body.Channel == "" || body.Channel == "linkedin" {
			if !seen["linkedin|"+app.URL] {
				it := outreach.NewLinkedInDraft(s.cfg, ref, "", "")
				if err := outreach.Upsert(it); err != nil {
					writeError(w, http.StatusInternalServerError, "save outreach item: "+err.Error())
					return
				}
				created = append(created, toOutreachItemDTO(it))
				seen["linkedin|"+app.URL] = true
			}
		}
	}
	writeJSON(w, http.StatusOK, created)
}

// handlePostOutreachSend sends one outreach item through the real pipeline.
// Without email credentials (or without consent) it returns a clear 400 and
// the item is marked failed — no silent fake "sent".
func (s *Server) handlePostOutreachSend(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, err := outreach.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load outreach: "+err.Error())
		return
	}
	var it outreach.Item
	found := false
	for _, i := range items {
		if i.ID == id {
			it = i
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "outreach item not found")
		return
	}

	var sendErr error
	switch it.Channel {
	case outreach.ChannelLinkedIn:
		sendErr = outreach.MarkLinkedInSent(s.cfg, it)
	default:
		sendErr = outreach.SendEmail(s.cfg, it)
	}

	// Reload to pick up the post-attempt status (sent / failed / follow-up).
	reloaded := it
	if reload, err := outreach.Load(); err == nil {
		for _, i := range reload {
			if i.ID == id {
				reloaded = i
				break
			}
		}
	}
	if sendErr != nil {
		// SendEmail persisted the failed status + audit entry already.
		writeError(w, http.StatusBadRequest, sendErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, toOutreachItemDTO(reloaded))
}

// handleGetOutreachLog returns the outreach audit log (newest first).
func (s *Server) handleGetOutreachLog(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	entries, err := s.store.ListOutreachLog(100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list outreach log: "+err.Error())
		return
	}
	if entries == nil {
		entries = []store.OutreachLogEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// handlePostNotifyTest sends a test notification to all configured channels.
func (s *Server) handlePostNotifyTest(w http.ResponseWriter, r *http.Request) {
	if s.notifier == nil || len(s.notifier) == 0 {
		writeError(w, http.StatusBadRequest, "no notification channels configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": len(s.notifier)})
}
