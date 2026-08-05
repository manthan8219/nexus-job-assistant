package settings

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/pgutil"
)

// ApplyTo merges non-zero overrides into cfg. Zero values (empty string, false,
// 0) are treated as "not set" and never overwrite a config-file/env value, so a
// partially-filled settings row upgrades an existing config without wiping it.
func (o *ConfigOverrides) ApplyTo(c *config.Config) {
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&c.FirstName, o.FirstName)
	set(&c.LastName, o.LastName)
	set(&c.Email, o.Email)
	set(&c.GmailAppPassword, o.GmailAppPassword)
	set(&c.City, o.City)
	set(&c.Phone, o.Phone)
	set(&c.LinkedInID, o.LinkedInID)
	set(&c.TargetJobTitles, o.TargetJobTitles)
	set(&c.WorkType, o.WorkType)
	set(&c.TargetLocations, o.TargetLocations)
	set(&c.Currency, o.Currency)
	set(&c.MinSalary, o.MinSalary)
	set(&c.CompanyBlocklist, o.CompanyBlocklist)
	set(&c.NoticePeriodDays, o.NoticePeriodDays)
	set(&c.OfficeDaysPerWeek, o.OfficeDaysPerWeek)
	set(&c.CoverLetterMode, o.CoverLetterMode)
	set(&c.WorkAuth, o.WorkAuth)
	set(&c.ResumePath, o.ResumePath)

	if o.OutreachConsent {
		c.OutreachConsent = true
	}
	if o.ApplyConsent {
		c.ApplyConsent = true
	}
	if o.InboxScanMinutes != 0 {
		c.InboxScanMinutes = o.InboxScanMinutes
	}
	if o.MaxAppsPerRun != 0 {
		c.MaxAppsPerRun = o.MaxAppsPerRun
	}
	if o.MaxAppsPerDay != 0 {
		c.MaxAppsPerDay = o.MaxAppsPerDay
	}
	if o.ApplyDelaySec != 0 {
		c.ApplyDelaySec = o.ApplyDelaySec
	}
	if o.MinFitScore != 0 {
		c.MinFitScore = o.MinFitScore
	}
}

// SettingsMasterKeyEnv is the env var holding the master key that unwraps the
// in-DB key sealing each user's Gmail app password. It never lives in config
// files or the database itself. Optional: when unset, a stored password simply
// cannot be decrypted and the config-file/env password is used instead.
const SettingsMasterKeyEnv = "NEXUS_SETTINGS_MASTER_KEY"

// ApplyToConfig reads the user_settings row from the Supabase Postgres named
// by cfg.DatabaseURL and merges it into cfg (DB values win). It is a no-op
// when no DatabaseURL is configured. If a stored Gmail password cannot be
// decrypted (missing/wrong master key) the failure is swallowed so config-file
// or env credentials still work - graceful degradation, never app failure.
func ApplyToConfig(ctx context.Context, cfg *config.Config) error {
	if cfg == nil || strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return pgutil.WrapConnectError(err, cfg.DatabaseURL)
	}
	defer db.Close()

	st := NewStore(db, []byte(os.Getenv(SettingsMasterKeyEnv)))
	if err := st.EnsureSchema(ctx); err != nil {
		return pgutil.WrapConnectError(err, cfg.DatabaseURL)
	}
	over, err := st.Load(ctx)
	if err != nil {
		if errors.Is(err, ErrSealed) || errors.Is(err, ErrNotFound) {
			return nil // cannot decrypt or nothing stored - keep existing values
		}
		return err
	}
	over.ApplyTo(cfg)
	return nil
}
