package outreach

import (
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// Check is one readiness row for the Setup UI.
type Check struct {
	OK      bool
	Label   string
	FixHint string
}

func EmailReady(cfg *config.Config) []Check {
	if cfg == nil {
		return []Check{{OK: false, Label: "Config", FixHint: "Open Config and fill your profile"}}
	}
	gmailAuth := strings.TrimSpace(cfg.GmailAppPassword) != "" || HasGmailOAuth(cfg)
	finderLabel := "Contact finding: careers@/hr@ patterns"
	if strings.TrimSpace(cfg.HunterKey) != "" || strings.TrimSpace(cfg.ApolloKey) != "" {
		finderLabel = "Contact finding: Hunter/Apollo + GitHub + OSINT + patterns"
	}
	return []Check{
		{cfg.OutreachConsent, "Outreach consent", "Turn on consent under Outreach → Setup"},
		{strings.TrimSpace(cfg.Email) != "", "Your email (From address)", "Set Email in Config → Personal"},
		{gmailAuth, "Gmail auth (app password or OAuth token)", "Config → Outreach → Gmail App Password, or run nexus-gmailauth"},
		{true, finderLabel, "Optional: add Hunter/Apollo keys in Config → Outreach for better recruiter emails"},
		{strings.TrimSpace(cfg.FirstName) != "" && strings.TrimSpace(cfg.LastName) != "", "Your name for signature", "Config → Personal Information"},
	}
}

func LinkedInReady(cfg *config.Config) []Check {
	if cfg == nil {
		return []Check{{OK: false, Label: "Config", FixHint: "Open Config and fill your profile"}}
	}
	mode := EffectiveMode(cfg)
	return []Check{
		{cfg.OutreachConsent, "Outreach consent", "Turn on consent under Outreach → Setup"},
		{strings.TrimSpace(cfg.LinkedInID) != "", "Your LinkedIn profile ID", "Config → Personal → LinkedIn ID"},
		{strings.TrimSpace(cfg.FirstName) != "", "Your name for messages", "Config → Personal Information"},
		{true, "Automation mode: " + ModeLabel(mode), "Change under Outreach → Setup"},
		{true, "Browser open", "Nexus opens LinkedIn people-search / messaging in your browser"},
	}
}

func AllOK(checks []Check) bool {
	for _, c := range checks {
		if !c.OK {
			return false
		}
	}
	return true
}

func DefaultEmailSubject() string {
	return "Quick note — {{role}} at {{company}}"
}

func DefaultEmailBody() string {
	return `Hi {{contact_name}},

I recently applied for {{role}} at {{company}} and wanted to reach out briefly.

I'm a {{headline}} and would welcome any guidance on the process or the team.

Thanks for your time,
{{full_name}}
{{linkedin}}`
}

func DefaultLinkedInMsg() string {
	return `Hi {{contact_name}} — I applied for {{role}} at {{company}}. Would appreciate any referral or tip on the hiring process. Thanks! — {{full_name}}`
}
