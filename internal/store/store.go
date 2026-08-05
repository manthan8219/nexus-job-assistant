package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // register the pgx driver
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/nexusdir"
	"github.com/manthan8219/nexus-job-assistant/internal/pgutil"
	_ "modernc.org/sqlite"
)

const contactsDDL = `
CREATE TABLE IF NOT EXISTS contacts (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	company     TEXT    NOT NULL DEFAULT '',
	domain      TEXT    NOT NULL DEFAULT '',
	name        TEXT    NOT NULL DEFAULT '',
	title       TEXT    NOT NULL DEFAULT '',
	email       TEXT    NOT NULL DEFAULT '',
	email_type  TEXT    NOT NULL DEFAULT '',
	linkedin    TEXT    NOT NULL DEFAULT '',
	source      TEXT    NOT NULL DEFAULT '',
	confidence  INTEGER NOT NULL DEFAULT 0,
	found_at    DATETIME NOT NULL,
	notes       TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_contacts_company ON contacts(company);
CREATE UNIQUE INDEX IF NOT EXISTS idx_contacts_email_company ON contacts(email, company);
`

const contactsMigrateEmailType = `ALTER TABLE contacts ADD COLUMN email_type TEXT NOT NULL DEFAULT ''`

const ddl = `
CREATE TABLE IF NOT EXISTS applications (
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
	outcome_at  DATETIME NOT NULL DEFAULT '0001-01-01T00:00:00Z',
	approved    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_applied_at ON applications(applied_at);
CREATE INDEX IF NOT EXISTS idx_status     ON applications(status);
`

type Store struct {
	db *sql.DB
}

func Open() (*Store, error) {
	dir := nexusdir.Home()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return openPath(filepath.Join(dir, "applications.db"))
}

// OpenAt opens the store at a specific database path (used by tests and tools
// that need a hermetic store outside ~/.nexus).
func OpenAt(path string) (*Store, error) {
	return openPath(path)
}

func openPath(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite is single-writer: serializing on one connection prevents
	// SQLITE_BUSY when the engine's background scoring writes concurrently
	// with the apply pipeline (dropped inserts otherwise).
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(ddl); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(contactsDDL); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(outreachLogDDL); err != nil {
		db.Close()
		return nil, err
	}
	// Best-effort migration: add new columns to existing databases.
	db.Exec(contactsMigrateEmailType) // ignore error — column may already exist
	for _, col := range []string{
		`ALTER TABLE applications ADD COLUMN location    TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE applications ADD COLUMN remote      INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE applications ADD COLUMN posted_at   DATETIME NOT NULL DEFAULT '0001-01-01T00:00:00Z'`,
		`ALTER TABLE applications ADD COLUMN description TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE applications ADD COLUMN fit_score   INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE applications ADD COLUMN fit_summary TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE applications ADD COLUMN outcome     TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE applications ADD COLUMN outcome_at  DATETIME NOT NULL DEFAULT '0001-01-01T00:00:00Z'`,
		`ALTER TABLE applications ADD COLUMN approved    INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE applications ADD COLUMN submitted_payload TEXT NOT NULL DEFAULT ''`,
	} {
		db.Exec(col) // ignore errors — column already exists
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// OpenPath opens (or creates) a SQLite application store at an explicit path,
// running the same schema setup + migrations as Open. Use it in tests with a
// t.TempDir() file so the store is hermetic (no ~/.nexus writes, no network).
func OpenPath(path string) (*Store, error) { return openPath(path) }

// pgDDL ensures the store's tables exist on a Postgres backend (idempotent).
// Mirrors the SQLite ddl/contactsDDL/outreachLogDDL but with Postgres types.
const pgDDL = `
CREATE TABLE IF NOT EXISTS applications (
	id         BIGSERIAL PRIMARY KEY,
	provider   TEXT NOT NULL,
	company    TEXT NOT NULL,
	role       TEXT NOT NULL,
	url        TEXT NOT NULL UNIQUE,
	status     TEXT NOT NULL DEFAULT 'applied',
	reason     TEXT NOT NULL DEFAULT '',
	applied_at TIMESTAMPTZ NOT NULL,
	location   TEXT NOT NULL DEFAULT '',
	remote     BOOLEAN NOT NULL DEFAULT FALSE,
	posted_at  TIMESTAMPTZ NOT NULL DEFAULT '0001-01-01T00:00:00Z',
	description TEXT NOT NULL DEFAULT '',
	fit_score  INTEGER NOT NULL DEFAULT 0,
	fit_summary TEXT NOT NULL DEFAULT '',
	outcome    TEXT NOT NULL DEFAULT '',
	outcome_at TIMESTAMPTZ NOT NULL DEFAULT '0001-01-01T00:00:00Z',
	approved   BOOLEAN NOT NULL DEFAULT FALSE,
	submitted_payload TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_applications_applied_at ON applications(applied_at);
CREATE INDEX IF NOT EXISTS idx_applications_status ON applications(status);
CREATE TABLE IF NOT EXISTS contacts (
	id         BIGSERIAL PRIMARY KEY,
	company    TEXT NOT NULL DEFAULT '',
	domain     TEXT NOT NULL DEFAULT '',
	name       TEXT NOT NULL DEFAULT '',
	title      TEXT NOT NULL DEFAULT '',
	email      TEXT NOT NULL DEFAULT '',
	email_type TEXT NOT NULL DEFAULT '',
	linkedin   TEXT NOT NULL DEFAULT '',
	source     TEXT NOT NULL DEFAULT '',
	confidence INTEGER NOT NULL DEFAULT 0,
	found_at   TIMESTAMPTZ NOT NULL,
	notes      TEXT NOT NULL DEFAULT '',
	UNIQUE (email, company)
);
CREATE INDEX IF NOT EXISTS idx_contacts_company ON contacts(company);
CREATE TABLE IF NOT EXISTS outreach_log (
	id             BIGSERIAL PRIMARY KEY,
	channel        TEXT NOT NULL DEFAULT '',
	job_url        TEXT NOT NULL DEFAULT '',
	company        TEXT NOT NULL DEFAULT '',
	role           TEXT NOT NULL DEFAULT '',
	contact_name   TEXT NOT NULL DEFAULT '',
	contact_email  TEXT NOT NULL DEFAULT '',
	contact_source TEXT NOT NULL DEFAULT '',
	subject        TEXT NOT NULL DEFAULT '',
	body           TEXT NOT NULL DEFAULT '',
	status         TEXT NOT NULL DEFAULT '',
	error          TEXT NOT NULL DEFAULT '',
	review_score   INTEGER NOT NULL DEFAULT 0,
	attempts       INTEGER NOT NULL DEFAULT 0,
	created_at     TIMESTAMPTZ NOT NULL,
	sent_at        TIMESTAMPTZ NOT NULL DEFAULT '0001-01-01T00:00:00Z'
);
CREATE INDEX IF NOT EXISTS idx_outreach_log_sent_at ON outreach_log(sent_at);
CREATE INDEX IF NOT EXISTS idx_outreach_log_company ON outreach_log(company);
`

// OpenPG opens (or creates) the store against a Postgres database - the
// Supabase managed backend. The schema is ensured idempotently at open.
func OpenPG(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, pgutil.WrapConnectError(err, dsn)
	}
	db.SetMaxOpenConns(5)
	if _, err := db.Exec(pgDDL); err != nil {
		db.Close()
		return nil, pgutil.WrapConnectError(err, dsn)
	}
	return &Store{db: db}, nil
}

// OpenFromConfig picks the storage backend from config: managed Postgres
// (Supabase) when DatabaseURL is set, otherwise the local SQLite store.
func OpenFromConfig(cfg *config.Config) (*Store, error) {
	if cfg != nil && strings.TrimSpace(cfg.DatabaseURL) != "" {
		return OpenPG(cfg.DatabaseURL)
	}
	return Open()
}

// Exists returns true if this URL has already been applied to.
func (s *Store) Exists(url string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM applications WHERE url = $1`, url).Scan(&count)
	return count > 0, err
}

// Insert records a new application. Silently ignores duplicate URLs.
func (s *Store) Insert(app Application) error {
	postedAt := app.PostedAt
	if postedAt.IsZero() {
		postedAt = app.AppliedAt
	}
	_, err := s.db.Exec(
		`INSERT INTO applications
		 (provider, company, role, url, status, reason, applied_at, location, remote, posted_at, description, fit_score, fit_summary, outcome, outcome_at, approved, submitted_payload)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 ON CONFLICT (url) DO NOTHING`,
		app.Provider, app.Company, app.Role, app.URL,
		string(app.Status), app.Reason, app.AppliedAt.UTC(),
		app.Location, app.Remote, postedAt.UTC(), app.Description,
		app.FitScore, app.FitSummary,
		string(app.Outcome), app.OutcomeAt.UTC(),
		app.Approved, app.SubmittedPayload,
	)
	return err
}

// SetSubmittedPayload records the JSON audit of the exact submission for an
// application (KAN-33). Fails open at the caller — never blocks an apply.
func (s *Store) SetSubmittedPayload(id int64, payloadJSON string) error {
	res, err := s.db.Exec(
		`UPDATE applications SET submitted_payload = $1 WHERE id = $2`,
		payloadJSON, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("store: no application with id %d", id)
	}
	return nil
}

// SetSubmittedPayloadByURL records the submission audit for the application
// with the given URL (used by the run loop, which inserts before scoring).
func (s *Store) SetSubmittedPayloadByURL(url, payloadJSON string) error {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM applications WHERE url = $1`, url).Scan(&id)
	if err != nil {
		return err
	}
	return s.SetSubmittedPayload(id, payloadJSON)
}

// SetOutcome records the post-apply outcome for one application.
// OutcomeNone clears the outcome (back to "waiting").
func (s *Store) SetOutcome(id int64, outcome Outcome) error {
	if !ValidOutcome(outcome) {
		return fmt.Errorf("store: invalid outcome %q", outcome)
	}
	at := time.Time{}
	if outcome != OutcomeNone {
		at = time.Now()
	}
	res, err := s.db.Exec(
		`UPDATE applications SET outcome = $1, outcome_at = $2 WHERE id = $3`,
		string(outcome), at.UTC(), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("store: no application with id %d", id)
	}
	return nil
}

// SetApproved marks (or unmarks) an application for a real apply.
func (s *Store) SetApproved(id int64, approved bool) error {
	res, err := s.db.Exec(
		`UPDATE applications SET approved = $1 WHERE id = $2`,
		approved, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("store: no application with id %d", id)
	}
	return nil
}

// SetStatus records a new apply status + reason for an application and stamps
// applied_at (used by the apply-selected flow after a real submission).
func (s *Store) SetStatus(id int64, status Status, reason string) error {
	res, err := s.db.Exec(
		`UPDATE applications SET status = $1, reason = $2, applied_at = $3 WHERE id = $4`,
		string(status), reason, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("store: no application with id %d", id)
	}
	return nil
}

// GetByIDs returns the applications with the given ids, in the same order.
func (s *Store) GetByIDs(ids []int64) ([]Application, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := make([]string, len(ids))
	for i := range ph {
		ph[i] = fmt.Sprintf("$%d", i+1)
	}
	placeholders := strings.Join(ph, ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query(
		`SELECT id, provider, company, role, url, status, reason, applied_at,
		        location, remote, posted_at, description, fit_score, fit_summary,
		        outcome, outcome_at, approved, submitted_payload
		 FROM applications WHERE id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApplications(rows)
}

// SetOutcomeByURL records the outcome for the application with the given URL.
// Returns false when no application matches (e.g. a reply for an unrecorded job).
func (s *Store) SetOutcomeByURL(url string, outcome Outcome) (bool, error) {
	if !ValidOutcome(outcome) {
		return false, fmt.Errorf("store: invalid outcome %q", outcome)
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM applications WHERE url = $1`, url).Scan(&id)
	if err != nil {
		return false, nil // no rows → not our application; treat as no-op
	}
	return true, s.SetOutcome(id, outcome)
}

// OutcomeStats counts applications per non-empty outcome (the funnel:
// replied → interview → offer, plus rejected/ghosted).
func (s *Store) OutcomeStats() (map[Outcome]int, error) {
	rows, err := s.db.Query(
		`SELECT outcome, COUNT(1) FROM applications WHERE outcome != '' GROUP BY outcome`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[Outcome]int{}
	for rows.Next() {
		var outcome string
		var n int
		if err := rows.Scan(&outcome, &n); err != nil {
			return nil, err
		}
		counts[Outcome(outcome)] = n
	}
	return counts, rows.Err()
}

// UpdateDescriptionFit patches description + optional fit fields for an existing URL.
func (s *Store) UpdateDescriptionFit(jobURL, description string, fitScore int, fitSummary string) error {
	_, err := s.db.Exec(
		`UPDATE applications SET description = $1, fit_score = $2, fit_summary = $3 WHERE url = $4`,
		description, fitScore, fitSummary, jobURL,
	)
	return err
}

// ListMissingDescription returns apps with empty descriptions (newest first).
func (s *Store) ListMissingDescription() ([]Application, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var out []Application
	for _, a := range all {
		if strings.TrimSpace(a.Description) == "" {
			out = append(out, a)
		}
	}
	return out, nil
}

// List returns all applications ordered by most recent first.
func (s *Store) List() ([]Application, error) {
	rows, err := s.db.Query(
		`SELECT id, provider, company, role, url, status, reason, applied_at,
		        location, remote, posted_at, description, fit_score, fit_summary,
		        outcome, outcome_at, approved, submitted_payload
		 FROM applications ORDER BY applied_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApplications(rows)
}

// CompanyKey normalizes a company name so scraped jobs can be matched back to
// a company regardless of case or surrounding whitespace.
func CompanyKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// CompanyJobCounts returns how many scraped jobs are recorded per company,
// keyed by CompanyKey (lower-cased, trimmed company name).
func (s *Store) CompanyJobCounts() (map[string]int, error) {
	rows, err := s.db.Query(
		`SELECT lower(trim(company)), COUNT(1) FROM applications
		 WHERE trim(company) != '' GROUP BY lower(trim(company))`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var key string
		var n int
		if err := rows.Scan(&key, &n); err != nil {
			return nil, err
		}
		counts[key] += n
	}
	return counts, rows.Err()
}

// ListByCompany returns every scraped job recorded for a company
// (case-insensitive name match), newest first.
func (s *Store) ListByCompany(company string) ([]Application, error) {
	rows, err := s.db.Query(
		`SELECT id, provider, company, role, url, status, reason, applied_at,
		        location, remote, posted_at, description, fit_score, fit_summary,
		        outcome, outcome_at, approved, submitted_payload
		 FROM applications
		 WHERE lower(trim(company)) = lower(trim($1))
		 ORDER BY applied_at DESC`,
		company,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanApplications(rows)
}

func scanApplications(rows *sql.Rows) ([]Application, error) {
	var apps []Application
	for rows.Next() {
		var a Application
		var appliedAt, postedAt, outcomeAt time.Time
		var remote, approved scanBool
		var outcome string
		if err := rows.Scan(
			&a.ID, &a.Provider, &a.Company, &a.Role,
			&a.URL, &a.Status, &a.Reason, &appliedAt,
			&a.Location, &remote, &postedAt, &a.Description,
			&a.FitScore, &a.FitSummary, &outcome, &outcomeAt, &approved,
			&a.SubmittedPayload,
		); err != nil {
			return nil, err
		}
		a.AppliedAt = appliedAt
		a.PostedAt = postedAt
		a.OutcomeAt = outcomeAt
		a.Outcome = Outcome(outcome)
		a.Remote = remote.v
		a.Approved = approved.v
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

// Stats returns total applied, skipped, failed counts.
func (s *Store) Stats() (applied, skipped, failed int, err error) {
	rows, err := s.db.Query(
		`SELECT status, COUNT(1) FROM applications GROUP BY status`,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err = rows.Scan(&status, &count); err != nil {
			return
		}
		switch Status(status) {
		case StatusApplied:
			applied = count
		case StatusSkipped:
			skipped = count
		case StatusFailed:
			failed = count
		}
	}
	return
}

// CountAppliedSince returns how many successful applies happened at/after since (UTC).
// CountAppliedSince counts applications recorded as applied at or after the
// given time. applied_at is stored by the driver as a native time.Time that
// SQLite's datetime() cannot parse, so the filter runs in Go where the value
// round-trips correctly.
func (s *Store) CountAppliedSince(since time.Time) (int, error) {
	rows, err := s.db.Query(`SELECT applied_at FROM applications WHERE status = ?`, string(StatusApplied))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			return 0, err
		}
		if !at.Before(since) {
			n++
		}
	}
	return n, rows.Err()
}
