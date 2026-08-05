package api

import (
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// TestConfigRoundtrip verifies catalog-less two-way conversion between the
// backend config.Config and the frontend NexusConfig shape: every field the
// handlers expose survives configToNexusConfig → applyNexusConfig.
func TestConfigRoundtrip(t *testing.T) {
	cfg := testConfig()
	cfg.NoticePeriodDays = "30"
	cfg.OfficeDaysPerWeek = "4"

	nc := configToNexusConfig(cfg)
	if nc.FirstName != "Ada" || nc.NoticePeriodDays != 30 || nc.OfficeDaysPerWeek != 4 {
		t.Fatalf("configToNexusConfig = %+v; want Ada + 30 + 4", nc)
	}
	if nc.MaxAppsPerRun != 5 || !nc.ApplyConsent || nc.NotifyChannels[0] != "discord" {
		t.Errorf("converted apply fields wrong: %+v", nc)
	}

	back := &config.Config{}
	applyNexusConfig(back, nc)
	if back.FirstName != "Ada" || back.NoticePeriodDays != "30" {
		t.Errorf("applyNexusConfig: first=%q notice=%q; want Ada/30", back.FirstName, back.NoticePeriodDays)
	}
	if !back.ApplyConsent || back.MaxAppsPerRun != 5 || back.EmailNotifications != true {
		t.Errorf("applyNexusConfig lost apply/notify fields: %+v", back)
	}
	if back.TargetLocations != "Remote" || back.DailyRunAt != "09:00" {
		t.Errorf("applyNexusConfig lost preference fields: %+v", back)
	}
}

func TestIntrospectFormatting(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "empty", in: "", want: 0},
		{name: "plain number", in: "42", want: 42},
		{name: "trailing junk", in: "12x", want: 0},
		{name: "embedded junk", in: "1a2", want: 0},
		{name: "multi-digit", in: "99999999", want: 99999999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseInt(tt.in); got != tt.want {
				t.Errorf("parseInt(%q) = %d; want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{in: 0, want: ""},
		{in: 7, want: "7"},
		{in: 123, want: "123"},
		{in: 1000, want: "1000"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := itoa(tt.in); got != tt.want {
				t.Errorf("itoa(%d) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}
