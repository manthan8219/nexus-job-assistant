package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	// Personal
	FirstName          string `json:"first_name"`
	LastName           string `json:"last_name"`
	Email              string `json:"email"`
	Phone              string `json:"phone"`
	LinkedInID         string `json:"linkedin_id"`
	ResumePath         string `json:"resume_path"`
	City               string `json:"city"`
	YearsOfExperience  string `json:"years_of_experience"`

	// Job Preferences
	TargetJobTitles    string `json:"target_job_titles"`   // comma-separated
	JobIntent          string `json:"job_intent,omitempty"` // free-text: what kind of job they want
	WorkType           string `json:"work_type"`           // Remote / Onsite / Hybrid
	TargetLocations    string `json:"target_locations"`    // comma-separated
	Currency           string `json:"currency"`            // USD, INR, EUR, GBP, CAD, AUD, SGD
	MinSalary          string `json:"min_salary"`          // raw number

	// Provider API keys — optional, activates providers that require auth
	// Keys: "linkedin", "indeed", etc.
	ProviderKeys       map[string]string `json:"provider_keys,omitempty"`

	// AI
	// AIAssist enables AI-powered improvements (cover letters, answers, matching, etc.).
	AIAssist   bool   `json:"ai_assist,omitempty"`
	// AIProvider is "local" (Ollama etc.) or "api" (Anthropic/OpenAI). Empty when AIAssist is off.
	AIProvider string `json:"ai_provider,omitempty"`
	AnthropicKey  string `json:"anthropic_key,omitempty"`
	OpenAIKey     string `json:"openai_key,omitempty"`
	LocalLLMURL   string `json:"local_llm_url,omitempty"`
	LocalLLMModel string `json:"local_llm_model,omitempty"`

	// Notifications
	// NotifyChannels lists which channels receive job-apply notifications.
	// Channel IDs from notifier.Available(). Empty = all configured channels fire.
	NotifyChannels    []string `json:"notify_channels,omitempty"`
	DiscordWebhookURL string   `json:"discord_webhook_url,omitempty"`
	TelegramBotToken  string `json:"telegram_bot_token,omitempty"` // Bot token from @BotFather
	TelegramChatID    string `json:"telegram_chat_id,omitempty"`   // Chat/channel ID to send to
	GmailAppPassword string `json:"gmail_app_password,omitempty"` // Gmail SMTP app password (16-char)
	HunterKey        string `json:"hunter_key,omitempty"`         // Hunter.io — find recruiter emails
	ApolloKey        string `json:"apollo_key,omitempty"`          // Apollo.io — contact database

	// Apply Safety — consent + rate limits (required for responsible auto-apply)
	// ApplyConsent means the user acknowledged Nexus may submit applications on their behalf.
	ApplyConsent bool   `json:"apply_consent,omitempty"`
	ApplyConsentAt string `json:"apply_consent_at,omitempty"` // RFC3339 when consent was given
	MaxAppsPerRun int    `json:"max_apps_per_run,omitempty"` // 0 = default 10
	MaxAppsPerDay int    `json:"max_apps_per_day,omitempty"` // 0 = default 25
	ApplyDelaySec int    `json:"apply_delay_sec,omitempty"`  // pause between real applies; 0 = default 3
	CompanyBlocklist string `json:"company_blocklist,omitempty"` // comma-separated company names to skip
	// WorkAuth: authorized | citizen | need_sponsorship | unspecified
	WorkAuth string `json:"work_auth,omitempty"`
	// NoticePeriodDays and OfficeDaysPerWeek answer the two custom
	// screening questions that came up repeatedly in real applications
	// (Lever/Greenhouse "notice period?" and "days per week in office?")
	// but had no home in the profile — free text since notice periods
	// are sometimes described as "immediate" rather than a number.
	NoticePeriodDays  string `json:"notice_period_days,omitempty"`
	OfficeDaysPerWeek string `json:"office_days_per_week,omitempty"`
	// CoverLetterMode: off | template | ai
	CoverLetterMode string `json:"cover_letter_mode,omitempty"`
	CoverLetterText string `json:"cover_letter_text,omitempty"` // used when mode=template

	// Career Scraper — comma-separated Name:URL pairs for career page targets
	ScraperTargets string `json:"scraper_targets,omitempty"`

	// Outreach — email + LinkedIn follow-up after apply/queue
	OutreachConsent   bool   `json:"outreach_consent,omitempty"`
	OutreachConsentAt string `json:"outreach_consent_at,omitempty"`
	MaxEmailsPerDay   int    `json:"max_emails_per_day,omitempty"`    // 0 = default 10
	MaxLinkedInPerDay int    `json:"max_linkedin_per_day,omitempty"`  // 0 = default 10
	EmailSubjectTpl   string `json:"email_subject_tpl,omitempty"`
	EmailBodyTpl      string `json:"email_body_tpl,omitempty"`
	LinkedInMsgTpl    string `json:"linkedin_msg_tpl,omitempty"`
	// OutreachMode: confirm (ask before each) | queue (generate batch, tap send repeatedly) | auto (send/open all)
	OutreachMode string `json:"outreach_mode,omitempty"`
	// LinkedInMode kept for older configs; prefer OutreachMode. Values: confirm|queue|auto (legacy: assisted|autosend).
	LinkedInMode string `json:"linkedin_mode,omitempty"`
	// LinkedInSessionCookie reserved for deeper LinkedIn automation (optional).
	LinkedInSessionCookie string `json:"linkedin_session_cookie,omitempty"`
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nexus"), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// NotifyFields returns all notification-related fields.
func (c *Config) NotifyFields() (discordURL, tgToken, tgChatID string, channels []string) {
	return c.DiscordWebhookURL, c.TelegramBotToken, c.TelegramChatID, c.NotifyChannels
}
