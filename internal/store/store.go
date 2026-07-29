package store

import (
	"database/sql"
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
	fit_summary TEXT    NOT NULL DEFAULT ''
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
	// Best-effort migration: add new columns to existing databases.
	db.Exec(contactsMigrateEmailType) // ignore error — column may already exist
	for _, col := range []string{
		`ALTER TABLE applications ADD COLUMN location    TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE applications ADD COLUMN remote      INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE applications ADD COLUMN posted_at   DATETIME NOT NULL DEFAULT '0001-01-01T00:00:00Z'`,
		`ALTER TABLE applications ADD COLUMN description TEXT    NOT NULL DEFAULT ''`,
		`ALTER TABLE applications ADD COLUMN fit_score   INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE applications ADD COLUMN fit_summary TEXT    NOT NULL DEFAULT ''`,
	} {
		db.Exec(col) // ignore errors — column already exists
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

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
		 (provider, company, role, url, status, reason, applied_at, location, remote, posted_at, description, fit_score, fit_summary)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		app.Provider, app.Company, app.Role, app.URL,
		string(app.Status), app.Reason, app.AppliedAt.UTC(),
		app.Location, remote, postedAt.UTC(), app.Description,
		app.FitScore, app.FitSummary,
	)
	return err
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
		        location, remote, posted_at, description, fit_score, fit_summary
		 FROM applications ORDER BY applied_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []Application
	for rows.Next() {
		var a Application
		var appliedAt, postedAt string
		var remote int
		if err := rows.Scan(
			&a.ID, &a.Provider, &a.Company, &a.Role,
			&a.URL, &a.Status, &a.Reason, &appliedAt,
			&a.Location, &remote, &postedAt, &a.Description,
			&a.FitScore, &a.FitSummary,
		); err != nil {
			return nil, err
		}
		a.AppliedAt, _ = time.Parse(time.RFC3339, appliedAt)
		a.PostedAt, _ = time.Parse(time.RFC3339, postedAt)
		a.Remote = remote == 1
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
