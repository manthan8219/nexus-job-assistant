package settings

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// sqliteDDL mirrors the production Postgres schema with SQLite-compatible
// types (TEXT instead of BYTEA, no TIMESTAMPTZ) so unit tests can run
// hermetically against a file-backed database in t.TempDir().
const sqliteDDL = `
CREATE TABLE IF NOT EXISTS user_settings (
    id INT PRIMARY KEY DEFAULT 1,
    first_name TEXT DEFAULT '',
    last_name TEXT DEFAULT '',
    email TEXT DEFAULT '',
    gmail_password_encrypted TEXT DEFAULT '',
    outreach_consent BOOLEAN DEFAULT FALSE,
    inbox_scan_minutes INT DEFAULT 60,
    city TEXT DEFAULT '',
    phone TEXT DEFAULT '',
    linkedin_id TEXT DEFAULT '',
    target_job_titles TEXT DEFAULT '',
    work_type TEXT DEFAULT '',
    target_locations TEXT DEFAULT '',
    currency TEXT DEFAULT '',
    min_salary TEXT DEFAULT '',
    apply_consent BOOLEAN DEFAULT FALSE,
    max_apps_per_run INT DEFAULT 10,
    max_apps_per_day INT DEFAULT 25,
    apply_delay_sec INT DEFAULT 3,
    min_fit_score INT DEFAULT 0,
    company_blocklist TEXT DEFAULT '',
    notice_period_days TEXT DEFAULT '',
    office_days_per_week TEXT DEFAULT '',
    cover_letter_mode TEXT DEFAULT '',
    work_auth TEXT DEFAULT '',
    resume_path TEXT DEFAULT '',
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS encryption_key (
    id INT PRIMARY KEY DEFAULT 1,
    encrypted_key TEXT NOT NULL,
    nonce TEXT NOT NULL
);
`

// newTestStore opens a file-backed SQLite database in a temp dir, creates the
// schema, and returns a Store bound to it plus the raw *sql.DB for assertions.
func newTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(sqliteDDL); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return NewStore(db, []byte("test-master-key")), db
}
func TestEnsureSchemaCreatesTables(t *testing.T) {
	ctx := context.Background()
	s, raw := newTestStore(t)

	// The tables already exist from newTestStore — drop and recreate via
	// EnsureSchema to prove it works against an empty database.
	if _, err := raw.Exec(`DROP TABLE IF EXISTS user_settings`); err != nil {
		t.Fatalf("drop user_settings: %v", err)
	}
	if _, err := raw.Exec(`DROP TABLE IF EXISTS encryption_key`); err != nil {
		t.Fatalf("drop encryption_key: %v", err)
	}

	if err := s.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema on sqlite should work: %v", err)
	}

	// Prove both tables exist.
	var n int
	if err := raw.QueryRow("SELECT COUNT(1) FROM user_settings").Scan(&n); err != nil {
		t.Errorf("user_settings not created: %v", err)
	}
	if err := raw.QueryRow("SELECT COUNT(1) FROM encryption_key").Scan(&n); err != nil {
		t.Errorf("encryption_key not created: %v", err)
	}
}
func TestSaveAndLoad(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)

	in := &ConfigOverrides{
		FirstName:         "Ada",
		LastName:          "Lovelace",
		Email:             "ada@example.com",
		GmailAppPassword:  "mysecret",
		OutreachConsent:   true,
		InboxScanMinutes:  120,
		City:              "London",
		Phone:             "+44 20 7946 0958",
		LinkedInID:        "ada-lovelace",
		TargetJobTitles:   "Engineer, Architect",
		WorkType:          "Remote",
		TargetLocations:   "UK, EU",
		Currency:          "GBP",
		MinSalary:         "80000",
		ApplyConsent:      true,
		MaxAppsPerRun:     5,
		MaxAppsPerDay:     12,
		ApplyDelaySec:     7,
		MinFitScore:       70,
		CompanyBlocklist:  "Acme, Globex",
		NoticePeriodDays:  "30",
		OfficeDaysPerWeek: "2",
		CoverLetterMode:   "concise",
		WorkAuth:          "citizen",
		ResumePath:        "/home/ada/resume.pdf",
	}
	if err := s.Save(ctx, in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.FirstName != in.FirstName || got.LastName != in.LastName || got.Email != in.Email {
		t.Errorf("identity = %q %q %q; want %q %q %q", got.FirstName, got.LastName, got.Email, in.FirstName, in.LastName, in.Email)
	}
	if got.GmailAppPassword != "mysecret" {
		t.Errorf("GmailAppPassword = %q; want %q", got.GmailAppPassword, "mysecret")
	}
	if got.OutreachConsent != true || got.ApplyConsent != true {
		t.Errorf("consents = outreach:%v apply:%v; want both true", got.OutreachConsent, got.ApplyConsent)
	}
	if got.InboxScanMinutes != 120 || got.MaxAppsPerRun != 5 || got.MaxAppsPerDay != 12 || got.ApplyDelaySec != 7 || got.MinFitScore != 70 {
		t.Errorf("ints = %d/%d/%d/%d/%d", got.InboxScanMinutes, got.MaxAppsPerRun, got.MaxAppsPerDay, got.ApplyDelaySec, got.MinFitScore)
	}
	if got.City != in.City || got.TargetJobTitles != in.TargetJobTitles || got.WorkType != in.WorkType ||
		got.TargetLocations != in.TargetLocations || got.Currency != in.Currency || got.MinSalary != in.MinSalary ||
		got.CompanyBlocklist != in.CompanyBlocklist || got.NoticePeriodDays != in.NoticePeriodDays ||
		got.OfficeDaysPerWeek != in.OfficeDaysPerWeek || got.CoverLetterMode != in.CoverLetterMode ||
		got.WorkAuth != in.WorkAuth || got.ResumePath != in.ResumePath || got.Phone != in.Phone || got.LinkedInID != in.LinkedInID {
		t.Errorf("text fields differ: got %+v want %+v", got, in)
	}

	// UPSERT path: saving again must update, not duplicate or fail.
	in.MinSalary = "90000"
	in.City = "Cambridge"
	if err := s.Save(ctx, in); err != nil {
		t.Fatalf("Save #2: %v", err)
	}
	got2, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load #2: %v", err)
	}
	if got2.MinSalary != "90000" || got2.City != "Cambridge" {
		t.Errorf("after upsert = %q/%q; want 90000/Cambridge", got2.MinSalary, got2.City)
	}

	var rows int
	if err := s.db.QueryRow("SELECT COUNT(1) FROM user_settings").Scan(&rows); err != nil || rows != 1 {
		t.Errorf("row count = %d, err %v; want exactly 1", rows, err)
	}
}
func TestLoadEmptyReturnsEmptyOverrides(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)

	got, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load on empty table = %v; want nil", err)
	}
	if got == nil {
		t.Fatal("Load returned nil overrides; want zero-value struct")
	}
	if *got != (ConfigOverrides{}) {
		t.Errorf("overrides = %+v; want zero value", *got)
	}
}
func TestEncryptDecryptRoundTrip(t *testing.T) {
	enc := NewEncrypter([]byte("any-length master key works!"))
	plain := []byte("sup3r-s3cret-gmail-app-password")

	ct, nonce, err := enc.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if len(nonce) != gcmNonceSize {
		t.Errorf("nonce = %d bytes; want %d", len(nonce), gcmNonceSize)
	}
	got, err := enc.Decrypt(ct, nonce)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plain) {
		t.Errorf("round trip = %q; want %q", got, plain)
	}

	// Failure paths: wrong key and tampered ciphertext must both error.
	other := NewEncrypter([]byte("different-master-key"))
	if _, err := other.Decrypt(ct, nonce); err == nil {
		t.Error("Decrypt with wrong key should error")
	}
	tampered := append([]byte(nil), ct...)
	tampered[0] ^= 0xFF
	if _, err := enc.Decrypt(tampered, nonce); err == nil {
		t.Error("Decrypt of tampered ciphertext should error")
	}
	if _, err := enc.Decrypt(ct, []byte("short")); err == nil {
		t.Error("Decrypt with wrong nonce length should error")
	}
}
func TestGmailPasswordEncryptedInDB(t *testing.T) {
	ctx := context.Background()
	s, raw := newTestStore(t)

	if err := s.Save(ctx, &ConfigOverrides{
		FirstName:        "Ada",
		Email:            "ada@example.com",
		GmailAppPassword: "mysecret",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Read the raw column exactly as stored — it must be non-empty and must
	// not hold the plaintext.
	var stored []byte
	if err := raw.QueryRow("SELECT gmail_password_encrypted FROM user_settings WHERE id = 1").Scan(&stored); err != nil {
		t.Fatalf("query raw column: %v", err)
	}
	if len(stored) == 0 {
		t.Fatal("gmail_password_encrypted is empty; expected ciphertext")
	}
	if string(stored) == "mysecret" {
		t.Error("gmail_password_encrypted holds the plaintext password in the DB")
	}

	// The encryption key row must have been bootstrapped and, when loaded
	// back through the Store, the password must still decrypt correctly.
	var n int
	if err := raw.QueryRow("SELECT COUNT(1) FROM encryption_key").Scan(&n); err != nil || n != 1 {
		t.Errorf("encryption_key rows = %d, err %v; want 1 (bootstrapped)", n, err)
	}
	got, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.GmailAppPassword != "mysecret" {
		t.Errorf("password after reload = %q; want %q", got.GmailAppPassword, "mysecret")
	}
}
