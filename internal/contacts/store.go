// Package contacts provides a local SQLite store for saved HR/recruiter
// contacts discovered via OSINT (~/.nexus/contacts.db by default).
package contacts

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/osint"
	_ "modernc.org/sqlite"
)

// DB is the local saved-contacts database.
type DB struct {
	db *sql.DB
}

// defaultDBPath returns ~/.nexus/contacts.db, creating the directory if needed.
func defaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".nexus")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "contacts.db"), nil
}

// OpenDefault opens ~/.nexus/contacts.db (creates dir + schema).
func OpenDefault() (*DB, error) {
	path, err := defaultDBPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

// Open opens (or creates) a SQLite saved-contacts DB at path.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	s := &DB{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying connection. Safe on a nil receiver.
func (s *DB) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *DB) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS saved_contacts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  company TEXT NOT NULL DEFAULT '',
  domain TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL DEFAULT '',
  email_type TEXT NOT NULL DEFAULT '',
  linked_in TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  confidence INTEGER NOT NULL DEFAULT 0,
  found_at TEXT NOT NULL,
  notes TEXT NOT NULL DEFAULT '',
  UNIQUE(company, email, linked_in)
);
CREATE INDEX IF NOT EXISTS idx_saved_contacts_company ON saved_contacts(company);
CREATE INDEX IF NOT EXISTS idx_saved_contacts_email ON saved_contacts(email);
`)
	return err
}

const selectCols = `id, company, domain, name, title, email, email_type, linked_in, source, confidence, found_at, notes`

// Count returns the total number of saved contacts.
func (s *DB) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM saved_contacts`).Scan(&n)
	return n, err
}

// List returns all saved contacts, optionally filtered by a free-text query
// across company, name, email and title. Results are newest-first.
func (s *DB) List(query string) ([]osint.Contact, error) {
	q := `SELECT ` + selectCols + ` FROM saved_contacts`
	args := []any{}
	if q := strings.TrimSpace(query); q != "" {
		like := "%" + q + "%"
		q += ` WHERE company LIKE ? OR name LIKE ? OR email LIKE ? OR title LIKE ?`
		args = append(args, like, like, like, like)
	}
	q += ` ORDER BY id DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []osint.Contact
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

// scanContact maps one saved_contacts row into an osint.Contact.
func scanContact(s rowScanner) (osint.Contact, error) {
	var c osint.Contact
	var foundAt string
	err := s.Scan(&c.ID, &c.Company, &c.Domain, &c.Name, &c.Title,
		&c.Email, &c.EmailType, &c.LinkedIn, &c.Source, &c.Confidence,
		&foundAt, &c.Notes)
	if err != nil {
		return c, err
	}
	if t, perr := time.Parse(time.RFC3339, foundAt); perr == nil {
		c.FoundAt = t
	}
	return c, nil
}

// Save upserts a contact (unique by company+email+linked_in) and returns the
// stored row with its ID and a non-zero foundAt timestamp.
func (s *DB) Save(c osint.Contact) (osint.Contact, error) {
	if c.FoundAt.IsZero() {
		c.FoundAt = time.Now().UTC()
	}
	foundAt := c.FoundAt.UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
INSERT INTO saved_contacts
  (company, domain, name, title, email, email_type, linked_in, source, confidence, found_at, notes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(company, email, linked_in) DO UPDATE SET
  domain=excluded.domain, name=excluded.name, title=excluded.title,
  email_type=excluded.email_type, linked_in=excluded.linked_in,
  source=excluded.source, confidence=excluded.confidence,
  found_at=excluded.found_at, notes=excluded.notes`,
		c.Company, c.Domain, c.Name, c.Title, c.Email, c.EmailType,
		c.LinkedIn, c.Source, c.Confidence, foundAt, c.Notes)
	if err != nil {
		return c, err
	}
	if c.ID == 0 {
		err = s.db.QueryRow(
			`SELECT id FROM saved_contacts WHERE company = ? AND email = ? AND linked_in = ?`,
			c.Company, c.Email, c.LinkedIn).Scan(&c.ID)
		if err != nil {
			return c, err
		}
	}
	c.FoundAt = c.FoundAt.UTC()
	return c, nil
}

// Delete removes a saved contact by id.
func (s *DB) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM saved_contacts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("contact %d not found", id)
	}
	return nil
}
