package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/companies"
)

// Company mirrors the frontend Company type.
type Company struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	Website       string   `json:"website"`
	ATS           string   `json:"ats"`
	Board         string   `json:"board"`
	BoardURL      string   `json:"boardURL"`
	HireCountries []string `json:"hireCountries"`
	HQCountry     string   `json:"hqCountry"`
	Kind          string   `json:"kind"`
	Industry      string   `json:"industry"`
	Source        string   `json:"source"`
	UpdatedAt     string   `json:"updatedAt"`
}

// CompaniesResult mirrors the frontend CompaniesResult type.
type CompaniesResult struct {
	Items  []Company      `json:"items"`
	Total  int            `json:"total"`
	Counts map[string]int `json:"counts"`
}

// companyKey matches the frontend's CompanyKey (lowercase, non-alnum → '-').
func companyKey(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func companyToFrontend(c companies.Company) Company {
	return Company{
		ID:            c.ID,
		Name:          c.Name,
		Website:       c.Website,
		ATS:           c.ATS,
		Board:         c.Board,
		BoardURL:      c.BoardURL,
		HireCountries: c.HireCountries,
		HQCountry:     c.HQCountry,
		Kind:          c.Kind,
		Industry:      c.Industry,
		Source:        c.Source,
		UpdatedAt:     c.UpdatedAt.Format(time.RFC3339),
	}
}

// splitCountries parses a comma-separated country list into display names.
func splitCountries(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// handleGetCompanies returns the company list, optionally filtered.
func (s *Server) handleGetCompanies(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	country := r.URL.Query().Get("country")

	items := make([]Company, 0)
	if s.companies != nil {
		list, err := s.companies.Search(q, country, 200)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "search companies: "+err.Error())
			return
		}
		for _, c := range list {
			items = append(items, companyToFrontend(c))
		}
	}

	counts := map[string]int{}
	if s.store != nil {
		if raw, err := s.store.CompanyJobCounts(); err == nil {
			for name, n := range raw {
				counts[companyKey(name)] = n
			}
		}
	}

	writeJSON(w, http.StatusOK, CompaniesResult{
		Items:  items,
		Total:  len(items),
		Counts: counts,
	})
}

// handlePutCompany adds or edits a company in the persisted store.
func (s *Server) handlePutCompany(w http.ResponseWriter, r *http.Request) {
	if s.companies == nil {
		writeError(w, http.StatusInternalServerError, "companies store not available")
		return
	}

	var input struct {
		Name      string `json:"name"`
		Website   string `json:"website"`
		BoardURL  string `json:"boardURL"`
		Countries string `json:"countries"`
		ATS       string `json:"ats"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	name := strings.TrimSpace(input.Name)
	boardURL := strings.TrimSpace(input.BoardURL)
	if name == "" || boardURL == "" {
		writeError(w, http.StatusBadRequest, "name and boardURL are required")
		return
	}

	c := companies.Company{
		Name:          name,
		Website:       strings.TrimSpace(input.Website),
		ATS:           strings.TrimSpace(input.ATS),
		BoardURL:      boardURL,
		HireCountries: splitCountries(input.Countries),
		Source:        "manual",
		UpdatedAt:     time.Now().UTC(),
	}
	if err := s.companies.Upsert(c); err != nil {
		writeError(w, http.StatusInternalServerError, "save company: "+err.Error())
		return
	}

	// Reload to pick up the generated id, then return the frontend shape.
	found, err := s.companies.Search(name, "", 1)
	if err == nil && len(found) > 0 {
		writeJSON(w, http.StatusOK, companyToFrontend(found[0]))
		return
	}
	writeError(w, http.StatusInternalServerError, "saved company not found after upsert")
}

// handlePostCompaniesRefresh upserts the embedded catalog and returns the
// number of companies upserted.
func (s *Server) handlePostCompaniesRefresh(w http.ResponseWriter, r *http.Request) {
	if s.companies == nil {
		writeError(w, http.StatusInternalServerError, "companies store not available")
		return
	}
	n := 0
	if c, err := s.companies.ImportNexusEmbeddedBoards(); err == nil {
		n += c
	}
	if c, err := s.companies.ImportIndiaEmployers(); err == nil {
		n += c
	}
	writeJSON(w, http.StatusOK, n)
}

// handleGetCompanyJobs returns recorded applications for a company.
func (s *Server) handleGetCompanyJobs(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusInternalServerError, "store not available")
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "company name is required")
		return
	}
	apps, err := s.store.ListByCompany(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list company jobs: "+err.Error())
		return
	}
	frontend := make([]Application, len(apps))
	for i, a := range apps {
		frontend[i] = storeAppToFrontend(a)
	}
	writeJSON(w, http.StatusOK, frontend)
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
