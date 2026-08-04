package supabase

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SchemaDDL creates the Nexus tables in the Supabase Postgres database,
// mirroring the local SQLite schemas (internal/store, internal/companies,
// internal/contacts) plus the outreach_items and highlights tables that
// replace the JSON file stores. All statements are idempotent.
const SchemaDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version     TEXT PRIMARY KEY,
	applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS applications (
	id                BIGSERIAL PRIMARY KEY,
	provider          TEXT NOT NULL,
	company           TEXT NOT NULL,
	role              TEXT NOT NULL,
	url               TEXT NOT NULL UNIQUE,
	status            TEXT NOT NULL DEFAULT 'applied',
	reason            TEXT NOT NULL DEFAULT '',
	applied_at        TIMESTAMPTZ NOT NULL,
	location          TEXT NOT NULL DEFAULT '',
	remote            BOOLEAN NOT NULL DEFAULT FALSE,
	posted_at         TIMESTAMPTZ NOT NULL DEFAULT '0001-01-01T00:00:00Z',
	description       TEXT NOT NULL DEFAULT '',
	fit_score         INTEGER NOT NULL DEFAULT 0,
	fit_summary       TEXT NOT NULL DEFAULT '',
	outcome           TEXT NOT NULL DEFAULT '',
	outcome_at        TIMESTAMPTZ NOT NULL DEFAULT '0001-01-01T00:00:00Z',
	approved          BOOLEAN NOT NULL DEFAULT FALSE,
	submitted_payload TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_applications_applied_at ON applications(applied_at);
CREATE INDEX IF NOT EXISTS idx_applications_status     ON applications(status);

CREATE TABLE IF NOT EXISTS companies (
	id                 BIGSERIAL PRIMARY KEY,
	name               TEXT NOT NULL,
	website            TEXT NOT NULL DEFAULT '',
	ats                TEXT NOT NULL DEFAULT '',
	board              TEXT NOT NULL DEFAULT '',
	board_url          TEXT NOT NULL DEFAULT '',
	hire_countries     TEXT NOT NULL DEFAULT '[]',
	hire_country_codes TEXT NOT NULL DEFAULT '[]',
	hq_country         TEXT NOT NULL DEFAULT '',
	hq_country_code    TEXT NOT NULL DEFAULT '',
	kind               TEXT NOT NULL DEFAULT '',
	industry           TEXT NOT NULL DEFAULT '',
	source             TEXT NOT NULL DEFAULT '',
	updated_at         TIMESTAMPTZ NOT NULL,
	UNIQUE (name, board_url)
);
CREATE INDEX IF NOT EXISTS idx_companies_name ON companies(name);

CREATE TABLE IF NOT EXISTS company_countries (
	company_id  BIGINT NOT NULL,
	country_key TEXT NOT NULL,
	PRIMARY KEY (company_id, country_key),
	FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_company_countries_key ON company_countries(country_key);

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

CREATE TABLE IF NOT EXISTS saved_contacts (
	id         BIGSERIAL PRIMARY KEY,
	company    TEXT NOT NULL DEFAULT '',
	domain     TEXT NOT NULL DEFAULT '',
	name       TEXT NOT NULL DEFAULT '',
	title      TEXT NOT NULL DEFAULT '',
	email      TEXT NOT NULL DEFAULT '',
	email_type TEXT NOT NULL DEFAULT '',
	linked_in  TEXT NOT NULL DEFAULT '',
	source     TEXT NOT NULL DEFAULT '',
	confidence INTEGER NOT NULL DEFAULT 0,
	found_at   TIMESTAMPTZ NOT NULL,
	notes      TEXT NOT NULL DEFAULT '',
	UNIQUE (company, email, linked_in)
);
CREATE INDEX IF NOT EXISTS idx_saved_contacts_company ON saved_contacts(company);
CREATE INDEX IF NOT EXISTS idx_saved_contacts_email   ON saved_contacts(email);

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

CREATE TABLE IF NOT EXISTS outreach_items (
	id             TEXT PRIMARY KEY,
	channel        TEXT NOT NULL DEFAULT 'email',
	job_url        TEXT NOT NULL DEFAULT '',
	company        TEXT NOT NULL DEFAULT '',
	role           TEXT NOT NULL DEFAULT '',
	provider       TEXT NOT NULL DEFAULT '',
	contact_name   TEXT NOT NULL DEFAULT '',
	contact_email  TEXT NOT NULL DEFAULT '',
	contact_title  TEXT NOT NULL DEFAULT '',
	contact_source TEXT NOT NULL DEFAULT '',
	linkedin_url   TEXT NOT NULL DEFAULT '',
	subject        TEXT NOT NULL DEFAULT '',
	body           TEXT NOT NULL DEFAULT '',
	status         TEXT NOT NULL DEFAULT '',
	error          TEXT NOT NULL DEFAULT '',
	auto           BOOLEAN NOT NULL DEFAULT FALSE,
	review_score   INTEGER NOT NULL DEFAULT 0,
	review_notes   TEXT NOT NULL DEFAULT '',
	attempts       INTEGER NOT NULL DEFAULT 0,
	created_at     TIMESTAMPTZ NOT NULL,
	updated_at     TIMESTAMPTZ NOT NULL,
	sent_at        TIMESTAMPTZ NOT NULL DEFAULT '0001-01-01T00:00:00Z',
	follow_up_step INTEGER NOT NULL DEFAULT 0,
	next_send_at   TIMESTAMPTZ NOT NULL DEFAULT '0001-01-01T00:00:00Z',
	variant        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_outreach_items_status ON outreach_items(status);

CREATE TABLE IF NOT EXISTS highlights (
	id           TEXT PRIMARY KEY,
	message_id   TEXT NOT NULL DEFAULT '',
	from_addr    TEXT NOT NULL DEFAULT '',
	from_name    TEXT NOT NULL DEFAULT '',
	subject      TEXT NOT NULL DEFAULT '',
	body_preview TEXT NOT NULL DEFAULT '',
	date         TIMESTAMPTZ NOT NULL,
	signal       TEXT NOT NULL DEFAULT '',
	confidence   INTEGER NOT NULL DEFAULT 0,
	domain       TEXT NOT NULL DEFAULT '',
	company      TEXT NOT NULL DEFAULT '',
	app_id       BIGINT NOT NULL DEFAULT 0,
	seen         BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX IF NOT EXISTS idx_highlights_signal ON highlights(signal);
CREATE INDEX IF NOT EXISTS idx_highlights_date ON highlights(date);
`

// SchemaVersion is recorded in schema_migrations after a successful run.
const SchemaVersion = "0001_supabase"

// CreateSchema applies SchemaDDL to the Supabase database and records the
// migration version. Safe to re-run.
func CreateSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("supabase: no database")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("supabase: begin schema: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, SchemaDDL); err != nil {
		return fmt.Errorf("supabase: apply schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2) ON CONFLICT (version) DO NOTHING`,
		SchemaVersion, time.Now().UTC()); err != nil {
		return fmt.Errorf("supabase: record migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("supabase: commit schema: %w", err)
	}
	return nil
}
