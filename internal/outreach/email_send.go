package outreach

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/osint"
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

// relayEndpoint resolves the optional custom-domain SMTP relay. Returns
// (addr "host:port", host, from, enabled). The port defaults to 587 and From
// falls back to the configured Email address. A nil config or empty host
// disables the relay path so Gmail sending is used unchanged.
func relayEndpoint(cfg *config.Config) (addr, host, from string, enabled bool) {
	if cfg == nil || strings.TrimSpace(cfg.SmtpRelayHost) == "" {
		return "", "", "", false
	}
	host = strings.TrimSpace(cfg.SmtpRelayHost)
	port := cfg.SmtpRelayPort
	if port <= 0 {
		port = 587
	}
	from = strings.TrimSpace(cfg.SmtpRelayFrom)
	if from == "" {
		from = strings.TrimSpace(cfg.Email)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), host, from, true
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
	relayAddr, relayHost, relayFrom, useRelay := relayEndpoint(cfg)
	if useRelay {
		from = relayFrom
	}
	if from == "" {
		return fmt.Errorf("set your Email in Config → Personal")
	}
	if !useRelay && !HasGmailOAuth(cfg) && strings.TrimSpace(cfg.GmailAppPassword) == "" {
		return fmt.Errorf("set a Gmail App Password in Config → Outreach, or run nexus-gmailauth to connect a Gmail token")
	}
	if to == "" {
		return fmt.Errorf("contact email is empty — add an address before sending")
	}

	// Recipient verification (KAN-23): when the user has opted into the slow
	// SMTP probe (OutreachSMTPVerify), a definitive invalid verdict blocks the
	// send — malformed address, no MX, or a 5xx rejection of the exact address.
	// Inconclusive results fail open so a network hiccup never stalls a send.
	if cfg.OutreachSMTPVerify {
		vctx, vcancel := context.WithTimeout(context.Background(), 30*time.Second)
		ver := osint.NewVerifier().Verify(vctx, to)
		vcancel()
		if ver.Status == osint.StatusInvalid {
			item.Status = StatusSkipped
			item.Error = "recipient verification failed: " + ver.Detail
			_ = Upsert(item)
			logAttempt(item, StatusSkipped, fmt.Errorf("%s", item.Error))
			return fmt.Errorf("recipient verification failed: %s", ver.Detail)
		}
	}

	// Warm-up ramp (KAN-23): cap the day's sends at the ramp schedule until the
	// sender has been active for SmtpWarmupDays, never exceeding MaxEmailsPerDay.
	// A store read failure fails open to the configured cap.
	max := cfg.MaxEmailsPerDay
	if max <= 0 {
		max = 10
	}
	if cfg.SmtpWarmupDays > 0 {
		if items, err := Load(); err == nil {
			ramped := warmupCap(sendingDaysActive(items, time.Now()), cfg.SmtpWarmupDays, max)
			if ramped < max {
				max = ramped
			}
		}
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
	switch {
	case useRelay:
		auth := smtp.PlainAuth("", strings.TrimSpace(cfg.SmtpRelayUser), cfg.SmtpRelayPass, relayHost)
		if err := smtp.SendMail(relayAddr, auth, from, []string{to}, []byte(raw)); err != nil {
			sendErr = fmt.Errorf("relay send: %w", err)
		}
	case HasGmailOAuth(cfg):
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		sendErr = sendViaGmailAPI(ctx, cfg, raw)
	default:
		auth := smtp.PlainAuth("", from, strings.TrimSpace(cfg.GmailAppPassword), "smtp.gmail.com")
		if err := smtp.SendMail("smtp.gmail.com:587", auth, from, []string{to}, []byte(raw)); err != nil {
			sendErr = fmt.Errorf("smtp send: %w", err)
		}
	}
	if sendErr != nil {
		// Bounce handling (KAN-23): 5xx recipient rejections become a terminal
		// bounced status (no more follow-ups); 4xx and other errors stay failed.
		st, _ := classifySendError(sendErr)
		item.Status = st
		item.Error = sendErr.Error()
		_ = Upsert(item)
		logAttempt(item, st, sendErr)
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
