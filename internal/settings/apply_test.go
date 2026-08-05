package settings

import (
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestApplyTo(t *testing.T) {
	tests := []struct {
		name  string
		over  ConfigOverrides
		input config.Config
		want  config.Config
	}{
		{
			name: "non-zero values win",
			over: ConfigOverrides{
				FirstName:        "Ada",
				Email:            "ada@example.com",
				GmailAppPassword: "sekret",
				InboxScanMinutes: 30,
				OutreachConsent:  true,
			},
			input: config.Config{FirstName: "Old", InboxScanMinutes: 60},
			want: config.Config{FirstName: "Ada", Email: "ada@example.com",
				GmailAppPassword: "sekret", InboxScanMinutes: 30, OutreachConsent: true},
		},
		{
			name:  "zero values never overwrite",
			over:  ConfigOverrides{FirstName: "", InboxScanMinutes: 0, OutreachConsent: false},
			input: config.Config{FirstName: "Keep", InboxScanMinutes: 60, OutreachConsent: true},
			want:  config.Config{FirstName: "Keep", InboxScanMinutes: 60, OutreachConsent: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input
			tt.over.ApplyTo(&got)
			if got.FirstName != tt.want.FirstName || got.Email != tt.want.Email ||
				got.GmailAppPassword != tt.want.GmailAppPassword ||
				got.InboxScanMinutes != tt.want.InboxScanMinutes ||
				got.OutreachConsent != tt.want.OutreachConsent {
				t.Errorf("ApplyTo() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
