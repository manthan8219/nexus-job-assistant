// Command set-settings saves the current effective profile (config file + env)
// into the Supabase user_settings table - including the Gmail app password,
// which is AES-encrypted at rest. Run it once (or after editing profile fields)
// so the deployed app can source per-user values from Postgres instead of env.
//
// Usage:
//
//	set-settings                       save profile + Gmail password from config
//	set-settings -gmail-password X     override the password for this save
//	set-settings -clear-gmail          store an empty password (clear it)
//
// The Gmail password is encrypted with a key that lives in the same project and
// is itself wrapped by NEXUS_SETTINGS_MASTER_KEY - a raw database dump exposes
// only ciphertext, never the plaintext password.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/settings"
)

func main() {
	gmail := flag.String("gmail-password", "", "Gmail app password to store (overrides config value)")
	clearGmail := flag.Bool("clear-gmail", false, "store an empty Gmail password (clears it)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		fmt.Fprintln(os.Stderr, "database_url is not configured; nothing to save into.")
		os.Exit(1)
	}

	pw := cfg.GmailAppPassword
	if *gmail != "" {
		pw = *gmail
	}
	if *clearGmail {
		pw = ""
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer db.Close()

	st := settings.NewStore(db, []byte(os.Getenv(settings.SettingsMasterKeyEnv)))
	ctx := context.Background()
	if err := st.EnsureSchema(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "schema:", err)
		os.Exit(1)
	}

	over := &settings.ConfigOverrides{
		FirstName:         cfg.FirstName,
		LastName:          cfg.LastName,
		Email:             cfg.Email,
		GmailAppPassword:  pw,
		OutreachConsent:   cfg.OutreachConsent,
		InboxScanMinutes:  cfg.InboxScanMinutes,
		City:              cfg.City,
		Phone:             cfg.Phone,
		LinkedInID:        cfg.LinkedInID,
		TargetJobTitles:   cfg.TargetJobTitles,
		WorkType:          cfg.WorkType,
		TargetLocations:   cfg.TargetLocations,
		Currency:          cfg.Currency,
		MinSalary:         cfg.MinSalary,
		ApplyConsent:      cfg.ApplyConsent,
		MaxAppsPerRun:     cfg.MaxAppsPerRun,
		MaxAppsPerDay:     cfg.MaxAppsPerDay,
		ApplyDelaySec:     cfg.ApplyDelaySec,
		MinFitScore:       cfg.MinFitScore,
		CompanyBlocklist:  cfg.CompanyBlocklist,
		NoticePeriodDays:  cfg.NoticePeriodDays,
		OfficeDaysPerWeek: cfg.OfficeDaysPerWeek,
		CoverLetterMode:   cfg.CoverLetterMode,
		WorkAuth:          cfg.WorkAuth,
		ResumePath:        cfg.ResumePath,
	}
	if err := st.Save(ctx, over); err != nil {
		fmt.Fprintln(os.Stderr, "save:", err)
		os.Exit(1)
	}

	stored := "stored"
	if pw == "" {
		stored = "empty (cleared)"
	} else {
		stored = "stored (AES-encrypted at rest)"
	}
	fmt.Printf("OK - user_settings row saved to Supabase. Gmail password: %s. Master key %s set.\n",
		stored, map[bool]string{true: "IS", false: "is NOT"}[os.Getenv(settings.SettingsMasterKeyEnv) != ""])
	fmt.Printf("Profile: %s %s <%s>.\n", cfg.FirstName, cfg.LastName, cfg.Email)
}
