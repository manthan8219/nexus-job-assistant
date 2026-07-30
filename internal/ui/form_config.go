package ui

// Package ui — form_config.go
// Config serialization for the Config form: building config.Config from form
// state, the auto-save command, provider-key assembly, and value getters for
// the custom (non-textinput) widgets.

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/geo"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
)

// buildProviderKeys assembles the provider key map for config, omitting empty values.
func buildProviderKeys(linkedIn, indeed string) map[string]string {
	m := make(map[string]string)
	if strings.TrimSpace(linkedIn) != "" {
		m["linkedin"] = strings.TrimSpace(linkedIn)
	}
	if strings.TrimSpace(indeed) != "" {
		m["indeed"] = strings.TrimSpace(indeed)
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// toConfig builds a config.Config snapshot from the current form state.
func (m FormModel) toConfig() *config.Config {
	cur := currencies[m.currencyIdx]
	salary := ""
	if m.salaryPreset >= 0 && m.salaryPreset < len(cur.Presets) {
		salary = strconv.Itoa(cur.Presets[m.salaryPreset])
	} else if m.salaryCustom != "" {
		salary = m.salaryCustom
	}
	return &config.Config{
		FirstName:              m.inputs[fFirstName].Value(),
		LastName:               m.inputs[fLastName].Value(),
		Email:                  m.inputs[fEmail].Value(),
		Phone:                  m.inputs[fPhone].Value(),
		LinkedInID:             m.inputs[fLinkedInID].Value(),
		ResumePath:             m.inputs[fResumePath].Value(),
		City:                   m.inputs[fCity].Value(),
		YearsOfExperience:      m.inputs[fYearsExp].Value(),
		TargetJobTitles:        strings.Join(m.jobTitleTags, ", "),
		JobIntent:              m.jobIntent,
		WorkType:               m.workTypeValue(),
		TargetLocations:        geo.JoinLocationTags(m.locationTags),
		Currency:               cur.Code,
		MinSalary:              salary,
		ProviderKeys:           buildProviderKeys(m.inputs[fLinkedInKey].Value(), m.inputs[fIndeedKey].Value()),
		AIAssist:               m.aiAssist,
		AIProvider:             m.aiProviderValue(),
		AnthropicKey:           m.inputs[fAnthropicKey].Value(),
		OpenAIKey:              m.inputs[fOpenAIKey].Value(),
		LocalLLMURL:            m.inputs[fLocalLLMURL].Value(),
		LocalLLMModel:          m.inputs[fLocalLLMModel].Value(),
		GmailAppPassword:       m.inputs[fGmailPassword].Value(),
		HunterKey:              m.inputs[fHunterKey].Value(),
		ApolloKey:              m.inputs[fApolloKey].Value(),
		NotifyChannels:         m.notifyChannelValue(),
		DiscordWebhookURL:      m.inputs[fDiscordWebhook].Value(),
		TelegramBotToken:       m.inputs[fTelegramBotToken].Value(),
		TelegramChatID:         m.inputs[fTelegramChatID].Value(),
		ApplyConsent:           m.applyConsent,
		ApplyConsentAt:         m.applyConsentAt,
		MaxAppsPerRun:          parsePositiveInt(m.inputs[fMaxPerRun].Value(), 10),
		MaxAppsPerDay:          parsePositiveInt(m.inputs[fMaxPerDay].Value(), 25),
		ApplyDelaySec:          parsePositiveInt(m.inputs[fApplyDelaySec].Value(), 3),
		CompanyBlocklist:       strings.TrimSpace(m.inputs[fCompanyBlocklist].Value()),
		WorkAuth:               m.workAuth,
		NoticePeriodDays:       strings.TrimSpace(m.inputs[fNoticePeriod].Value()),
		OfficeDaysPerWeek:      strings.TrimSpace(m.inputs[fOfficeDays].Value()),
		CoverLetterMode:        m.coverLetterMode,
		CoverLetterText:        m.inputs[fCoverLetterText].Value(),
		OutreachConsent:        m.outreachConsent,
		OutreachConsentAt:      m.outreachConsentAt,
		OutreachMode:           m.outreachMode,
		MaxEmailsPerDay:        m.maxEmailsPerDay,
		MaxLinkedInPerDay:      m.maxLinkedInPerDay,
		EmailSubjectTpl:        m.emailSubjectTpl,
		EmailBodyTpl:           m.emailBodyTpl,
		LinkedInMsgTpl:         m.linkedinMsgTpl,
		LinkedInMode:           m.linkedinMode,
		LinkedInSessionCookie:  m.linkedinSessionCookie,
		OutreachAutoQueue:      m.outreachAutoQueue,
		OutreachAICompose:      m.outreachAICompose,
		OutreachAIReview:       m.outreachAIReview,
		OutreachGenModel:       m.outreachGenModel,
		OutreachCheckModel:     m.outreachCheckModel,
		OutreachMinScore:       m.outreachMinScore,
		OutreachMaxRetries:     m.outreachMaxRetries,
		OutreachSMTPVerify:     m.outreachSMTPVerify,
		GmailOAuthClientID:     m.gmailOAuthClientID,
		GmailOAuthClientSecret: m.gmailOAuthClientSecret,
		GmailOAuthRefreshToken: m.gmailOAuthRefreshToken,
	}
}

// saveCmd returns a command that writes the current form state to config.json.
// It preserves externally-managed Gmail OAuth tokens that nexus-gmailauth may
// have written while the TUI is running, so a stale in-memory form never wipes
// them on save.
func (m FormModel) saveCmd() tea.Cmd {
	complete := m.IsComplete()
	return func() tea.Msg {
		cfg := m.toConfig()
		// Preserve externally-managed secrets: nexus-gmailauth writes Gmail
		// OAuth tokens straight to config.json while the TUI is running, so a
		// stale in-memory form must not wipe them on the next save.
		if cur, err := config.Load(); err == nil && cur != nil {
			if strings.TrimSpace(cfg.GmailOAuthRefreshToken) == "" {
				cfg.GmailOAuthRefreshToken = cur.GmailOAuthRefreshToken
			}
			if strings.TrimSpace(cfg.GmailOAuthClientID) == "" {
				cfg.GmailOAuthClientID = cur.GmailOAuthClientID
			}
			if strings.TrimSpace(cfg.GmailOAuthClientSecret) == "" {
				cfg.GmailOAuthClientSecret = cur.GmailOAuthClientSecret
			}
		}
		if err := config.Save(cfg); err != nil {
			return ErrMsg{err}
		}
		if complete {
			return ProfileCompleteMsg{Cfg: cfg}
		}
		return SavedMsg{Cfg: cfg}
	}
}

// workTypeValue joins the selected work-type options into a comma-separated string.
func (m FormModel) workTypeValue() string {
	var parts []string
	for i, sel := range m.wtSelected {
		if sel {
			parts = append(parts, wtOptions[i])
		}
	}
	return strings.Join(parts, ", ")
}

// aiProviderValue returns the configured AI backend ("local", "api") or "" when
// AI Assist is off.
func (m FormModel) aiProviderValue() string {
	if !m.aiAssist {
		return ""
	}
	if m.aiProvider == "api" {
		return "api"
	}
	return "local"
}

// notifyChannelValue returns the IDs of the selected notification channels,
// resolved against the notifier registry at call time.
func (m FormModel) notifyChannelValue() []string {
	avail := notifier.Available()
	var out []string
	for i, sel := range m.ncSelected {
		if sel && i < len(avail) {
			out = append(out, avail[i].ID)
		}
	}
	return out
}
