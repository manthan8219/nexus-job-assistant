// Command migrate-supabase migrates Nexus local data (SQLite + JSON stores +
// resume PDFs) into the configured Supabase project: it creates the Postgres
// schema, imports every table, and uploads resume files to the resumes bucket.
//
// Everything is idempotent (ON CONFLICT DO NOTHING), so re-running after a
// partial failure is safe. Credentials come from config (never flags/args);
// verify the wiring first with cmd/supabase-check.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/nexusdir"
	"github.com/manthan8219/nexus-job-assistant/internal/supabase"
	_ "modernc.org/sqlite"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fatal("config", err)
	}
	c := supabase.FromConfig(cfg)
	if c == nil {
		fatal("supabase", fmt.Errorf("not configured - set supabase_url in config and run cmd/supabase-check first"))
	}

	ctx := context.Background()
	pg, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		fatal("open supabase db", err)
	}
	defer pg.Close()
	if err := pg.PingContext(ctx); err != nil {
		fatal("ping supabase db", err)
	}

	fmt.Println("Supabase reachable. Applying schema...")
	if err := supabase.CreateSchema(ctx, pg); err != nil {
		fatal("create schema", err)
	}

	home := nexusdir.Home()

	// Local SQLite stores.
	apps, err := openLocal(filepath.Join(home, "applications.db"))
	if err != nil {
		fatal("open applications.db", err)
	}
	defer apps.Close()
	companies, err := openLocal(filepath.Join(home, "companies.db"))
	if err != nil {
		fatal("open companies.db", err)
	}
	defer companies.Close()
	contacts, err := openLocal(filepath.Join(home, "contacts.db"))
	if err != nil {
		fatal("open contacts.db", err)
	}
	defer contacts.Close()

	report := supabase.ImportReport{}
	report.Applications, err = supabase.ImportApplications(ctx, pg, apps)
	if err != nil {
		report.Errors = append(report.Errors, "applications: "+err.Error())
	}
	report.Companies, err = supabase.ImportCompanies(ctx, pg, companies)
	if err != nil {
		report.Errors = append(report.Errors, "companies: "+err.Error())
	}
	report.SavedContacts, err = supabase.ImportSavedContacts(ctx, pg, contacts)
	if err != nil {
		report.Errors = append(report.Errors, "saved_contacts: "+err.Error())
	}
	report.OutreachItems, err = supabase.ImportOutreachItems(ctx, pg)
	if err != nil {
		report.Errors = append(report.Errors, "outreach_items: "+err.Error())
	}
	report.Highlights, err = supabase.ImportHighlights(ctx, pg)
	if err != nil {
		report.Errors = append(report.Errors, "highlights: "+err.Error())
	}
	report.Contacts, err = supabase.ImportStoreContacts(ctx, pg, apps)
	if err != nil {
		report.Errors = append(report.Errors, "contacts: "+err.Error())
	}
	report.OutreachLog, err = supabase.ImportOutreachLog(ctx, pg, apps)
	if err != nil {
		report.Errors = append(report.Errors, "outreach_log: "+err.Error())
	}
	var errs []string
	report.Resumes, errs = supabase.UploadResumes(ctx, c, filepath.Join(home, "resumes"))
	report.Errors = append(report.Errors, errs...)

	fmt.Printf("Migration report:\n  applications=%d companies=%d saved_contacts=%d outreach_items=%d highlights=%d resumes=%d\n",
		report.Applications, report.Companies, report.SavedContacts, report.OutreachItems, report.Highlights, report.Resumes)
	for _, e := range report.Errors {
		fmt.Fprintln(os.Stderr, "  warn:", e)
	}
	if len(report.Errors) == 0 {
		fmt.Println("OK - local data migrated to Supabase.")
	} else {
		fmt.Fprintln(os.Stderr, "Completed with warnings above; re-run to retry failed rows (idempotent).")
	}
}

func openLocal(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func fatal(where string, err error) {
	fmt.Fprintf(os.Stderr, "migrate-supabase: %s: %v\n", where, err)
	os.Exit(1)
}
