package outreach

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// sentLogger is an optional audit hook (set via SetSentLogger) that records
// every send attempt — success and failure — to the outreach log.
var sentLogger func(store.OutreachLogEntry)

// SetSentLogger registers the audit hook called after every send attempt.
// Call once at startup (TUI and CLI both wire this to the store).
func SetSentLogger(fn func(store.OutreachLogEntry)) { sentLogger = fn }

func logAttempt(it Item, status Status, err error) {
	if sentLogger == nil {
		return
	}
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	entry := store.OutreachLogEntry{
		Channel:       string(it.Channel),
		JobURL:        it.JobURL,
		Company:       it.Company,
		Role:          it.Role,
		ContactName:   it.ContactName,
		ContactEmail:  it.ContactEmail,
		ContactSource: it.ContactSource,
		Subject:       it.Subject,
		Body:          it.Body,
		Status:        string(status),
		Error:         errStr,
		ReviewScore:   it.ReviewScore,
		Attempts:      it.Attempts,
		CreatedAt:     time.Now(),
	}
	if status == StatusSent || status == StatusOpened {
		entry.SentAt = time.Now()
	}
	func() {
		defer func() { _ = recover() }() // audit must never crash a send
		sentLogger(entry)
	}()
}

// SendEmail delivers one email item through the user's Gmail.
// Transport: Gmail API with the OAuth token when configured, otherwise
// Gmail SMTP with the app password. Both send from the user's own address.
func SendEmail(cfg *config.Config, item Item) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	if !cfg.OutreachConsent {
		return fmt.Errorf("outreach consent required")
	}
	from := strings.TrimSpace(cfg.Email)
	to := strings.TrimSpace(item.ContactEmail)
	if from == "" {
		return fmt.Errorf("set your Email in Config → Personal")
	}
	if !HasGmailOAuth(cfg) && strings.TrimSpace(cfg.GmailAppPassword) == "" {
		return fmt.Errorf("set a Gmail App Password in Config → Outreach, or run nexus-gmailauth to connect a Gmail token")
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
	raw := buildRFC822(from, to, subj, item.Body)

	var sendErr error
	if HasGmailOAuth(cfg) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		sendErr = sendViaGmailAPI(ctx, cfg, raw)
	} else {
		auth := smtp.PlainAuth("", from, strings.TrimSpace(cfg.GmailAppPassword), "smtp.gmail.com")
		if err := smtp.SendMail("smtp.gmail.com:587", auth, from, []string{to}, []byte(raw)); err != nil {
			sendErr = fmt.Errorf("smtp send: %w", err)
		}
	}
	if sendErr != nil {
		item.Status = StatusFailed
		item.Error = sendErr.Error()
		_ = Upsert(item)
		logAttempt(item, StatusFailed, sendErr)
		return sendErr
	}
	item.SentAt = time.Now()
	item.Error = ""
	// Advance the follow-up sequence (or close it when follow-ups are off /
	// exhausted). ScheduleAfterSend sets the terminal status accordingly.
	ScheduleAfterSend(cfg, &item, item.SentAt)
	if err := Upsert(item); err != nil {
		return err
	}
	logAttempt(item, StatusSent, nil)
	return nil
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
	if err := Upsert(item); err != nil {
		return err
	}
	logAttempt(item, StatusSent, nil)
	return nil
}
