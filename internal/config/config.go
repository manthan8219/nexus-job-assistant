package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	// Personal
	FirstName         string   `json:"first_name"`
	LastName          string   `json:"last_name"`
	Email             string   `json:"email"`
	Phone             string   `json:"phone"`
	LinkedInID        string   `json:"linkedin_id"`
	ResumePath        string   `json:"resume_path"`
	City              string   `json:"city"`
	YearsOfExperience string   `json:"years_of_experience"`
	Skills            []string `json:"skills,omitempty"`

	// Job Preferences
	TargetJobTitles string `json:"target_job_titles"`    // comma-separated
	JobIntent       string `json:"job_intent,omitempty"` // free-text: what kind of job they want
	WorkType        string `json:"work_type"`            // Remote / Onsite / Hybrid
	TargetLocations string `json:"target_locations"`     // comma-separated
	Currency        string `json:"currency"`             // USD, INR, EUR, GBP, CAD, AUD, SGD
	MinSalary       string `json:"min_salary"`           // raw number

	// Provider API keys — optional, activates providers that require auth
	// Keys: "linkedin", "indeed", etc.
	ProviderKeys map[string]string `json:"provider_keys,omitempty"`

	// AI
	// AIAssist enables AI-powered improvements (cover letters, answers, matching, etc.).
	AIAssist bool `json:"ai_assist,omitempty"`
	// AIProvider is "local" (Ollama etc.) or "api" (Anthropic/OpenAI). Empty when AIAssist is off.
	AIProvider    string `json:"ai_provider,omitempty"`
	AnthropicKey  string `json:"anthropic_key,omitempty"`
	OpenAIKey     string `json:"openai_key,omitempty"`
	LocalLLMURL   string `json:"local_llm_url,omitempty"`
	LocalLLMModel string `json:"local_llm_model,omitempty"`

	// Notifications
	// NotifyChannels lists which channels receive job-apply notifications.
	// Channel IDs from notifier.Available(). Empty = all configured channels fire.
	NotifyChannels    []string `json:"notify_channels,omitempty"`
	DiscordWebhookURL string   `json:"discord_webhook_url,omitempty"`
	TelegramBotToken  string   `json:"telegram_bot_token,omitempty"` // Bot token from @BotFather
	TelegramChatID    string   `json:"telegram_chat_id,omitempty"`   // Chat/channel ID to send to
	GmailAppPassword  string   `json:"gmail_app_password,omitempty"` // Gmail SMTP app password (16-char)
	HunterKey         string   `json:"hunter_key,omitempty"`         // Hunter.io — find recruiter emails
	ApolloKey         string   `json:"apollo_key,omitempty"`         // Apollo.io — contact database

	// Apply Safety — consent + rate limits (required for responsible auto-apply)
	// ApplyConsent means the user acknowledged Nexus may submit applications on their behalf.
	ApplyConsent   bool   `json:"apply_consent,omitempty"`
	ApplyConsentAt string `json:"apply_consent_at,omitempty"` // RFC3339 when consent was given
	MaxAppsPerRun  int    `json:"max_apps_per_run,omitempty"` // 0 = default 10
	MaxAppsPerDay  int    `json:"max_apps_per_day,omitempty"` // 0 = default 25
	ApplyDelaySec  int    `json:"apply_delay_sec,omitempty"`  // pause between real applies; 0 = default 3
	// MinFitScore gates auto-apply: jobs scoring below it (when AI fit scoring
	// is active) are recorded as skipped instead of applied. 0 = off.
	MinFitScore      int    `json:"min_fit_score,omitempty"`
	CompanyBlocklist string `json:"company_blocklist,omitempty"` // comma-separated company names to skip
	// FreshJobPriority applies freshly-posted jobs before older ones within
	// each provider's search batch (newest PostedAt first). Off by default.
	FreshJobPriority bool `json:"fresh_job_priority,omitempty"`
	// StaleJobCutoffDays skips jobs whose posting date is older than this many
	// days instead of applying to them (0 = disabled). Jobs without a known
	// posting date are never skipped — we fail open rather than drop a listing
	// we cannot date.
	StaleJobCutoffDays int `json:"stale_job_cutoff_days,omitempty"`
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

	// Tailor — JD-tailored CV + cover letter with HR-agent review loop
	// TailorPerJob generates an HR-reviewed tailored kit for each application (engine hook).
	TailorPerJob bool `json:"tailor_per_job,omitempty"`
	// TailorMaxRounds caps the writer→HR review loop per kit (0 = default 3).
	TailorMaxRounds int `json:"tailor_max_rounds,omitempty"`

	// Career Scraper — comma-separated Name:URL pairs for career page targets
	ScraperTargets string `json:"scraper_targets,omitempty"`

	// Outreach — email + LinkedIn follow-up after apply/queue
	OutreachConsent   bool   `json:"outreach_consent,omitempty"`
	OutreachConsentAt string `json:"outreach_consent_at,omitempty"`
	MaxEmailsPerDay   int    `json:"max_emails_per_day,omitempty"`   // 0 = default 10
	MaxLinkedInPerDay int    `json:"max_linkedin_per_day,omitempty"` // 0 = default 10
	EmailSubjectTpl   string `json:"email_subject_tpl,omitempty"`
	EmailBodyTpl      string `json:"email_body_tpl,omitempty"`
	LinkedInMsgTpl    string `json:"linkedin_msg_tpl,omitempty"`
	// OutreachReferralAsk switches email drafts to the referral-ask variant
	// (asks the contact to point you at the right person or refer you) instead
	// of a direct-interest email. Off by default.
	OutreachReferralAsk bool `json:"outreach_referral_ask,omitempty"`
	// ReferralSubjectTpl overrides the referral-ask subject template.
	ReferralSubjectTpl string `json:"referral_subject_tpl,omitempty"`
	// ReferralBodyTpl overrides the referral-ask body template.
	ReferralBodyTpl string `json:"referral_body_tpl,omitempty"`
	// OutreachMode: confirm (ask before each) | queue (generate batch, tap send repeatedly) | auto (send/open all)
	OutreachMode string `json:"outreach_mode,omitempty"`
	// LinkedInMode kept for older configs; prefer OutreachMode. Values: confirm|queue|auto (legacy: assisted|autosend).
	LinkedInMode string `json:"linkedin_mode,omitempty"`
	// LinkedInSessionCookie reserved for deeper LinkedIn automation (optional).
	LinkedInSessionCookie string `json:"linkedin_session_cookie,omitempty"`

	// ── Outreach pipeline (auto find → AI draft → AI review → approve/send) ──
	// OutreachAutoQueue starts the email pipeline automatically whenever an
	// application is recorded (engine apply or queue build).
	OutreachAutoQueue bool `json:"outreach_auto_queue,omitempty"`
	// OutreachAICompose lets an LLM write the email (falls back to templates when off/unavailable).
	OutreachAICompose bool `json:"outreach_ai_compose,omitempty"`
	// OutreachAIReview lets a second LLM check email quality before it is marked ready.
	OutreachAIReview bool `json:"outreach_ai_review,omitempty"`
	// OutreachGenModel overrides the local model used to write emails (empty = LocalLLMModel).
	OutreachGenModel string `json:"outreach_gen_model,omitempty"`
	// OutreachCheckModel overrides the local model used to review emails (empty = same as generator).
	OutreachCheckModel string `json:"outreach_check_model,omitempty"`
	// OutreachMinScore is the 0-100 quality score the reviewer must give to pass (0 = default 70).
	OutreachMinScore int `json:"outreach_min_score,omitempty"`
	// OutreachMaxRetries caps regenerate→review loops per email (0 = default 3).
	OutreachMaxRetries int `json:"outreach_max_retries,omitempty"`
	// OutreachSMTPVerify enables SMTP probing of guessed pattern addresses (slow; off by default).
	OutreachSMTPVerify bool `json:"outreach_smtp_verify,omitempty"`
	// OutreachFollowUpsOff disables the automatic +3/+7/+14-day follow-up
	// sequence after each sent outreach email. Follow-ups are on by default
	// (they respect the same daily caps and stop on reply detection).
	OutreachFollowUpsOff bool `json:"outreach_follow_ups_off,omitempty"`
	// ReplyLookbackDays caps how far back reply detection scans the inbox
	// (0 = default 45 days).
	ReplyLookbackDays int `json:"reply_lookback_days,omitempty"`
	// OutreachBatchSize caps how many emails the auto-send loop fires in one
	// batch before pausing (0 = default 5). Batching never exceeds the daily cap.
	OutreachBatchSize int `json:"outreach_batch_size,omitempty"`
	// OutreachBatchPauseSec is the pause between auto-send batches (0 = default 60).
	OutreachBatchPauseSec int `json:"outreach_batch_pause_sec,omitempty"`

	// SMTP relay — send outreach from a custom domain instead of Gmail.
	// When SmtpRelayHost is set, SendEmail routes through this relay and Gmail
	// credentials are not required. The relay is expected to sign mail
	// (SPF/DKIM) for the From domain; credentials live only in config.
	SmtpRelayHost string `json:"smtp_relay_host,omitempty"`
	SmtpRelayPort int    `json:"smtp_relay_port,omitempty"` // 0 = default 587
	SmtpRelayUser string `json:"smtp_relay_user,omitempty"`
	SmtpRelayPass string `json:"smtp_relay_pass,omitempty"` // never logged
	SmtpRelayFrom string `json:"smtp_relay_from,omitempty"` // From address (defaults to Email)

	// Gmail OAuth — send from the user's Gmail via the Gmail API using a token
	// instead of an SMTP app password. Get a refresh token with: nexus-gmailauth
	// (see cmd/gmailauth). When GmailOAuthRefreshToken is set it takes
	// precedence over GmailAppPassword.
	GmailOAuthClientID     string `json:"gmail_oauth_client_id,omitempty"`
	GmailOAuthClientSecret string `json:"gmail_oauth_client_secret,omitempty"`
	GmailOAuthRefreshToken string `json:"gmail_oauth_refresh_token,omitempty"`

	// Automation — daily safe dry-run + email run updates.
	// DailyRunEnabled triggers one search-only run per day at DailyRunAt
	// (the web UI fires it while the dashboard is open; a future daemon can
	// run it while closed).
	DailyRunEnabled bool   `json:"daily_run_enabled,omitempty"`
	DailyRunAt      string `json:"daily_run_at,omitempty"` // "HH:MM" 24h
	// EmailNotifications opts into email run summaries (needs Email +
	// GmailAppPassword; delivered by the email notifier channel).
	EmailNotifications bool `json:"email_notifications,omitempty"`
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
