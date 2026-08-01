package store

import (
	"strconv"
	"time"
)

// OutreachLogEntry is a permanent audit record of one outreach action
// (email sent / send failed / LinkedIn opened) — what was sent, to whom,
// for which job, and with what AI review data.
type OutreachLogEntry struct {
	ID            int64     `json:"id"`
	Channel       string    `json:"channel"` // "email" | "linkedin"
	JobURL        string    `json:"jobURL"`
	Company       string    `json:"company"`
	Role          string    `json:"role"`
	ContactName   string    `json:"contactName"`
	ContactEmail  string    `json:"contactEmail"`
	ContactSource string    `json:"contactSource"`
	Subject       string    `json:"subject"`
	Body          string    `json:"body"`
	Status        string    `json:"status"` // "sent" | "failed" | "opened"
	Error         string    `json:"error"`
	ReviewScore   int       `json:"reviewScore"`
	Attempts      int       `json:"attempts"`
	CreatedAt     time.Time `json:"createdAt"`
	SentAt        time.Time `json:"sentAt"`
}

const outreachLogDDL = `
CREATE TABLE IF NOT EXISTS outreach_log (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	channel        TEXT    NOT NULL DEFAULT '',
	job_url        TEXT    NOT NULL DEFAULT '',
	company        TEXT    NOT NULL DEFAULT '',
	role           TEXT    NOT NULL DEFAULT '',
	contact_name   TEXT    NOT NULL DEFAULT '',
	contact_email  TEXT    NOT NULL DEFAULT '',
	contact_source TEXT    NOT NULL DEFAULT '',
	subject        TEXT    NOT NULL DEFAULT '',
	body           TEXT    NOT NULL DEFAULT '',
	status         TEXT    NOT NULL DEFAULT '',
	error          TEXT    NOT NULL DEFAULT '',
	review_score   INTEGER NOT NULL DEFAULT 0,
	attempts       INTEGER NOT NULL DEFAULT 0,
	created_at     DATETIME NOT NULL,
	sent_at        DATETIME NOT NULL DEFAULT '0001-01-01T00:00:00Z'
);
CREATE INDEX IF NOT EXISTS idx_outreach_log_sent_at ON outreach_log(sent_at);
CREATE INDEX IF NOT EXISTS idx_outreach_log_company ON outreach_log(company);
`

// SaveOutreachLog appends one entry to the outreach audit log.
func (s *Store) SaveOutreachLog(e OutreachLogEntry) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	sentAt := e.SentAt
	if sentAt.IsZero() {
		sentAt = time.Time{}
	}
	_, err := s.db.Exec(
		`INSERT INTO outreach_log
		 (channel, job_url, company, role, contact_name, contact_email, contact_source,
		  subject, body, status, error, review_score, attempts, created_at, sent_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Channel, e.JobURL, e.Company, e.Role, e.ContactName, e.ContactEmail, e.ContactSource,
		e.Subject, e.Body, e.Status, e.Error, e.ReviewScore, e.Attempts,
		e.CreatedAt.UTC().Format(time.RFC3339), sentAt.UTC().Format(time.RFC3339),
	)
	return err
}

// ListOutreachLog returns log entries, newest first. limit <= 0 means all.
func (s *Store) ListOutreachLog(limit int) ([]OutreachLogEntry, error) {
	q := `SELECT id, channel, job_url, company, role, contact_name, contact_email, contact_source,
	             subject, body, status, error, review_score, attempts, created_at, sent_at
	      FROM outreach_log ORDER BY id DESC`
	if limit > 0 {
		q += ` LIMIT ` + strconv.Itoa(limit)
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OutreachLogEntry
	for rows.Next() {
		var e OutreachLogEntry
		var createdAt, sentAt string
		if err := rows.Scan(
			&e.ID, &e.Channel, &e.JobURL, &e.Company, &e.Role,
			&e.ContactName, &e.ContactEmail, &e.ContactSource,
			&e.Subject, &e.Body, &e.Status, &e.Error,
			&e.ReviewScore, &e.Attempts, &createdAt, &sentAt,
		); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		e.SentAt, _ = time.Parse(time.RFC3339, sentAt)
		out = append(out, e)
	}
	return out, rows.Err()
}
