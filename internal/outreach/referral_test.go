package outreach

import (
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestEmailTemplates_VariantSelection(t *testing.T) {
	tests := []struct {
		name             string
		cfg              *config.Config
		wantReferral     bool
		wantSubjectCheck string // substring the subject should contain
	}{
		{"standard by default", &config.Config{}, false, "Quick note"},
		{"referral when enabled", &config.Config{OutreachReferralAsk: true}, true, "Referral"},
		{"custom standard subject", &config.Config{EmailSubjectTpl: "Custom {{role}}"}, false, "Custom"},
		{"custom referral subject", &config.Config{OutreachReferralAsk: true, ReferralSubjectTpl: "My referral {{role}}"}, true, "My referral"},
		{"custom referral body", &config.Config{OutreachReferralAsk: true, ReferralBodyTpl: "Body X {{role}}"}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subj, body := emailTemplates(tt.cfg)
			if tt.wantReferral && !strings.Contains(strings.ToLower(subj), "referral") {
				t.Errorf("expected a referral subject; got %q", subj)
			}
			if tt.wantSubjectCheck != "" && !strings.Contains(subj, tt.wantSubjectCheck) {
				t.Errorf("subject = %q; want it to contain %q", subj, tt.wantSubjectCheck)
			}
			if body == "" {
				t.Error("body template must never be empty")
			}
		})
	}
}

func TestNewEmailDraft_ReferralVariant(t *testing.T) {
	job := JobRef{URL: "https://example.com/j", Company: "Acme", Role: "Backend Engineer"}

	standard := NewEmailDraft(&config.Config{}, job, "Jane", "jane@acme.com")
	if strings.Contains(standard.Subject, "Referral") {
		t.Errorf("standard draft subject = %q; should not be a referral", standard.Subject)
	}
	if !strings.Contains(standard.Body, "recently applied") {
		t.Errorf("standard draft body = %q; want the direct-interest template", standard.Body)
	}

	referral := NewEmailDraft(&config.Config{OutreachReferralAsk: true}, job, "Jane", "jane@acme.com")
	if !strings.Contains(referral.Subject, "Referral") {
		t.Errorf("referral draft subject = %q; want Referral prefix", referral.Subject)
	}
	if !strings.Contains(referral.Body, "referral") {
		t.Errorf("referral draft body = %q; want referral ask", referral.Body)
	}
	if referral.Status != StatusReady {
		t.Errorf("with a contact email the draft should be ready; got %s", referral.Status)
	}
}

func TestApplyTemplate_ReferralVariant(t *testing.T) {
	cfg := &config.Config{OutreachReferralAsk: true}
	it := &Item{Company: "Acme", Role: "Backend Engineer"}
	in := ComposeInput{Company: "Acme", Role: "Backend Engineer", ContactName: "Jane", FullName: "Pat", Headline: "engineer"}
	applyTemplate(cfg, it, in)
	if !strings.Contains(it.Subject, "Referral") {
		t.Errorf("applyTemplate subject = %q; want referral", it.Subject)
	}
	if !strings.Contains(it.Body, "Jane") || !strings.Contains(it.Body, "Pat") {
		t.Errorf("applyTemplate body = %q; want rendered contact + sender", it.Body)
	}
	if strings.Contains(it.Body, "{{") {
		t.Errorf("body still has unresolved placeholders: %q", it.Body)
	}
}
