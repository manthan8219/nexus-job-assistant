package notifier

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

func init() {
	Register(Channel{
		ID:          "email",
		DisplayName: "Email",
		WarnMsg:     "add your Email + Gmail app password in Config → Outreach, and enable Email notifications",
		Configured: func(c *NotifyConfig) bool {
			return c.EmailNotifications && c.Email != "" && c.GmailAppPassword != ""
		},
		Build: func(c *NotifyConfig) Notifier {
			return NewEmailNotifier(c.Email, c.GmailAppPassword)
		},
	})
}

// defaultSMTPServer is Gmail's SMTP submission endpoint.
const defaultSMTPServer = "smtp.gmail.com:587"

// EmailNotifier sends run updates to the user's own inbox via Gmail SMTP.
type EmailNotifier struct {
	from     string
	password string
	server   string // overridable for tests
}

// NewEmailNotifier creates an email notifier for the given Gmail address and
// app password. Empty credentials produce a no-op notifier.
func NewEmailNotifier(from, password string) *EmailNotifier {
	return &EmailNotifier{from: from, password: password, server: defaultSMTPServer}
}

// Name returns "email".
func (e *EmailNotifier) Name() string { return "email" }

// Send delivers a notification email to the user's own inbox.
func (e *EmailNotifier) Send(ctx context.Context, ev Event) error {
	if e.from == "" || e.password == "" {
		return nil // no-op when not configured
	}
	subject, body := e.render(ev)
	if subject == "" {
		return nil
	}
	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	raw := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		e.from, e.from, subject, ts.Format(time.RFC1123Z), body)

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", e.server)
	if err != nil {
		return fmt.Errorf("email dial: %w", err)
	}
	host, _, err := net.SplitHostPort(e.server)
	if err != nil {
		conn.Close()
		return fmt.Errorf("email server address: %w", err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("email client: %w", err)
	}
	defer client.Close()

	// Gmail requires TLS before credentials; upgrade when the server
	// advertises STARTTLS (the test server skips it).
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{ServerName: host}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("email starttls: %w", err)
		}
	}

	auth := smtp.PlainAuth("", e.from, e.password, host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("email auth: %w", err)
	}
	if err := client.Mail(e.from); err != nil {
		return fmt.Errorf("email mail: %w", err)
	}
	if err := client.Rcpt(e.from); err != nil {
		return fmt.Errorf("email rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email data: %w", err)
	}
	if _, err := w.Write([]byte(raw)); err != nil {
		return fmt.Errorf("email write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email close: %w", err)
	}
	return client.Quit()
}

// render builds a plain-text subject + body for an event. Empty subject means
// the event kind is not worth an email.
func (e *EmailNotifier) render(ev Event) (subject, body string) {
	switch ev.Kind {
	case EventJobApplied:
		subject = "✅ Applied: " + ev.JobTitle + " @ " + ev.Company
		body = fmt.Sprintf("Nexus applied to:\n\n  %s @ %s\n  %s\n  provider: %s",
			ev.JobTitle, ev.Company, ev.Location, ev.Provider)
	case EventJobFailed:
		subject = "❌ Application failed: " + ev.JobTitle
		body = fmt.Sprintf("Nexus failed to apply:\n\n  %s @ %s\n  reason: %s",
			ev.JobTitle, ev.Company, ev.Reason)
	case EventRunComplete:
		subject = "⚡ Nexus run complete"
		body = fmt.Sprintf("Applications submitted: %d", ev.TotalApplied)
	case EventDailySummary, EventWeeklySummary:
		subject = "📊 Nexus " + strings.ReplaceAll(string(ev.Kind), "_", " ")
		body = fmt.Sprintf("Applied: %d · Failed: %d · Skipped: %d",
			ev.TotalApplied, ev.TotalFailed, ev.TotalSkipped)
	case EventCustom:
		subject = ev.Title
		if subject == "" {
			subject = "⚡ Nexus"
		}
		body = ev.Message
	default:
		return "", ""
	}
	return subject, body
}
