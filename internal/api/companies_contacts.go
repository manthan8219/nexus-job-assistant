package api

import "net/http"

// handleGetCompanies returns the company list (stub — returns empty).
func (s *Server) handleGetCompanies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  []any{},
		"total":  0,
		"counts": map[string]int{},
	})
}

// handlePutCompany adds or edits a company (stub).
func (s *Server) handlePutCompany(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id": 0, "name": "", "message": "company CRUD coming soon",
	})
}

// handlePostCompaniesRefresh refreshes company list from network (stub).
func (s *Server) handlePostCompaniesRefresh(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"count": 0})
}

// handleGetCompanyJobs returns scraped jobs for a company (stub).
func (s *Server) handleGetCompanyJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

// handleGetContactsSearch searches for contacts via OSINT (stub).
func (s *Server) handleGetContactsSearch(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

// handleGetContactsSaved returns saved contacts (stub).
func (s *Server) handleGetContactsSaved(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

// handlePutContactsSaved saves a contact (stub).
func (s *Server) handlePutContactsSaved(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id": 0, "message": "contact CRUD coming soon",
	})
}

// handleDeleteContactsSaved deletes a saved contact (stub).
func (s *Server) handleDeleteContactsSaved(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
