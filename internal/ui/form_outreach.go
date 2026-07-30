package ui

import (
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
)

func (m *FormModel) initOutreachFromCfg(cfg *config.Config) {
	if cfg == nil {
		return
	}
	m.outreachConsent = cfg.OutreachConsent
	m.outreachConsentAt = cfg.OutreachConsentAt
	m.maxEmailsPerDay = cfg.MaxEmailsPerDay
	m.maxLinkedInPerDay = cfg.MaxLinkedInPerDay
	m.emailSubjectTpl = cfg.EmailSubjectTpl
	m.emailBodyTpl = cfg.EmailBodyTpl
	m.linkedinMsgTpl = cfg.LinkedInMsgTpl
	m.outreachMode = outreach.EffectiveMode(cfg)
	m.linkedinMode = m.outreachMode
	m.linkedinSessionCookie = cfg.LinkedInSessionCookie
	m.outreachAutoQueue = cfg.OutreachAutoQueue
	m.outreachAICompose = cfg.OutreachAICompose
	m.outreachAIReview = cfg.OutreachAIReview
	m.outreachGenModel = cfg.OutreachGenModel
	m.outreachCheckModel = cfg.OutreachCheckModel
	m.outreachMinScore = cfg.OutreachMinScore
	m.outreachMaxRetries = cfg.OutreachMaxRetries
	m.outreachSMTPVerify = cfg.OutreachSMTPVerify
	m.gmailOAuthClientID = cfg.GmailOAuthClientID
	m.gmailOAuthClientSecret = cfg.GmailOAuthClientSecret
	m.gmailOAuthRefreshToken = cfg.GmailOAuthRefreshToken
}

// ApplyOutreachSetup merges Outreach → Setup fields into the form (for config save).
func (m *FormModel) ApplyOutreachSetup(s OutreachSetupSaveMsg) {
	m.outreachConsent = s.Consent
	m.outreachConsentAt = s.ConsentAt
	m.maxEmailsPerDay = s.MaxEmail
	m.maxLinkedInPerDay = s.MaxLI
	m.outreachMode = outreach.NormalizeMode(s.LIMode)
	m.linkedinMode = m.outreachMode // keep legacy field in sync
	m.linkedinSessionCookie = s.LICookie
	m.outreachAutoQueue = s.AutoQueue
	m.outreachAICompose = s.AICompose
	m.outreachAIReview = s.AIReview
	m.outreachGenModel = s.GenModel
	m.outreachCheckModel = s.CheckModel
	m.outreachMinScore = s.MinScore
	m.outreachMaxRetries = s.MaxRetries
	m.outreachSMTPVerify = s.SMTPVerify
}
