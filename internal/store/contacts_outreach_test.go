package store

import (
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/osint"
)

func TestContactsLifecycle(t *testing.T) {
	st := tempStore(t)
	c := osint.Contact{
		Company: "Acme", Domain: "acme.com", Name: "Jane Recruiter",
		Email: "jane@acme.com", Source: "hunter", Confidence: 90, Notes: "hi",
	}
	if err := st.SaveContact(c); err != nil {
		t.Fatalf("SaveContact: %v", err)
	}
	// Duplicate (same company + email) is ignored.
	if err := st.SaveContact(c); err != nil {
		t.Fatalf("SaveContact dup: %v", err)
	}

	contacts, err := st.ListContacts()
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("ListContacts = %d, want 1 (duplicate ignored)", len(contacts))
	}
	if contacts[0].Email != "jane@acme.com" {
		t.Errorf("contact email = %q, want jane@acme.com", contacts[0].Email)
	}

	domain, err := st.DomainForCompany("acme") // case-insensitive lookup
	if err != nil {
		t.Fatalf("DomainForCompany: %v", err)
	}
	if domain != "acme.com" {
		t.Errorf("DomainForCompany = %q, want acme.com", domain)
	}

	if err := st.DeleteContact(contacts[0].ID); err != nil {
		t.Fatalf("DeleteContact: %v", err)
	}
	after, _ := st.ListContacts()
	if len(after) != 0 {
		t.Errorf("contacts after delete = %d, want 0", len(after))
	}
}

func TestOutreachLogLifecycle(t *testing.T) {
	st := tempStore(t)
	entry := OutreachLogEntry{
		Channel: "email", Company: "Acme", Role: "Engineer",
		ContactEmail: "jane@acme.com", Status: "sent", Subject: "Hello",
	}
	if err := st.SaveOutreachLog(entry); err != nil {
		t.Fatalf("SaveOutreachLog: %v", err)
	}

	logs, err := st.ListOutreachLog(0)
	if err != nil {
		t.Fatalf("ListOutreachLog: %v", err)
	}
	if len(logs) != 1 || logs[0].Company != "Acme" || logs[0].Status != "sent" {
		t.Errorf("outreach log = %+v, want one Acme/sent entry", logs)
	}

	limited, err := st.ListOutreachLog(1)
	if err != nil {
		t.Fatalf("ListOutreachLog(1): %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("ListOutreachLog(1) = %d entries, want 1", len(limited))
	}
}

func TestUpdateDescriptionFitAndMissing(t *testing.T) {
	st := tempStore(t)
	app := Application{
		Provider: "greenhouse", Company: "Acme", Role: "Engineer",
		URL: "https://example.com/job1", Status: StatusQueued, AppliedAt: time.Now(),
	}
	if err := st.Insert(app); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	apps, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	id := apps[0].ID

	missing, err := st.ListMissingDescription()
	if err != nil {
		t.Fatalf("ListMissingDescription: %v", err)
	}
	if len(missing) != 1 {
		t.Fatalf("ListMissingDescription = %d, want 1", len(missing))
	}

	if err := st.UpdateDescriptionFit("https://example.com/job1", "job description", 85, "strong match"); err != nil {
		t.Fatalf("UpdateDescriptionFit: %v", err)
	}
	got, err := st.GetByIDs([]int64{id})
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	if got[0].Description != "job description" || got[0].FitScore != 85 || got[0].FitSummary != "strong match" {
		t.Errorf("updated app = %+v", got[0])
	}

	missing, _ = st.ListMissingDescription()
	if len(missing) != 0 {
		t.Errorf("ListMissingDescription after update = %d, want 0", len(missing))
	}
}

func TestCountAppliedSince(t *testing.T) {
	st := tempStore(t)
	now := time.Now().UTC()
	if err := st.Insert(Application{
		Provider: "greenhouse", Company: "Acme", Role: "A",
		URL: "https://example.com/a", Status: StatusApplied, AppliedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Insert(Application{
		Provider: "greenhouse", Company: "Acme", Role: "B",
		URL: "https://example.com/b", Status: StatusApplied, AppliedAt: now.Add(-48 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	n, err := st.CountAppliedSince(dayStart)
	if err != nil {
		t.Fatalf("CountAppliedSince: %v", err)
	}
	if n != 1 {
		t.Errorf("CountAppliedSince = %d, want 1 (only today's application)", n)
	}
}
