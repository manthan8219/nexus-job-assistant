package ui

// Package ui — form_fields.go
// Static form metadata for the Config screen: field index constants, labels,
// placeholders, section grouping, the provider/key catalogue, work-type and
// currency/salary preset tables, and the shared label/tag styles.

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// ── Field indices ─────────────────────────────────────────────────────────────

const (
	fFirstName = iota
	fLastName
	fEmail
	fPhone
	fLinkedInID
	fResumePath
	fCity
	fYearsExp
	fJobTitles        // tag input
	fWorkType         // checkbox
	fLocations        // tag input
	fCurrency         // cycle select
	fMinSalary        // preset picker
	fLinkedInKey      // provider key — LinkedIn
	fIndeedKey        // provider key — Indeed
	fAIAssist         // AI — master toggle: use AI to improve quality
	fAIProvider       // AI — local LLM vs cloud API keys
	fAnthropicKey     // AI — Claude
	fOpenAIKey        // AI — OpenAI
	fLocalLLMURL      // AI — local LLM endpoint
	fLocalLLMModel    // AI — local LLM model name
	fGmailPassword    // Outreach — Gmail app password (SMTP)
	fHunterKey        // Outreach — Hunter.io (find recruiter emails)
	fApolloKey        // Outreach — Apollo.io (contact database)
	fDiscordWebhook   // Integrations — Discord webhook URL
	fTelegramBotToken // Integrations — Telegram bot token
	fTelegramChatID   // Integrations — Telegram chat ID
	fNotifyChannels   // Integrations — notify channel selector (custom widget)
	fApplyConsent     // Apply Safety — consent to submit applications
	fMaxPerRun        // Apply Safety — max apps per engine run
	fMaxPerDay        // Apply Safety — max successful applies per day
	fApplyDelaySec    // Apply Safety — seconds between real applies
	fCompanyBlocklist // Apply Safety — comma-separated companies to skip
	fWorkAuth         // Apply Safety — work authorization
	fNoticePeriod     // Apply Safety — notice period, answers common screening questions
	fOfficeDays       // Apply Safety — days/week willing to work onsite
	fCoverLetterMode  // Apply Safety — off | template | ai
	fCoverLetterText  // Apply Safety — template body
	fScraperTargets   // Career Scraper — comma-separated company:url pairs
	fieldCount
)

var fieldLabels = [fieldCount]string{
	"First Name",
	"Last Name",
	"Email",
	"Phone",
	"LinkedIn ID",
	"Resume Path",
	"City",
	"Years of Experience",
	"Job Titles (AI)",
	"Work Type",
	"Target Locations",
	"Currency",
	"Min Salary",
	"LinkedIn API Token",
	"Indeed API Key",
	"Use AI Assist",
	"AI Backend",
	"Anthropic API Key",
	"OpenAI API Key",
	"Local LLM URL",
	"Local Model",
	"Gmail App Password",
	"Hunter.io API Key",
	"Apollo.io API Key",
	"Discord Webhook URL",
	"Telegram Bot Token",
	"Telegram Chat ID",
	"Notify on Apply",
	"Apply Consent",
	"Max Apps Per Run",
	"Max Apps Per Day",
	"Delay Between Applies (sec)",
	"Company Blocklist",
	"Work Authorization",
	"Notice Period",
	"Days/Week in Office",
	"Cover Letter",
	"Cover Letter Template",
	"Career Page Targets",
}

var fieldPlaceholders = [fieldCount]string{
	"John", "Doe", "john@example.com", "+1 555 000 0000",
	"johndoe", "/Users/john/resume.pdf", "San Francisco", "3",
	"describe the job you want…", // AI expands · enter generates when AI on
	"",                           // WorkType — custom widget
	"type city — pick from list", // geo autocomplete
	"",                           // Currency — custom widget
	"",                           // MinSalary — custom widget
	"paste token here",           // LinkedIn API token
	"paste key here",             // Indeed API key
	"",                           // AI Assist — custom yes/no
	"",                           // AI Backend — custom local/api
	"sk-ant-...",                 // Anthropic API key
	"sk-...",                     // OpenAI API key
	"http://localhost:11434",     // Local LLM URL
	"llama3",                     // Local LLM model name
	"xxxx xxxx xxxx xxxx",        // Gmail app password (16-char)
	"paste key here",             // Hunter.io API key
	"paste key here",             // Apollo.io API key
	"https://discord.com/api/webhooks/...",
	"1234567890:ABCDEFGabcdefg...", // from @BotFather
	"-1001234567890",               // chat/group/channel ID
	"",                             // notify channels — custom widget
	"",                             // apply consent — custom
	"10",
	"25",
	"3",
	"Acme Staffing, Example Corp",
	"",
	"30, or Immediate",
	"3",
	"",
	"I am excited to apply for…",
	"Stripe:https://stripe.com/jobs, Linear:https://linear.app/careers",
}

var formSections = []struct {
	title string
	start int
}{
	{"Personal Information", fFirstName},
	{"Job Preferences", fJobTitles},
	{"Provider Keys", fLinkedInKey},
	{"AI Configuration", fAIAssist},
	{"Outreach", fGmailPassword},
	{"Integrations", fDiscordWebhook},
	{"Apply Safety", fApplyConsent},
	{"Career Scraper", fScraperTargets},
}

// providerStatus describes a job board provider shown in the Providers section.
type providerStatus struct {
	name     string
	keyField int    // -1 = no key required (always active)
	cfgKey   string // key in config.ProviderKeys
}

// allProviders lists every provider and whether it needs a user key.
// Providers with keyField == -1 are always active (no user key needed).
var allProviders = []providerStatus{
	{"Greenhouse", -1, ""},
	{"Ashby", -1, ""},
	{"SmartRecruiters", -1, ""},
	{"Lever", -1, ""},
	{"Workable", -1, ""},
	{"RemoteOK", -1, ""},
	{"Remotive", -1, ""},
	{"Arbeitnow", -1, ""},
	{"Jobicy", -1, ""},
	{"HackerNews", -1, ""},
	{"Workday", -1, ""},
	{"LinkedIn", fLinkedInKey, "linkedin"},
	{"Indeed", fIndeedKey, "indeed"},
}

// ── Work type ─────────────────────────────────────────────────────────────────

var wtOptions = [3]string{"Remote", "Onsite", "Hybrid"}

// ── Currency + salary presets ─────────────────────────────────────────────────

type currencyDef struct {
	Code    string
	Symbol  string
	Presets []int
}

var currencies = []currencyDef{
	{"USD", "$", []int{60000, 80000, 100000, 120000, 150000, 200000}},
	{"INR", "₹", []int{500000, 800000, 1200000, 1500000, 2000000, 3000000}},
	{"EUR", "€", []int{50000, 70000, 90000, 110000, 130000, 160000}},
	{"GBP", "£", []int{50000, 70000, 90000, 110000, 130000, 150000}},
	{"CAD", "CA$", []int{70000, 90000, 110000, 130000, 150000, 180000}},
	{"AUD", "A$", []int{70000, 90000, 110000, 130000, 150000, 180000}},
	{"SGD", "S$", []int{80000, 100000, 120000, 150000, 180000, 220000}},
}

func formatSalary(c currencyDef, amount int) string {
	if c.Code == "INR" {
		// Show in lakhs
		l := float64(amount) / 100000
		if l == float64(int(l)) {
			return fmt.Sprintf("%s%.0fL", c.Symbol, l)
		}
		return fmt.Sprintf("%s%.1fL", c.Symbol, l)
	}
	// Show in thousands
	k := amount / 1000
	return fmt.Sprintf("%s%dk", c.Symbol, k)
}

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPurpleMuted)).
			MarginTop(1)

	labelActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorPurple)).
				Bold(true)

	labelInactiveStyle = lipgloss.NewStyle().
				Foreground(textSecondary)

	savedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGreen)).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorRed)).
			Bold(true)

	// Inline tag — uses text characters not lipgloss border (stays single-line)
	tagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPurple))

	tagRemoveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGrey))
)
