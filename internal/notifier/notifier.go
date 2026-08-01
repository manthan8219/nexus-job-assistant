// Package notifier provides a notification abstraction over multiple channels.
//
// The core interface is Notifier. Each platform (Discord, Slack, Telegram, etc.)
// implements this interface. A MultiNotifier fans out to many notifiers at once.
// The Engine uses the notifier package to report application results, errors,
// CAPTCHAs, and periodic summaries without caring which channels are configured.
package notifier

import (
	"context"
	"fmt"
	"time"
)

// ── Event types ──────────────────────────────────────────────────────────────

// EventKind categorises a notification event.
type EventKind string

const (
	EventJobApplied    EventKind = "job_applied"
	EventJobFailed     EventKind = "job_failed"
	EventCAPTCHA       EventKind = "captcha"
	EventError         EventKind = "error"
	EventDailySummary  EventKind = "daily_summary"
	EventWeeklySummary EventKind = "weekly_summary"
	EventCustom        EventKind = "custom"
	EventRunStarted    EventKind = "run_started"
	EventRunComplete   EventKind = "run_complete"
	EventReplyReceived EventKind = "reply_received"
)

// Event carries all data a notifier might need to build a message.
type Event struct {
	Kind      EventKind
	Timestamp time.Time

	// ── job-related fields (populated for job_applied / job_failed) ──
	JobTitle string
	Company  string
	Location string
	Provider string
	Status   string // "applied", "failed", "skipped"
	Reason   string // failure / skip reason

	// ── summary fields (populated for daily / weekly summary) ──
	TotalApplied int
	TotalFailed  int
	TotalSkipped int
	RunDuration  time.Duration
	Errors       []string

	// ── CAPTCHA ──
	CAPTCHAURL string

	// ── reply_received (a human answered an outreach email / application) ──
	ReplyFrom    string // sender address of the reply
	ReplySubject string // subject line of the reply

	// ── custom ──
	Title   string
	Message string
	Fields  map[string]string
}

// ── Interfaces ───────────────────────────────────────────────────────────────

// Notifier sends a single event to one notification channel.
type Notifier interface {
	// Name returns a human-readable identifier (e.g. "discord").
	Name() string

	// Send delivers an event. Implementations must be safe for concurrent use.
	Send(ctx context.Context, ev Event) error
}

// MultiNotifier fans out an event to every registered notifier in parallel.
// A nil slice is safe to use (it acts as a no-op notifier).
type MultiNotifier []Notifier

// Send delivers the event to all registered notifiers concurrently.
// Errors are collected; a single notifier failure does not block the others.
func (mn MultiNotifier) Send(ctx context.Context, ev Event) []error {
	var errs []error
	for _, n := range mn {
		if err := n.Send(ctx, ev); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// NotifyConfig holds all notification credentials and channel selection.
// Mirrors the relevant fields from config.Config so this package stays decoupled.
// To add a new channel: implement Notifier and Register() it from that file's init().
type NotifyConfig struct {
	DiscordWebhookURL string
	TelegramBotToken  string
	TelegramChatID    string
	// Email run updates (delivered to the user's own inbox via Gmail SMTP).
	Email              string
	GmailAppPassword   string
	EmailNotifications bool
	// EnabledChannels is the user-selected list of channels to use.
	// Empty means all channels with valid credentials are active.
	EnabledChannels []string
}

// Channel describes a discoverable notification integration.
// Each notifier file self-registers via init() → Register(); the UI and
// FromConfig both read Available() — nothing else needs updating for the list.
type Channel struct {
	ID          string // stable id stored in config (e.g. "discord")
	DisplayName string // human label shown in the UI
	WarnMsg     string // shown when selected but credentials are missing
	Configured  func(*NotifyConfig) bool
	Build       func(*NotifyConfig) Notifier
}

// registry is populated at package init by each notifier calling Register.
var registry []Channel

// Register adds a channel to the discovery list.
// Call from init() in the notifier's own file so the list auto-grows.
func Register(ch Channel) {
	registry = append(registry, ch)
}

// Available returns every registered notification channel (UI discovery).
func Available() []Channel {
	out := make([]Channel, len(registry))
	copy(out, registry)
	return out
}

// FromConfig builds a MultiNotifier from the application config.
// Only channels that are both enabled and have valid credentials are included.
func FromConfig(cfg *NotifyConfig) MultiNotifier {
	var mn MultiNotifier
	for _, ch := range registry {
		if !cfg.channelEnabled(ch.ID) || !ch.Configured(cfg) {
			continue
		}
		mn = append(mn, ch.Build(cfg))
	}
	return mn
}

// channelEnabled returns true if the channel should be active.
// When EnabledChannels is empty, all channels with credentials are active.
func (c *NotifyConfig) channelEnabled(name string) bool {
	if len(c.EnabledChannels) == 0 {
		return true
	}
	for _, ch := range c.EnabledChannels {
		if ch == name {
			return true
		}
	}
	return false
}

// FormatDuration returns a human-readable duration string.
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	if mins < 60 {
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	h := mins / 60
	m := mins % 60
	return fmt.Sprintf("%dh %dm %ds", h, m, secs)
}
