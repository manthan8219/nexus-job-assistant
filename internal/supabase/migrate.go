package supabase

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/inbox"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
)

// ImportReport summarises one migration run.
type ImportReport struct {
	Applications  int      `json:"applications"`
	Companies     int      `json:"companies"`
	Contacts      int      `json:"contacts"`
	SavedContacts int      `json:"savedContacts"`
	OutreachLog   int      `json:"outreachLog"`
	OutreachItems int      `json:"outreachItems"`
	Highlights    int      `json:"highlights"`
	Resumes       int      `json:"resumes"`
	Errors        []string `json:"errors,omitempty"`
}

// batchSize bounds rows per multi-row INSERT (under Postgres' 65535 param cap).
const batchSize = 400

type comp struct {
	id                      string
	name, site, ats, board  string
	boardURL, hc, hcc       string
	hq, hqc, kind, industry string
	src, updatedAt          string
}

// multiInsertSQL builds an INSERT for a batch with sequential placeholders.
func multiInsertSQL(table, cols string, n, perRow int) string {
	var b strings.Builder
	b.WriteString("INSERT INTO " + table + " (" + cols + ") VALUES ")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(")
		for j := 0; j < perRow; j++ {
			if j > 0 {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf("$%d", i*perRow+j+1))
		}
		b.WriteString(")")
	}
	return b.String()
}

// insertBatched performs a multi-row INSERT over values in batches.
func insertBatched(ctx context.Context, tx *sql.Tx, table, colsSQL string, values [][]any, onConflict string) (int, error) {
	if len(values) == 0 || len(values[0]) == 0 {
		return 0, nil
	}
	perRow := len(values[0])
	ins := 0
	for start := 0; start < len(values); start += batchSize {
		end := start + batchSize
		if end > len(values) {
			end = len(values)
		}
		batch := values[start:end]
		sqlStr := multiInsertSQL(table, colsSQL, len(batch), perRow) + " " + onConflict
		flat := make([]any, 0, len(batch)*perRow)
		for _, row := range batch {
			flat = append(flat, row...)
		}
		if _, err := tx.ExecContext(ctx, sqlStr, flat...); err != nil {
			return ins, fmt.Errorf("insert %s batch: %w", table, err)
		}
		ins += len(batch)
	}
	return ins, nil
}

// ImportApplications copies applications from local SQLite to Supabase.
func ImportApplications(ctx context.Context, pg *sql.DB, src *sql.DB) (int, error) {
	rows, err := src.QueryContext(ctx, `SELECT provider, company, role, url, status, reason, applied_at, location, remote,
		posted_at, description, fit_score, fit_summary, outcome, outcome_at, approved, submitted_payload
		FROM applications ORDER BY id`)
	if err != nil {
		return 0, fmt.Errorf("read applications: %w", err)
	}
	defer rows.Close()

	var values [][]any
	for rows.Next() {
		var provider, company, role, url, status, reason, appliedAt, location, postedAt string
		var description, fitScore, fitSummary, outcome, outcomeAt, submittedPayload string
		var remote, approved int64
		if err := rows.Scan(&provider, &company, &role, &url, &status, &reason, &appliedAt,
			&location, &remote, &postedAt, &description, &fitScore, &fitSummary,
			&outcome, &outcomeAt, &approved, &submittedPayload); err != nil {
			return 0, fmt.Errorf("scan application: %w", err)
		}
		values = append(values, []any{provider, company, role, url, status, reason, appliedAt,
			location, remote != 0, postedAt, description, parseScore(fitScore), fitSummary,
			outcome, outcomeAt, approved != 0, submittedPayload})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	tx, err := pg.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n, err := insertBatched(ctx, tx, "applications",
		"provider, company, role, url, status, reason, applied_at, location, remote, posted_at, description, fit_score, fit_summary, outcome, outcome_at, approved, submitted_payload",
		values, "ON CONFLICT (url) DO NOTHING")
	if err != nil {
		return n, err
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

// ImportCompanies copies companies, then company_countries via a single pg id
// lookup. Idempotent.
func ImportCompanies(ctx context.Context, pg *sql.DB, src *sql.DB) (int, error) {
	rows, err := src.QueryContext(ctx, `SELECT id, name, website, ats, board, board_url, hire_countries, hire_country_codes,
		hq_country, hq_country_code, kind, industry, source, updated_at FROM companies ORDER BY id`)
	if err != nil {
		return 0, fmt.Errorf("read companies: %w", err)
	}
	defer rows.Close()

	var comps []comp
	var values [][]any
	for rows.Next() {
		var id, name, website, ats, board, boardURL, hc, hcc, hq, hqc, kind, industry, source, updatedAt string
		if err := rows.Scan(&id, &name, &website, &ats, &board, &boardURL, &hc, &hcc,
			&hq, &hqc, &kind, &industry, &source, &updatedAt); err != nil {
			continue
		}
		comps = append(comps, comp{id: id, name: name, site: website, ats: ats, board: board,
			boardURL: boardURL, hc: hc, hcc: hcc, hq: hq, hqc: hqc, kind: kind, industry: industry,
			src: source, updatedAt: updatedAt})
		values = append(values, []any{name, website, ats, board, boardURL, hc, hcc,
			hq, hqc, kind, industry, source, updatedAt})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	tx, err := pg.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n, err := insertBatched(ctx, tx, "companies",
		"name, website, ats, board, board_url, hire_countries, hire_country_codes, hq_country, hq_country_code, kind, industry, source, updated_at",
		values, "ON CONFLICT (name, board_url) DO NOTHING")
	if err != nil {
		return n, err
	}
	if err := ImportCompanyCountries(ctx, tx, src, comps); err != nil {
		return n, err
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

// ImportCompanyCountries inserts company_countries, mapping local company ids
// to pg ids through one SELECT (in memory) to avoid per-row round trips.
func ImportCompanyCountries(ctx context.Context, tx *sql.Tx, src *sql.DB, comps []comp) error {
	// local id -> (lower name, board_url)
	byLocal := make(map[string][2]string, len(comps))
	for _, c := range comps {
		byLocal[c.id] = [2]string{strings.ToLower(c.name), c.boardURL}
	}

	// pg id lookup: one query, build map (lower(name), board_url) -> id
	pgMap := map[[2]string]int64{}
	pgRows, err := tx.QueryContext(ctx, `SELECT id, lower(name), board_url FROM companies`)
	if err != nil {
		return fmt.Errorf("list pg companies: %w", err)
	}
	defer pgRows.Close()
	for pgRows.Next() {
		var id int64
		var name, board string
		if err := pgRows.Scan(&id, &name, &board); err == nil {
			pgMap[[2]string{name, board}] = id
		}
	}
	if err := pgRows.Err(); err != nil {
		return err
	}

	ccRows, err := src.QueryContext(ctx, `SELECT company_id, country_key FROM company_countries`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	defer ccRows.Close()

	var values [][]any
	for ccRows.Next() {
		var lid, key string
		if err := ccRows.Scan(&lid, &key); err != nil {
			continue
		}
		nameURL, ok := byLocal[lid]
		if !ok {
			continue
		}
		if pgID, ok := pgMap[nameURL]; ok {
			values = append(values, []any{pgID, key})
		}
	}
	_, err = insertBatched(ctx, tx, "company_countries", "company_id, country_key", values, "ON CONFLICT DO NOTHING")
	return err
}

// ImportSavedContacts copies saved OSINT contacts.
func ImportSavedContacts(ctx context.Context, pg *sql.DB, src *sql.DB) (int, error) {
	return importTable(ctx, pg, src, "saved_contacts", []string{
		"company", "domain", "name", "title", "email", "email_type", "linked_in", "source", "confidence", "found_at", "notes",
	}, `SELECT company, domain, name, title, email, email_type, linked_in, source, confidence, found_at, notes FROM saved_contacts`,
		"ON CONFLICT (company, email, linked_in) DO NOTHING")
}

// ImportStoreContacts copies the store finder contacts table (applications.db).
func ImportStoreContacts(ctx context.Context, pg *sql.DB, src *sql.DB) (int, error) {
	return importTable(ctx, pg, src, "contacts", []string{
		"company", "domain", "name", "title", "email", "email_type", "linkedin", "source", "confidence", "found_at", "notes",
	}, `SELECT company, domain, name, title, email, email_type, linkedin, source, confidence, found_at, notes FROM contacts`,
		"ON CONFLICT (email, company) DO NOTHING")
}

// ImportOutreachLog copies the outreach audit log (applications.db).
func ImportOutreachLog(ctx context.Context, pg *sql.DB, src *sql.DB) (int, error) {
	return importTable(ctx, pg, src, "outreach_log", []string{
		"channel", "job_url", "company", "role", "contact_name", "contact_email", "contact_source",
		"subject", "body", "status", "error", "review_score", "attempts", "created_at", "sent_at",
	}, `SELECT channel, job_url, company, role, contact_name, contact_email, contact_source,
	   subject, body, status, error, review_score, attempts, created_at, sent_at FROM outreach_log`,
		"")
}

// importTable reads a simple text-typed table from src and bulk-inserts it.
func importTable(ctx context.Context, pg *sql.DB, src *sql.DB, table string, cols []string, query, onConflict string) (int, error) {
	rows, err := src.QueryContext(ctx, query)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, fmt.Errorf("read %s: %w", table, err)
	}
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return 0, err
	}
	var values [][]any
	for rows.Next() {
		rec := make([]any, len(colTypes))
		ptrs := make([]any, len(colTypes))
		for i := range rec {
			ptrs[i] = &rec[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		values = append(values, rec)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	tx, err := pg.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n, err := insertBatched(ctx, tx, table, strings.Join(cols, ", "), values, onConflict)
	if err != nil {
		return n, err
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

// ImportOutreachItems copies the outreach JSON store into outreach_items.
func ImportOutreachItems(ctx context.Context, pg *sql.DB) (int, error) {
	items, err := outreach.Load()
	if err != nil {
		return 0, fmt.Errorf("load outreach items: %w", err)
	}
	values := make([][]any, 0, len(items))
	for _, it := range items {
		values = append(values, []any{it.ID, string(it.Channel), it.JobURL, it.Company, it.Role,
			it.Provider, it.ContactName, it.ContactEmail, it.ContactTitle, it.ContactSource,
			it.LinkedInURL, it.Subject, it.Body, string(it.Status), it.Error, it.Auto, it.ReviewScore,
			it.ReviewNotes, it.Attempts, ts(it.CreatedAt), ts(it.UpdatedAt), ts(it.SentAt),
			it.FollowUpStep, ts(it.NextSendAt), it.Variant})
	}
	tx, err := pg.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n, err := insertBatched(ctx, tx, "outreach_items", "id, channel, job_url, company, role, provider, contact_name, contact_email, contact_title, contact_source, linkedin_url, subject, body, status, error, auto, review_score, review_notes, attempts, created_at, updated_at, sent_at, follow_up_step, next_send_at, variant",
		values, "ON CONFLICT (id) DO NOTHING")
	if err != nil {
		return n, err
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

// ImportHighlights copies the highlights JSON store into the highlights table.
func ImportHighlights(ctx context.Context, pg *sql.DB) (int, error) {
	p, err := inbox.HighlightsPath()
	if err != nil {
		return 0, err
	}
	hs, err := inbox.LoadAll(p)
	if err != nil {
		return 0, fmt.Errorf("load highlights: %w", err)
	}
	values := make([][]any, 0, len(hs))
	for _, h := range hs {
		d := h.Date
		if d.IsZero() {
			d = time.Unix(0, 0).UTC()
		}
		values = append(values, []any{h.ID, h.MessageID, h.From, h.FromName, h.Subject,
			h.BodyPreview, ts(d), string(h.Signal), h.Confidence, h.Domain, h.Company, h.AppID, h.Seen})
	}
	tx, err := pg.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n, err := insertBatched(ctx, tx, "highlights", "id, message_id, from_addr, from_name, subject, body_preview, date, signal, confidence, domain, company, app_id, seen",
		values, "ON CONFLICT (id) DO NOTHING")
	if err != nil {
		return n, err
	}
	if err := tx.Commit(); err != nil {
		return n, err
	}
	return n, nil
}

// UploadResumes uploads every PDF in dir to the resumes bucket.
func UploadResumes(ctx context.Context, c *Client, dir string) (int, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, []string{fmt.Sprintf("read resumes dir: %v", err)}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	n := 0
	var errs []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pdf") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			errs = append(errs, e.Name()+": "+err.Error())
			continue
		}
		if err := c.UploadResume(ctx, e.Name(), data); err != nil {
			errs = append(errs, e.Name()+": "+err.Error())
			continue
		}
		n++
	}
	return n, errs
}

// ts renders a time as RFC3339 (zero -> the 0001 sentinel Postgres accepts).
func ts(t time.Time) string {
	if t.IsZero() {
		return "0001-01-01T00:00:00Z"
	}
	return t.UTC().Format(time.RFC3339)
}

// parseScore tolerantly converts a stored score to an int.
func parseScore(s string) int {
	var n int
	fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n
}
