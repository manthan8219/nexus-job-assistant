package outreach

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

type JobRef struct {
	URL      string
	Company  string
	Role     string
	Provider string
}

func RenderTemplate(tpl string, vars map[string]string) string {
	out := tpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	// Clean leftover empty contact name
	out = strings.ReplaceAll(out, "Hi ,", "Hi,")
	out = strings.ReplaceAll(out, "Hi  —", "Hi —")
	return strings.TrimSpace(out)
}

func Vars(cfg *config.Config, job JobRef, contactName, contactEmail string) map[string]string {
	full := ""
	headline := "software engineer"
	li := ""
	if cfg != nil {
		full = strings.TrimSpace(cfg.FirstName + " " + cfg.LastName)
		if cfg.YearsOfExperience != "" {
			headline = "engineer with " + cfg.YearsOfExperience + "+ years experience"
		}
		if cfg.LinkedInID != "" {
			li = "https://linkedin.com/in/" + cfg.LinkedInID
		}
	}
	if contactName == "" {
		contactName = "there"
	}
	return map[string]string{
		"contact_name":  contactName,
		"contact_email": contactEmail,
		"company":       job.Company,
		"role":          job.Role,
		"provider":      job.Provider,
		"full_name":     full,
		"headline":      headline,
		"linkedin":      li,
	}
}

func NewEmailDraft(cfg *config.Config, job JobRef, contactName, contactEmail string) Item {
	subjTpl := DefaultEmailSubject()
	bodyTpl := DefaultEmailBody()
	if cfg != nil {
		if strings.TrimSpace(cfg.EmailSubjectTpl) != "" {
			subjTpl = cfg.EmailSubjectTpl
		}
		if strings.TrimSpace(cfg.EmailBodyTpl) != "" {
			bodyTpl = cfg.EmailBodyTpl
		}
	}
	vars := Vars(cfg, job, contactName, contactEmail)
	now := time.Now()
	st := StatusDraft
	if strings.TrimSpace(contactEmail) != "" {
		st = StatusReady
	}
	return Item{
		ID:           uuid.NewString(),
		Channel:      ChannelEmail,
		JobURL:       job.URL,
		Company:      job.Company,
		Role:         job.Role,
		Provider:     job.Provider,
		ContactName:  contactName,
		ContactEmail: contactEmail,
		Subject:      RenderTemplate(subjTpl, vars),
		Body:         RenderTemplate(bodyTpl, vars),
		Status:       st,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func NewLinkedInDraft(cfg *config.Config, job JobRef, contactName, profileURL string) Item {
	tpl := DefaultLinkedInMsg()
	if cfg != nil && strings.TrimSpace(cfg.LinkedInMsgTpl) != "" {
		tpl = cfg.LinkedInMsgTpl
	}
	vars := Vars(cfg, job, contactName, "")
	now := time.Now()
	return Item{
		ID:          uuid.NewString(),
		Channel:     ChannelLinkedIn,
		JobURL:      job.URL,
		Company:     job.Company,
		Role:        job.Role,
		Provider:    job.Provider,
		ContactName: contactName,
		LinkedInURL: profileURL,
		Body:        RenderTemplate(tpl, vars),
		Status:      StatusDraft,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
