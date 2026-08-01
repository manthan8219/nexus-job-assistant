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

func TestSetSubmittedPayloadRoundTrip(t *testing.T) {
	s := tempStore(t)

	if err := s.Insert(Application{
		Provider: "greenhouse", Company: "Acme", Role: "Engineer",
		URL: "https://example.com/payload", Status: StatusApplied,
		AppliedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	apps, err := s.List()
	if err != nil || len(apps) != 1 {
		t.Fatalf("list = %d apps, err %v; want 1", len(apps), err)
	}

	payload := `{"profile":{"first_name":"Ada"},"answers":[{"question":"Why","answer":"x"}]}`
	if err := s.SetSubmittedPayload(apps[0].ID, payload); err != nil {
		t.Fatalf("SetSubmittedPayload: %v", err)
	}

	got, err := s.GetByIDs([]int64{apps[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].SubmittedPayload != payload {
		t.Errorf("payload = %q; want %q", got[0].SubmittedPayload, payload)
	}

	// By-URL variant (used by the run loop).
	if err := s.SetSubmittedPayloadByURL("https://example.com/payload", `{"resume":{"filename":"r.pdf"}}`); err != nil {
		t.Fatalf("SetSubmittedPayloadByURL: %v", err)
	}
	got2, _ := s.GetByIDs([]int64{apps[0].ID})
	if got2[0].SubmittedPayload != `{"resume":{"filename":"r.pdf"}}` {
		t.Errorf("payload after URL update = %q", got2[0].SubmittedPayload)
	}

	// Missing id → honest error.
	if err := s.SetSubmittedPayload(999999, "{}"); err == nil {
		t.Error("expected error for unknown id")
	}
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

func TestCompanyJobCounts(t *testing.T) {
	s := tempStore(t)
	_ = s.Insert(Application{Provider: "gh", Company: "Stripe", Role: "E", URL: "u1", Status: StatusApplied, AppliedAt: time.Now()})
	_ = s.Insert(Application{Provider: "gh", Company: "Stripe", Role: "E", URL: "u2", Status: StatusSkipped, AppliedAt: time.Now()})
	_ = s.Insert(Application{Provider: "lever", Company: " stripe ", Role: "E", URL: "u3", Status: StatusFailed, AppliedAt: time.Now()})
	_ = s.Insert(Application{Provider: "gh", Company: "Notion", Role: "E", URL: "u4", Status: StatusApplied, AppliedAt: time.Now()})

	counts, err := s.CompanyJobCounts()
	if err != nil {
		t.Fatal(err)
	}
	if got := counts["stripe"]; got != 3 {
		t.Errorf("counts[stripe] = %d, want 3", got)
	}
	if got := counts["notion"]; got != 1 {
		t.Errorf("counts[notion] = %d, want 1", got)
	}
	if got := counts["missing"]; got != 0 {
		t.Errorf("counts[missing] = %d, want 0", got)
	}
}

func TestListByCompany(t *testing.T) {
	s := tempStore(t)
	_ = s.Insert(Application{Provider: "gh", Company: "Stripe", Role: "Backend", URL: "u1", Status: StatusApplied, AppliedAt: time.Now().Add(-time.Hour)})
	_ = s.Insert(Application{Provider: "gh", Company: "Stripe", Role: "Frontend", URL: "u2", Status: StatusSkipped, AppliedAt: time.Now()})
	_ = s.Insert(Application{Provider: "gh", Company: "Notion", Role: "PM", URL: "u3", Status: StatusApplied, AppliedAt: time.Now()})

	apps, err := s.ListByCompany(" STRIPE ")
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps for stripe, got %d", len(apps))
	}
	if apps[0].Role != "Frontend" {
		t.Errorf("newest first: apps[0].Role = %q, want Frontend", apps[0].Role)
	}
	for _, a := range apps {
		if CompanyKey(a.Company) != "stripe" {
			t.Errorf("unexpected company %q in stripe results", a.Company)
		}
	}
}

func TestCompanyKey(t *testing.T) {
	if got := CompanyKey("  Stripe "); got != "stripe" {
		t.Errorf("CompanyKey trims+lowercases: got %q", got)
	}
	if got := CompanyKey(""); got != "" {
		t.Errorf("CompanyKey empty: got %q", got)
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

func TestNextOutcomeCycle(t *testing.T) {
	tests := []struct {
		name string
		in   Outcome
		want Outcome
	}{
		{"none starts cycle", OutcomeNone, OutcomeReplied},
		{"replied → interview", OutcomeReplied, OutcomeInterview},
		{"interview → offer", OutcomeInterview, OutcomeOffer},
		{"offer → rejected", OutcomeOffer, OutcomeRejected},
		{"rejected → ghosted", OutcomeRejected, OutcomeGhosted},
		{"ghosted wraps to none", OutcomeGhosted, OutcomeNone},
		{"unknown starts cycle", Outcome("bogus"), OutcomeReplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextOutcome(tt.in); got != tt.want {
				t.Errorf("NextOutcome(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidOutcome(t *testing.T) {
	tests := []struct {
		in   Outcome
		want bool
	}{
		{OutcomeNone, true},
		{OutcomeReplied, true},
		{OutcomeInterview, true},
		{OutcomeOffer, true},
		{OutcomeRejected, true},
		{OutcomeGhosted, true},
		{Outcome("hired"), false},
		{Outcome("REPLIED"), false},
	}
	for _, tt := range tests {
		if got := ValidOutcome(tt.in); got != tt.want {
			t.Errorf("ValidOutcome(%q) = %v; want %v", tt.in, got, tt.want)
		}
	}
}

func TestSetOutcomeRoundTrip(t *testing.T) {
	s := tempStore(t)
	url := "https://example.com/job/outcome"
	_ = s.Insert(Application{
		Provider: "gh", Company: "Stripe", Role: "Backend",
		URL: url, Status: StatusApplied, AppliedAt: time.Now(),
	})
	apps, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	id := apps[0].ID
	if apps[0].Outcome != OutcomeNone {
		t.Fatalf("fresh insert outcome = %q; want empty", apps[0].Outcome)
	}

	if err := s.SetOutcome(id, OutcomeInterview); err != nil {
		t.Fatalf("SetOutcome: %v", err)
	}
	apps, _ = s.List()
	if apps[0].Outcome != OutcomeInterview {
		t.Errorf("outcome = %q; want %q", apps[0].Outcome, OutcomeInterview)
	}
	if apps[0].OutcomeAt.IsZero() {
		t.Errorf("OutcomeAt should be set when outcome is non-empty")
	}

	// Clearing returns to none and zeroes the timestamp.
	if err := s.SetOutcome(id, OutcomeNone); err != nil {
		t.Fatalf("SetOutcome clear: %v", err)
	}
	apps, _ = s.List()
	if apps[0].Outcome != OutcomeNone {
		t.Errorf("cleared outcome = %q; want empty", apps[0].Outcome)
	}
}

func TestSetOutcomeRejectsInvalid(t *testing.T) {
	s := tempStore(t)
	if err := s.SetOutcome(1, Outcome("hired")); err == nil {
		t.Errorf("SetOutcome with invalid value should error")
	}
	if err := s.SetOutcome(999, OutcomeReplied); err == nil {
		t.Errorf("SetOutcome with unknown id should error")
	}
}

func TestSetOutcomeByURL(t *testing.T) {
	s := tempStore(t)
	url := "https://example.com/job/byurl"
	_ = s.Insert(Application{
		Provider: "lever", Company: "Notion", Role: "PM",
		URL: url, Status: StatusApplied, AppliedAt: time.Now(),
	})

	ok, err := s.SetOutcomeByURL(url, OutcomeReplied)
	if err != nil || !ok {
		t.Fatalf("SetOutcomeByURL = %v, %v; want true, nil", ok, err)
	}
	apps, _ := s.List()
	if apps[0].Outcome != OutcomeReplied {
		t.Errorf("outcome = %q; want %q", apps[0].Outcome, OutcomeReplied)
	}

	ok, err = s.SetOutcomeByURL("https://example.com/unknown", OutcomeReplied)
	if err != nil || ok {
		t.Errorf("unknown URL = %v, %v; want false, nil", ok, err)
	}
}

func TestOutcomeStats(t *testing.T) {
	s := tempStore(t)
	insert := func(url string, o Outcome) {
		_ = s.Insert(Application{
			Provider: "gh", Company: "C", Role: "E",
			URL: url, Status: StatusApplied, AppliedAt: time.Now(),
		})
		apps, _ := s.List()
		for _, a := range apps {
			if a.URL == url && o != OutcomeNone {
				_ = s.SetOutcome(a.ID, o)
			}
		}
	}
	insert("u1", OutcomeReplied)
	insert("u2", OutcomeReplied)
	insert("u3", OutcomeInterview)
	insert("u4", OutcomeNone)

	counts, err := s.OutcomeStats()
	if err != nil {
		t.Fatal(err)
	}
	if got := counts[OutcomeReplied]; got != 2 {
		t.Errorf("replied = %d; want 2", got)
	}
	if got := counts[OutcomeInterview]; got != 1 {
		t.Errorf("interview = %d; want 1", got)
	}
	if got := counts[OutcomeOffer]; got != 0 {
		t.Errorf("offer = %d; want 0", got)
	}
}
