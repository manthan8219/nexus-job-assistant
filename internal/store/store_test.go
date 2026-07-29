package store

import (
	"os"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	f, err := os.CreateTemp("", "nexus-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := openPath(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInsertAndExists(t *testing.T) {
	s := tempStore(t)

	url := "https://boards-api.greenhouse.io/v1/boards/notion/jobs/12345"

	exists, err := s.Exists(url)
	if err != nil || exists {
		t.Fatalf("expected not exists before insert, got err=%v exists=%v", err, exists)
	}

	err = s.Insert(Application{
		Provider:  "greenhouse",
		Company:   "Notion",
		Role:      "Software Engineer",
		URL:       url,
		Status:    StatusApplied,
		AppliedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	exists, err = s.Exists(url)
	if err != nil || !exists {
		t.Fatalf("expected exists after insert, got err=%v exists=%v", err, exists)
	}
}

func TestDuplicateInsertIgnored(t *testing.T) {
	s := tempStore(t)
	app := Application{
		Provider:  "greenhouse",
		Company:   "Stripe",
		Role:      "Backend Engineer",
		URL:       "https://example.com/job/1",
		Status:    StatusApplied,
		AppliedAt: time.Now(),
	}
	if err := s.Insert(app); err != nil {
		t.Fatal(err)
	}
	// Second insert with same URL must not error (INSERT OR IGNORE)
	if err := s.Insert(app); err != nil {
		t.Fatalf("duplicate insert should be ignored, got: %v", err)
	}
}

func TestList(t *testing.T) {
	s := tempStore(t)
	for i := 0; i < 3; i++ {
		_ = s.Insert(Application{
			Provider:  "greenhouse",
			Company:   "Company",
			Role:      "Engineer",
			URL:       "https://example.com/job/" + string(rune('A'+i)),
			Status:    StatusApplied,
			AppliedAt: time.Now(),
		})
	}
	apps, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 3 {
		t.Fatalf("expected 3 apps, got %d", len(apps))
	}
}

func TestStats(t *testing.T) {
	s := tempStore(t)
	_ = s.Insert(Application{Provider: "gh", Company: "A", Role: "E", URL: "u1", Status: StatusApplied, AppliedAt: time.Now()})
	_ = s.Insert(Application{Provider: "gh", Company: "B", Role: "E", URL: "u2", Status: StatusApplied, AppliedAt: time.Now()})
	_ = s.Insert(Application{Provider: "gh", Company: "C", Role: "E", URL: "u3", Status: StatusSkipped, AppliedAt: time.Now()})
	_ = s.Insert(Application{Provider: "gh", Company: "D", Role: "E", URL: "u4", Status: StatusFailed, AppliedAt: time.Now()})

	applied, skipped, failed, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 || skipped != 1 || failed != 1 {
		t.Errorf("Stats: applied=%d skipped=%d failed=%d, want 2/1/1", applied, skipped, failed)
	}
}
