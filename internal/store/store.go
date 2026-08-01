package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".nexus")
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

// Exists returns true if this URL has already been applied to.
func (s *Store) Exists(url string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM applications WHERE url = ?`, url).Scan(&count)
	return count > 0, err
}

// Insert records a new application. Silently ignores duplicate URLs.
func (s *Store) Insert(app Application) error {
	postedAt := app.PostedAt
	if postedAt.IsZero() {
		postedAt = app.AppliedAt
	}
	remote := 0
	if app.Remote {
		remote = 1
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO applications
		 (provider, company, role, url, status, reason, applied_at, location, remote, posted_at, description, fit_score, fit_summary, outcome, outcome_at, approved)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		app.Provider, app.Company, app.Role, app.URL,
		string(app.Status), app.Reason, app.AppliedAt.UTC(),
		app.Location, remote, postedAt.UTC(), app.Description,
		app.FitScore, app.FitSummary,
		string(app.Outcome), app.OutcomeAt.UTC(),
		app.Approved,
	)
	return err
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
		`UPDATE applications SET outcome = ?, outcome_at = ? WHERE id = ?`,
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
		`UPDATE applications SET approved = ? WHERE id = ?`,
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
		`UPDATE applications SET status = ?, reason = ?, applied_at = ? WHERE id = ?`,
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
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query(
		`SELECT id, provider, company, role, url, status, reason, applied_at,
		        location, remote, posted_at, description, fit_score, fit_summary,
		        outcome, outcome_at, approved
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
	err := s.db.QueryRow(`SELECT id FROM applications WHERE url = ?`, url).Scan(&id)
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
		`UPDATE applications SET description = ?, fit_score = ?, fit_summary = ? WHERE url = ?`,
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
		        outcome, outcome_at, approved
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
		        outcome, outcome_at, approved
		 FROM applications
		 WHERE lower(trim(company)) = lower(trim(?))
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
		var appliedAt, postedAt, outcomeAt string
		var remote, approved int
		var outcome string
		if err := rows.Scan(
			&a.ID, &a.Provider, &a.Company, &a.Role,
			&a.URL, &a.Status, &a.Reason, &appliedAt,
			&a.Location, &remote, &postedAt, &a.Description,
			&a.FitScore, &a.FitSummary, &outcome, &outcomeAt, &approved,
		); err != nil {
			return nil, err
		}
		a.AppliedAt, _ = time.Parse(time.RFC3339, appliedAt)
		a.PostedAt, _ = time.Parse(time.RFC3339, postedAt)
		a.OutcomeAt, _ = time.Parse(time.RFC3339, outcomeAt)
		a.Outcome = Outcome(outcome)
		a.Remote = remote == 1
		a.Approved = approved == 1
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
func (s *Store) CountAppliedSince(since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM applications WHERE status = ? AND applied_at >= ?`,
		string(StatusApplied), since.UTC().Format(time.RFC3339),
	).Scan(&n)
	return n, err
}
