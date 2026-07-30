package store

import (
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/osint"
)

// SaveContact inserts a contact. Duplicate emails per company are ignored.
func (s *Store) SaveContact(c osint.Contact) error {
	if c.FoundAt.IsZero() {
		c.FoundAt = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO contacts
		 (company, domain, name, title, email, email_type, linkedin, source, confidence, found_at, notes)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Company, c.Domain, c.Name, c.Title,
		c.Email, c.EmailType, c.LinkedIn, c.Source, c.Confidence,
		c.FoundAt.UTC().Format(time.RFC3339), c.Notes,
	)
	return err
}

// ListContacts returns all saved contacts, newest first.
func (s *Store) ListContacts() ([]osint.Contact, error) {
	rows, err := s.db.Query(
		`SELECT id, company, domain, name, title, email, email_type, linkedin, source, confidence, found_at, notes
		 FROM contacts ORDER BY found_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []osint.Contact
	for rows.Next() {
		var c osint.Contact
		var foundAt string
		if err := rows.Scan(
			&c.ID, &c.Company, &c.Domain, &c.Name, &c.Title,
			&c.Email, &c.EmailType, &c.LinkedIn, &c.Source, &c.Confidence,
			&foundAt, &c.Notes,
		); err != nil {
			return nil, err
		}
		c.FoundAt, _ = time.Parse(time.RFC3339, foundAt)
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteContact removes a contact by ID.
func (s *Store) DeleteContact(id int64) error {
	_, err := s.db.Exec(`DELETE FROM contacts WHERE id = ?`, id)
	return err
}

// DomainForCompany returns a previously discovered email domain for a company
// (case-insensitive), or "" when none is known. Lets the outreach pipeline
// reuse domains found by earlier searches instead of re-guessing.
func (s *Store) DomainForCompany(company string) (string, error) {
	var domain string
	err := s.db.QueryRow(
		`SELECT domain FROM contacts
		 WHERE LOWER(company) = LOWER(?) AND domain != ''
		 ORDER BY confidence DESC, found_at DESC LIMIT 1`,
		company,
	).Scan(&domain)
	if err != nil {
		return "", nil // no rows → no known domain; other errors are non-fatal too
	}
	return domain, nil
}
