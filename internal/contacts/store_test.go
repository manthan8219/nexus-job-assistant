package contacts

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/osint"
)

func TestStoreSaveListDelete(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if n, _ := db.Count(); n != 0 {
		t.Fatalf("expected empty store, got %d", n)
	}

	c := osint.Contact{
		Company:    "Acme Health",
		Domain:     "acme.health",
		Name:       "Recruiting Team",
		Title:      "Talent Acquisition",
		Email:      "careers@acme.health",
		EmailType:  "pattern",
		Source:     "pattern",
		Confidence: 25,
	}
	saved, err := db.Save(c)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == 0 {
		t.Error("expected a generated id")
	}
	if saved.FoundAt.IsZero() {
		t.Error("expected a foundAt timestamp")
	}

	// Upsert on the same contact keeps the same row.
	saved2, err := db.Save(c)
	if err != nil {
		t.Fatal(err)
	}
	if saved2.ID != saved.ID {
		t.Errorf("upsert should keep id %d, got %d", saved.ID, saved2.ID)
	}

	// List finds it; query filter matches.
	items, err := db.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Email != "careers@acme.health" {
		t.Fatalf("unexpected list: %+v", items)
	}
	filtered, err := db.List("acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Errorf("expected 1 filtered contact, got %d", len(filtered))
	}

	// Delete removes it.
	if err := db.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.Count(); n != 0 {
		t.Errorf("expected 0 after delete, got %d", n)
	}
	if err := db.Delete(saved.ID); err == nil {
		t.Error("expected an error deleting a missing contact")
	}
}

func TestStorePersistsFoundAt(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ts := time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC)
	c := osint.Contact{Company: "X", Email: "a@x.com", FoundAt: ts}
	saved, err := db.Save(c)
	if err != nil {
		t.Fatal(err)
	}
	items, _ := db.List("")
	if len(items) != 1 || !items[0].FoundAt.Equal(ts) {
		t.Errorf("foundAt should round-trip (%v), got %v", ts, items[0].FoundAt)
	}
	_ = saved
}
