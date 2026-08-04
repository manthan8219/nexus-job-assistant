package config

import (
	"path/filepath"
	"testing"
)

// TestLoadFrom_EnvOverrides ensures NEXUS_* env vars fill config from an
// empty file - the deployment path on platforms without a persistent disk.
func TestLoadFrom_EnvOverrides(t *testing.T) {
	t.Setenv("NEXUS_SUPABASE_URL", "https://x.supabase.co")
	t.Setenv("NEXUS_DATABASE_URL", "postgres://u:p@db:5432/postgres")
	t.Setenv("NEXUS_SUPABASE_SERVICE_KEY", "svc")
	t.Setenv("NEXUS_GMAIL_APP_PASSWORD", "pass")
	t.Setenv("NEXUS_EMAIL", "me@m.com")
	t.Setenv("NEXUS_FIRST_NAME", "Ada")
	t.Setenv("NEXUS_LAST_NAME", "Lovelace")
	t.Setenv("NEXUS_OUTREACH_CONSENT", "true")
	t.Setenv("NEXUS_INBOX_SCAN_MINUTES", "60")

	// Missing file -> empty config + env overrides.
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.SupabaseURL != "https://x.supabase.co" || cfg.DatabaseURL == "" || cfg.SupabaseServiceKey != "svc" {
		t.Errorf("supabase env not applied: %+v", cfg)
	}
	if cfg.GmailAppPassword != "pass" || cfg.Email != "me@m.com" {
		t.Errorf("gmail env not applied: %+v", cfg)
	}
	if cfg.FirstName != "Ada" || cfg.LastName != "Lovelace" {
		t.Errorf("identity env not applied: %+v", cfg)
	}
	if !cfg.OutreachConsent {
		t.Error("NEXUS_OUTREACH_CONSENT=true not applied")
	}
	if cfg.InboxScanMinutes != 60 {
		t.Errorf("NEXUS_INBOX_SCAN_MINUTES not applied: %d", cfg.InboxScanMinutes)
	}
}
