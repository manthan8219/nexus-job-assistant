package config

import (
	"os"
	"strconv"
	"strings"
)

// applyEnv overrides config fields from environment variables so the app can
// run without a config file - e.g. Render free tier has no persistent disk, so
// the NEXUS_* vars set in the platform dashboard become the source of truth.
// Empty env values leave the config-file value untouched. Secrets are never
// logged.
func applyEnv(c *Config) {
	if v := strings.TrimSpace(os.Getenv("NEXUS_SUPABASE_URL")); v != "" {
		c.SupabaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("NEXUS_DATABASE_URL")); v != "" {
		c.DatabaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("NEXUS_SUPABASE_SERVICE_KEY")); v != "" {
		c.SupabaseServiceKey = v
	}
	if v := strings.TrimSpace(os.Getenv("NEXUS_GMAIL_APP_PASSWORD")); v != "" {
		c.GmailAppPassword = v
	}
	if v := strings.TrimSpace(os.Getenv("NEXUS_EMAIL")); v != "" {
		c.Email = v
	}
	if v := strings.TrimSpace(os.Getenv("NEXUS_FIRST_NAME")); v != "" {
		c.FirstName = v
	}
	if v := strings.TrimSpace(os.Getenv("NEXUS_LAST_NAME")); v != "" {
		c.LastName = v
	}
	if v := strings.TrimSpace(os.Getenv("NEXUS_CITY")); v != "" {
		c.City = v
	}
	if v := strings.TrimSpace(os.Getenv("NEXUS_OUTREACH_CONSENT")); v != "" {
		c.OutreachConsent, _ = strconv.ParseBool(v)
	}
	if v := strings.TrimSpace(os.Getenv("NEXUS_INBOX_SCAN_MINUTES")); v != "" {
		c.InboxScanMinutes, _ = strconv.Atoi(v)
	}
}
