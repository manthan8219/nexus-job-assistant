package settings

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib" // register the "pgx" database/sql driver
)

// ErrNotFound reports that no user_settings row exists for the single id=1
// row. Load does not return it (missing row means empty overrides), but it is
// exported so callers that must distinguish "absent" from "zero" can branch.
var ErrNotFound = errors.New("settings: no row for user")

// Field names each persisted, user-editable setting. Constants mirror the
// underlying column names and are stable identifiers for UI bindings,
// validation, and error messages.
type Field string

const (
	FieldFirstName         Field = "first_name"
	FieldLastName          Field = "last_name"
	FieldEmail             Field = "email"
	FieldGmailPassword     Field = "gmail_password"
	FieldOutreachConsent   Field = "outreach_consent"
	FieldInboxScanMinutes  Field = "inbox_scan_minutes"
	FieldCity              Field = "city"
	FieldPhone             Field = "phone"
	FieldLinkedInID        Field = "linkedin_id"
	FieldTargetJobTitles   Field = "target_job_titles"
	FieldWorkType          Field = "work_type"
	FieldTargetLocations   Field = "target_locations"
	FieldCurrency          Field = "currency"
	FieldMinSalary         Field = "min_salary"
	FieldApplyConsent      Field = "apply_consent"
	FieldMaxAppsPerRun     Field = "max_apps_per_run"
	FieldMaxAppsPerDay     Field = "max_apps_per_day"
	FieldApplyDelaySec     Field = "apply_delay_sec"
	FieldMinFitScore       Field = "min_fit_score"
	FieldCompanyBlocklist  Field = "company_blocklist"
	FieldNoticePeriodDays  Field = "notice_period_days"
	FieldOfficeDaysPerWeek Field = "office_days_per_week"
	FieldCoverLetterMode   Field = "cover_letter_mode"
	FieldWorkAuth          Field = "work_auth"
	FieldResumePath        Field = "resume_path"
)

// userSettingsDDL is the Postgres schema Nexus uses against Supabase. Tests
// create an equivalent SQLite table (TEXT instead of BYTEA, no TIMESTAMPTZ)
// because this DDL is driver-specific.
const userSettingsDDL = `
CREATE TABLE IF NOT EXISTS user_settings (
    id INT PRIMARY KEY DEFAULT 1,
    first_name TEXT DEFAULT '',
    last_name TEXT DEFAULT '',
    email TEXT DEFAULT '',
    gmail_password_encrypted BYTEA DEFAULT '',
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
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS encryption_key (
    id INT PRIMARY KEY DEFAULT 1,
    encrypted_key BYTEA NOT NULL,
    nonce BYTEA NOT NULL
);
`

// userSettingsColumns is the shared column list (without id) used by both the
// Load SELECT and the Save INSERT/UPDATE, so the two must never drift.
const userSettingsColumns = `
	first_name, last_name, email, gmail_password_encrypted, outreach_consent,
	inbox_scan_minutes, city, phone, linkedin_id, target_job_titles, work_type,
	target_locations, currency, min_salary, apply_consent, max_apps_per_run,
	max_apps_per_day, apply_delay_sec, min_fit_score, company_blocklist,
	notice_period_days, office_days_per_week, cover_letter_mode, work_auth, resume_path
`

// Store persists ConfigOverrides in a user_settings row (id=1) and keeps the
// AES key that seals the Gmail password in an encryption_key row. The in-DB
// key is itself wrapped by the master key passed to NewStore, so the env-var
// master key decrypts the stored key, which in turn decrypts the password —
// double-wrapped. It drives any database/sql backend: pgx (Supabase/Postgres)
// in production and modernc.org/sqlite in hermetic tests.
type Store struct {
	db     *sql.DB
	master *Encrypter // wraps the env-var master key

	mu      sync.Mutex
	dataKey *[32]byte // the AES key that seals the Gmail password (lazy)
}

// NewStore builds a Store over db. masterKey is the env-var key (any length)
// that decrypts the stored encryption_key row. It does not touch the
// database; schema creation and key bootstrap happen lazily via EnsureSchema,
// Load, and Save.
func NewStore(db *sql.DB, masterKey []byte) *Store {
	return &Store{
		db:     db,
		master: NewEncrypter(masterKey),
	}
}

// EnsureSchema creates the user_settings and encryption_key tables if they do
// not already exist. The DDL is Postgres-specific; callers running against a
// non-Postgres backend (e.g. SQLite in tests) must create equivalent tables
// themselves.
func (s *Store) EnsureSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, userSettingsDDL); err != nil {
		return fmt.Errorf("settings: ensure schema: %w", err)
	}
	return nil
}

// Load reads the id=1 user_settings row. When no row exists it returns a
// zero-value ConfigOverrides and nil error ("absent" means "no overrides").
// If a Gmail password is stored it is decrypted and returned in plaintext;
// a missing encryption_key row is not an error — it simply means no password
// has been saved yet.
func (s *Store) Load(ctx context.Context) (*ConfigOverrides, error) {
	over := &ConfigOverrides{}
	row := s.db.QueryRowContext(ctx,
		`SELECT `+userSettingsColumns+` FROM user_settings WHERE id = 1`)

	var gmail []byte
	err := row.Scan(
		&over.FirstName, &over.LastName, &over.Email, &gmail, &over.OutreachConsent,
		&over.InboxScanMinutes, &over.City, &over.Phone, &over.LinkedInID,
		&over.TargetJobTitles, &over.WorkType, &over.TargetLocations, &over.Currency,
		&over.MinSalary, &over.ApplyConsent, &over.MaxAppsPerRun, &over.MaxAppsPerDay,
		&over.ApplyDelaySec, &over.MinFitScore, &over.CompanyBlocklist,
		&over.NoticePeriodDays, &over.OfficeDaysPerWeek, &over.CoverLetterMode,
		&over.WorkAuth, &over.ResumePath,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return over, nil
		}
		return nil, fmt.Errorf("settings: load row: %w", err)
	}
	if len(gmail) == 0 {
		return over, nil
	}

	dataKey, err := s.ensureDataKey(ctx)
	if err != nil {
		return nil, err
	}
	plain, err := decryptGmail(gmail, dataKey)
	if err != nil {
		return nil, err
	}
	over.GmailAppPassword = plain
	return over, nil
}

// Save upserts the id=1 user_settings row from over. The Gmail password is
// encrypted with the in-DB AES key (bootstrap-created on first use) before it
// is written, so the raw column never holds the plaintext. An empty password
// stores an empty value.
func (s *Store) Save(ctx context.Context, over *ConfigOverrides) error {
	dataKey, err := s.ensureDataKey(ctx)
	if err != nil {
		return err
	}

	var gmail []byte
	if over.GmailAppPassword != "" {
		gmail, err = encryptGmail(over.GmailAppPassword, dataKey)
		if err != nil {
			return err
		}
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO user_settings (
		id, `+userSettingsColumns+`
	) VALUES (1, `+placeholders(25)+`)
	ON CONFLICT (id) DO UPDATE SET
		first_name = EXCLUDED.first_name,
		last_name = EXCLUDED.last_name,
		email = EXCLUDED.email,
		gmail_password_encrypted = EXCLUDED.gmail_password_encrypted,
		outreach_consent = EXCLUDED.outreach_consent,
		inbox_scan_minutes = EXCLUDED.inbox_scan_minutes,
		city = EXCLUDED.city,
		phone = EXCLUDED.phone,
		linkedin_id = EXCLUDED.linkedin_id,
		target_job_titles = EXCLUDED.target_job_titles,
		work_type = EXCLUDED.work_type,
		target_locations = EXCLUDED.target_locations,
		currency = EXCLUDED.currency,
		min_salary = EXCLUDED.min_salary,
		apply_consent = EXCLUDED.apply_consent,
		max_apps_per_run = EXCLUDED.max_apps_per_run,
		max_apps_per_day = EXCLUDED.max_apps_per_day,
		apply_delay_sec = EXCLUDED.apply_delay_sec,
		min_fit_score = EXCLUDED.min_fit_score,
		company_blocklist = EXCLUDED.company_blocklist,
		notice_period_days = EXCLUDED.notice_period_days,
		office_days_per_week = EXCLUDED.office_days_per_week,
		cover_letter_mode = EXCLUDED.cover_letter_mode,
		work_auth = EXCLUDED.work_auth,
		resume_path = EXCLUDED.resume_path,
		updated_at = CURRENT_TIMESTAMP`,
		over.FirstName, over.LastName, over.Email, gmail, over.OutreachConsent,
		over.InboxScanMinutes, over.City, over.Phone, over.LinkedInID,
		over.TargetJobTitles, over.WorkType, over.TargetLocations, over.Currency,
		over.MinSalary, over.ApplyConsent, over.MaxAppsPerRun, over.MaxAppsPerDay,
		over.ApplyDelaySec, over.MinFitScore, over.CompanyBlocklist,
		over.NoticePeriodDays, over.OfficeDaysPerWeek, over.CoverLetterMode,
		over.WorkAuth, over.ResumePath,
	)
	if err != nil {
		return fmt.Errorf("settings: save row: %w", err)
	}
	return nil
}

// ensureDataKey returns the 32-byte AES key that seals the Gmail password,
// loading and unwrapping it from the encryption_key row if present, or
// generating, wrapping with the master key, and persisting it on first use.
// The result is cached on the Store for the process lifetime.
func (s *Store) ensureDataKey(ctx context.Context) (*[32]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dataKey != nil {
		return s.dataKey, nil
	}

	var encKey, nonce []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT encrypted_key, nonce FROM encryption_key WHERE id = 1`).Scan(&encKey, &nonce)
	switch {
	case err == nil:
		k, err := s.master.Decrypt(encKey, nonce)
		if err != nil {
			return nil, err
		}
		var key [32]byte
		copy(key[:], k)
		s.dataKey = &key
	case errors.Is(err, sql.ErrNoRows):
		var key [32]byte
		if _, err := rand.Read(key[:]); err != nil {
			return nil, fmt.Errorf("settings: generate data key: %w", err)
		}
		enc, nonce, err := s.master.Encrypt(key[:])
		if err != nil {
			return nil, err
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO encryption_key (id, encrypted_key, nonce) VALUES (1, $1, $2)`,
			enc, nonce); err != nil {
			return nil, fmt.Errorf("settings: store data key: %w", err)
		}
		s.dataKey = &key
	default:
		return nil, fmt.Errorf("settings: read encryption key: %w", err)
	}
	return s.dataKey, nil
}

// placeholders renders n non-padded "$1..$n" placeholders for a parameterized
// INSERT. Both pgx and modernc.org/sqlite accept the plain "$N" positional
// form; zero-padding ("$01") must be avoided because sqlite then parses it as
// a named parameter.
func placeholders(n int) string {
	b := make([]byte, 0, n*3)
	for i := 1; i <= n; i++ {
		if i > 1 {
			b = append(b, ',')
		}
		b = append(b, '$')
		b = strconv.AppendInt(b, int64(i), 10)
	}
	return string(b)
}

// encryptGmail seals the plaintext password with dataKey by appending the
// 12-byte nonce to the ciphertext. The stored form is {ciphertext}{nonce} so
// both are always retrieved together in one column.
func encryptGmail(plaintext string, dataKey *[32]byte) ([]byte, error) {
	enc := &Encrypter{key: *dataKey}
	ct, nonce, err := enc.Encrypt([]byte(plaintext))
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(ct)+len(nonce))
	copy(out, ct)
	copy(out[len(ct):], nonce)
	return out, nil
}

// decryptGmail reverses encryptGmail, splitting {ciphertext}{nonce} apart.
func decryptGmail(stored []byte, dataKey *[32]byte) (string, error) {
	enc := &Encrypter{key: *dataKey}
	if len(stored) < gcmNonceSize {
		return "", ErrSealed
	}
	ct := stored[:len(stored)-gcmNonceSize]
	nonce := stored[len(stored)-gcmNonceSize:]
	plain, err := enc.Decrypt(ct, nonce)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
