// Package tests — see notifier_fanout_test.go for the package doc.
package tests

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// Idempotency at the persistence boundary (AGENTS.md section 14): a job URL is
// never recorded twice — inserting the same URL again is a silent no-op so a
// re-run or retry cannot create duplicate application records. Uses the public
// store.OpenPath constructor with a t.TempDir() file (no ~/.nexus, no network).
func TestStoreIdempotency_SameURLRecordedOnce(t *testing.T) {
	st, err := store.OpenPath(filepath.Join(t.TempDir(), "idem.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	url := "https://boards-api.greenhouse.io/v1/boards/notion/jobs/12345"
	if exists, err := st.Exists(url); err != nil || exists {
		t.Fatalf("expected not exists before insert; got exists=%v err=%v", exists, err)
	}

	ins := func() {
		t.Helper()
		if err := st.Insert(store.Application{
			Provider: "greenhouse", Company: "Notion", Role: "Software Engineer",
			URL: url, Status: store.StatusApplied, AppliedAt: time.Now(),
		}); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	ins()
	ins() // retry / re-run: must be ignored, not a duplicate row

	if exists, err := st.Exists(url); err != nil || !exists {
		t.Errorf("expected exists after inserts; got exists=%v err=%v", exists, err)
	}
	rows, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("duplicate URL must not be recorded twice; got %d rows", len(rows))
	}
}

// OpenPath creates the schema on a fresh path (hermetic), and distinct URLs are
// stored as distinct rows.
func TestStoreOpenPath_DistinctURLsAreDistinctRows(t *testing.T) {
	st, err := store.OpenPath(filepath.Join(t.TempDir(), "distinct.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	base := store.Application{
		Provider: "greenhouse", Company: "Acme", Role: "X",
		Status: store.StatusApplied, AppliedAt: time.Now(),
	}
	for _, u := range []string{"https://example.com/a", "https://example.com/b"} {
		app := base
		app.URL = u
		if err := st.Insert(app); err != nil {
			t.Fatalf("Insert %s: %v", u, err)
		}
	}
	rows, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 distinct rows; got %d", len(rows))
	}
}
