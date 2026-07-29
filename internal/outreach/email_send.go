package outreach

import (
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/manthanmanthan/nexus/internal/config"
)

// SendEmail sends via Gmail SMTP using the app password in config.
func SendEmail(cfg *config.Config, item Item) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	if !cfg.OutreachConsent {
		return fmt.Errorf("outreach consent required")
	}
	from := strings.TrimSpace(cfg.Email)
	pass := strings.TrimSpace(cfg.GmailAppPassword)
	to := strings.TrimSpace(item.ContactEmail)
	if from == "" || pass == "" {
		return fmt.Errorf("set your Email and Gmail App Password in Config")
	}
	if to == "" {
		return fmt.Errorf("contact email is empty — add an address before sending")
	}
	max := cfg.MaxEmailsPerDay
	if max <= 0 {
		max = 10
	}
	if n, err := CountSentToday(ChannelEmail); err == nil && n >= max {
		return fmt.Errorf("daily email cap reached (%d)", max)
	}

	subj := strings.TrimSpace(item.Subject)
	if subj == "" {
		subj = "Hello"
	}
	msg := strings.Builder{}
	msg.WriteString("From: " + from + "\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + subj + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(item.Body)

	auth := smtp.PlainAuth("", from, pass, "smtp.gmail.com")
	addr := "smtp.gmail.com:587"
	if err := smtp.SendMail(addr, auth, from, []string{to}, []byte(msg.String())); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	item.Status = StatusSent
	item.SentAt = time.Now()
	item.Error = ""
	return Upsert(item)
}

// MarkLinkedInSent records an assisted LinkedIn send (user copied/sent manually).
func MarkLinkedInSent(cfg *config.Config, item Item) error {
	if cfg != nil && !cfg.OutreachConsent {
		return fmt.Errorf("outreach consent required")
	}
	max := 10
	if cfg != nil && cfg.MaxLinkedInPerDay > 0 {
		max = cfg.MaxLinkedInPerDay
	}
	if n, err := CountSentToday(ChannelLinkedIn); err == nil && n >= max {
		return fmt.Errorf("daily LinkedIn cap reached (%d)", max)
	}
	item.Status = StatusSent
	item.SentAt = time.Now()
	item.Error = ""
	return Upsert(item)
}
