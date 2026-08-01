package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/companies"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func newCompaniesServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.OpenAt(filepath.Join(t.TempDir(), "apps.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	cdb, err := companies.Open(filepath.Join(t.TempDir(), "companies.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cdb.Close() })
	return &Server{store: st, companies: cdb}
}

func TestCompaniesCRUD(t *testing.T) {
	srv := newCompaniesServer(t)

	// Initial GET → empty.
	req := httptest.NewRequest(http.MethodGet, "/api/companies", nil)
	rr := httptest.NewRecorder()
	srv.handleGetCompanies(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var initial CompaniesResult
	if err := json.Unmarshal(rr.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Total != 0 || len(initial.Items) != 0 {
		t.Errorf("expected empty companies, got total=%d items=%d", initial.Total, len(initial.Items))
	}

	// PUT a company.
	body, _ := json.Marshal(map[string]string{
		"name": "Acme Health", "website": "https://acme.health",
		"boardURL": "https://boards.greenhouse.io/acmehealth", "ats": "greenhouse",
		"countries": "Remote, US",
	})
	put := httptest.NewRequest(http.MethodPut, "/api/companies", bytes.NewReader(body))
	putRR := httptest.NewRecorder()
	srv.handlePutCompany(putRR, put)
	if putRR.Code != http.StatusOK {
		t.Fatalf("put status = %d; body %s", putRR.Code, putRR.Body.String())
	}
	var created Company
	if err := json.Unmarshal(putRR.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "Acme Health" || created.ID == 0 {
		t.Errorf("unexpected created company: %+v", created)
	}
	if len(created.HireCountries) != 2 {
		t.Errorf("hire countries = %v; want 2", created.HireCountries)
	}

	// GET lists it.
	list := httptest.NewRequest(http.MethodGet, "/api/companies", nil)
	listRR := httptest.NewRecorder()
	srv.handleGetCompanies(listRR, list)
	var result CompaniesResult
	_ = json.Unmarshal(listRR.Body.Bytes(), &result)
	if result.Total < 1 || result.Items[0].Name != "Acme Health" {
		t.Errorf("company not listed: %+v", result)
	}

	// GET with ?q= filter finds it.
	filtered := httptest.NewRequest(http.MethodGet, "/api/companies?q=acme", nil)
	filteredRR := httptest.NewRecorder()
	srv.handleGetCompanies(filteredRR, filtered)
	var filteredResult CompaniesResult
	_ = json.Unmarshal(filteredRR.Body.Bytes(), &filteredResult)
	if filteredResult.Total != 1 {
		t.Errorf("q=acme total = %d; want 1", filteredResult.Total)
	}

	// Missing fields → 400.
	bad := httptest.NewRequest(http.MethodPut, "/api/companies",
		bytes.NewReader([]byte(`{"name":"","boardURL":""}`)))
	badRR := httptest.NewRecorder()
	srv.handlePutCompany(badRR, bad)
	if badRR.Code != http.StatusBadRequest {
		t.Errorf("bad input status = %d; want 400", badRR.Code)
	}
}

func TestCompaniesRefreshAndJobs(t *testing.T) {
	srv := newCompaniesServer(t)

	// Refresh upserts the embedded catalog and returns a bare count.
	ref := httptest.NewRequest(http.MethodPost, "/api/companies/refresh", nil)
	refRR := httptest.NewRecorder()
	srv.handlePostCompaniesRefresh(refRR, ref)
	if refRR.Code != http.StatusOK {
		t.Fatalf("refresh status = %d", refRR.Code)
	}
	var n int
	if err := json.Unmarshal(refRR.Body.Bytes(), &n); err != nil {
		t.Fatalf("refresh should return a bare number, got %q", refRR.Body.String())
	}
	if n <= 0 {
		t.Errorf("refresh count = %d; want > 0", n)
	}

	// Seed an application so company-jobs drill-down has data.
	_ = srv.store.Insert(store.Application{
		Provider: "manual", Company: "Acme Health", Role: "Cardiologist",
		URL: "https://acme.health/careers/cardio", Status: store.StatusQueued,
		AppliedAt: time.Now().UTC(),
	})
	jobs := httptest.NewRequest(http.MethodGet, "/api/companies/Acme%20Health/jobs", nil)
	jobs.SetPathValue("name", "Acme Health")
	jobsRR := httptest.NewRecorder()
	srv.handleGetCompanyJobs(jobsRR, jobs)
	if jobsRR.Code != http.StatusOK {
		t.Fatalf("company jobs status = %d", jobsRR.Code)
	}
	var apps []Application
	if err := json.Unmarshal(jobsRR.Body.Bytes(), &apps); err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].Role != "Cardiologist" {
		t.Errorf("company jobs = %+v; want 1 Cardiologist", apps)
	}
	if !strings.Contains(string(jobsRR.Body.Bytes()), "fitScore") {
		t.Errorf("company jobs response should use camelCase frontend shape")
	}
}
