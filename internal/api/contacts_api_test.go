package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/contacts"
)

func TestContactsSearchRequiresParam(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/contacts/search", nil)
	rr := httptest.NewRecorder()
	srv.handleGetContactsSearch(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rr.Code)
	}
}

func TestContactsSearchReturnsPatternContacts(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/contacts/search?company=Acme&domain=acme.com", nil)
	rr := httptest.NewRecorder()
	srv.handleGetContactsSearch(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}

	var res struct {
		Contacts []struct {
			Email   string `json:"email"`
			Source  string `json:"source"`
			Name    string `json:"name"`
			FoundAt string `json:"foundAt"`
		} `json:"contacts"`
		Sources []string `json:"sources"`
		Errors  []string `json:"errors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Contacts) == 0 {
		t.Fatal("expected pattern contacts even without API keys")
	}
	if res.Contacts[0].Source != "pattern" {
		t.Errorf("first contact source = %q; want pattern", res.Contacts[0].Source)
	}
	if res.Contacts[0].Email == "" {
		t.Error("expected a generated email")
	}
	// JSON field names must match the frontend contract (camelCase).
	if res.Contacts[0].FoundAt == "" {
		t.Error("expected foundAt JSON key (camelCase)")
	}
}

func TestContactsSavedCRUD(t *testing.T) {
	cdb, err := contacts.Open(filepath.Join(t.TempDir(), "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cdb.Close()
	srv := &Server{contacts: cdb}

	// Save.
	body, _ := json.Marshal(map[string]any{
		"company": "Stripe", "domain": "stripe.com",
		"name": "Hiring Team", "title": "Recruiter",
		"email": "hiring@stripe.com", "emailType": "pattern",
		"source": "pattern", "confidence": 40,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/contacts/saved", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handlePutContactsSaved(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("save status = %d; body %s", rr.Code, rr.Body.String())
	}
	var saved struct {
		ID  int64  `json:"id"`
		URL string `json:"linkedIn"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ID == 0 {
		t.Error("expected an id on the saved contact")
	}

	// List.
	req = httptest.NewRequest(http.MethodGet, "/api/contacts/saved", nil)
	rr = httptest.NewRecorder()
	srv.handleGetContactsSaved(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d", rr.Code)
	}
	var items []struct {
		ID      int64  `json:"id"`
		Company string `json:"company"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Company != "Stripe" {
		t.Fatalf("unexpected saved list: %+v", items)
	}

	// Delete.
	req = httptest.NewRequest(http.MethodDelete, "/api/contacts/saved/1", nil)
	req.SetPathValue("id", "1")
	rr = httptest.NewRecorder()
	srv.handleDeleteContactsSaved(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d; body %s", rr.Code, rr.Body.String())
	}

	// Delete a missing one → 404.
	req = httptest.NewRequest(http.MethodDelete, "/api/contacts/saved/1", nil)
	req.SetPathValue("id", "1")
	rr = httptest.NewRecorder()
	srv.handleDeleteContactsSaved(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing delete status = %d; want 404", rr.Code)
	}

	// Save without email/linkedIn → 400.
	bad, _ := json.Marshal(map[string]any{"company": "X"})
	req = httptest.NewRequest(http.MethodPut, "/api/contacts/saved", bytes.NewReader(bad))
	rr = httptest.NewRecorder()
	srv.handlePutContactsSaved(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid save status = %d; want 400", rr.Code)
	}
}
