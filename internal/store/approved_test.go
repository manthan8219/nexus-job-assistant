package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openLegacyDB creates a database with the pre-approved schema (as shipped
// before the review-queue feature) and inserts one application.
func openLegacyDB(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE applications (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	provider    TEXT    NOT NULL,
	company     TEXT    NOT NULL,
	role        TEXT    NOT NULL,
	url         TEXT    NOT NULL UNIQUE,
	status      TEXT    NOT NULL DEFAULT 'applied',
	reason      TEXT    NOT NULL DEFAULT '',
	applied_at  DATETIME NOT NULL,
	location    TEXT    NOT NULL DEFAULT '',
	remote      INTEGER NOT NULL DEFAULT 0,
	posted_at   DATETIME NOT NULL DEFAULT '0001-01-01T00:00:00Z',
	description TEXT    NOT NULL DEFAULT '',
	fit_score   INTEGER NOT NULL DEFAULT 0,
	fit_summary TEXT    NOT NULL DEFAULT '',
	outcome     TEXT    NOT NULL DEFAULT '',
	outcome_at  DATETIME NOT NULL DEFAULT '0001-01-01T00:00:00Z'
);`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(
		`INSERT INTO applications (provider, company, role, url, status, applied_at)
		 VALUES ('greenhouse', 'Acme', 'Engineer', 'https://x/1', 'queued', ?)`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestApprovedMigrationAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	legacy := openLegacyDB(t, dir)

	st, err := OpenAt(legacy)
	if err != nil {
		t.Fatalf("OpenAt on legacy db: %v", err)
	}
	defer st.Close()

	// Legacy row opens with Approved=false and StatusQueued intact.
	apps, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("List len = %d; want 1", len(apps))
	}
	if apps[0].Approved {
		t.Error("legacy row should default approved=false")
	}
	if apps[0].Status != StatusQueued {
		t.Errorf("status = %q; want %q", apps[0].Status, StatusQueued)
	}

	// SetApproved round-trips through GetByIDs.
	if err := st.SetApproved(apps[0].ID, true); err != nil {
		t.Fatalf("SetApproved(true): %v", err)
	}
	got, err := st.GetByIDs([]int64{apps[0].ID})
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	if len(got) != 1 || !got[0].Approved {
		t.Errorf("GetByIDs approved = %v; want true", got)
	}
	if err := st.SetApproved(apps[0].ID, false); err != nil {
		t.Fatalf("SetApproved(false): %v", err)
	}
}

func TestSetStatus(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenAt(filepath.Join(dir, "apps.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UTC()
	if err := st.Insert(Application{
		Provider: "lever", Company: "Globex", Role: "PM", URL: "https://x/2",
		Status: StatusQueued, AppliedAt: now,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	apps, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	id := apps[0].ID

	if err := st.SetStatus(id, StatusApplied, ""); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	got, err := st.GetByIDs([]int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status != StatusApplied {
		t.Errorf("status = %q; want applied", got[0].Status)
	}
	if got[0].AppliedAt.IsZero() {
		t.Error("applied_at should be stamped by SetStatus")
	}
}
