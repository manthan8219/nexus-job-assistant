package ui

import (
	"github.com/manthanmanthan/nexus/internal/config"
	"github.com/manthanmanthan/nexus/internal/outreach"
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
}

// ApplyOutreachSetup merges Outreach → Setup fields into the form (for config save).
func (m *FormModel) ApplyOutreachSetup(consent bool, consentAt string, maxEmail, maxLI int, mode, liCookie string) {
	m.outreachConsent = consent
	m.outreachConsentAt = consentAt
	m.maxEmailsPerDay = maxEmail
	m.maxLinkedInPerDay = maxLI
	m.outreachMode = outreach.NormalizeMode(mode)
	m.linkedinMode = m.outreachMode // keep legacy field in sync
	m.linkedinSessionCookie = liCookie
}
