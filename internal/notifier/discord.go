package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func init() {
	Register(Channel{
		ID:          "discord",
		DisplayName: "Discord",
		WarnMsg:     "add webhook URL above",
		Configured:  func(c *NotifyConfig) bool { return c.DiscordWebhookURL != "" },
		Build:       func(c *NotifyConfig) Notifier { return NewDiscordNotifier(c.DiscordWebhookURL) },
	})
}

// ── Discord embed colours ────────────────────────────────────────────────────
const (
	colorGreen  = 0x57F287
	colorRed    = 0xED4245
	colorYellow = 0xFEE75C
	colorBlue   = 0x5865F2
	colorOrange = 0xF17F2B
	colorPurple = 0x9B59B6
)

// ── Discord API types (we only need what we send) ────────────────────────────

type discordWebhookPayload struct {
	Content   string         `json:"content,omitempty"`
	Username  string         `json:"username,omitempty"`
	AvatarURL string         `json:"avatar_url,omitempty"`
	Embeds    []discordEmbed `json:"embeds,omitempty"`
}

type discordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	URL         string         `json:"url,omitempty"`
	Color       int            `json:"color,omitempty"`
	Fields      []discordField `json:"fields,omitempty"`
	Footer      *discordFooter `json:"footer,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type discordFooter struct {
	Text string `json:"text"`
}

// ── Discord Notifier ─────────────────────────────────────────────────────────

// DiscordNotifier sends notifications via a Discord webhook URL.
// It uses Discord embeds for rich, colour-coded messages.
type DiscordNotifier struct {
	webhookURL string
	client     *http.Client
	username   string // optional: override the webhook's display name
}

// NewDiscordNotifier creates a Discord notifier backed by the given webhook URL.
// An empty webhookURL creates a no-op notifier (all Send calls return nil).
func NewDiscordNotifier(webhookURL string) *DiscordNotifier {
	if webhookURL == "" {
		return &DiscordNotifier{} // no-op
	}
	return &DiscordNotifier{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Name returns "discord".
func (d *DiscordNotifier) Name() string { return "discord" }

// SetUsername overrides the default webhook display name.
func (d *DiscordNotifier) SetUsername(name string) { d.username = name }

// Send delivers an event to the Discord webhook.
func (d *DiscordNotifier) Send(ctx context.Context, ev Event) error {
	if d.webhookURL == "" {
		return nil // no-op when not configured
	}

	payload := d.buildPayload(ev)
	if payload == nil {
		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("discord webhook returned HTTP %d (expected 204)", resp.StatusCode)
	}
	return nil
}

// ── Payload builders ─────────────────────────────────────────────────────────

func (d *DiscordNotifier) buildPayload(ev Event) *discordWebhookPayload {
	switch ev.Kind {
	case EventJobApplied:
		return d.jobAppliedPayload(ev)
	case EventJobFailed:
		return d.jobFailedPayload(ev)
	case EventCAPTCHA:
		return d.captchaPayload(ev)
	case EventError:
		return d.errorPayload(ev)
	case EventRunStarted:
		return d.runStartedPayload(ev)
	case EventRunComplete:
		return d.runCompletePayload(ev)
	case EventDailySummary, EventWeeklySummary:
		return d.summaryPayload(ev)
	case EventReplyReceived:
		return d.replyReceivedPayload(ev)
	case EventCustom:
		return d.customPayload(ev)
	default:
		return d.customPayload(ev)
	}
}

func (d *DiscordNotifier) basePayload() *discordWebhookPayload {
	p := &discordWebhookPayload{}
	if d.username != "" {
		p.Username = d.username
	}
	return p
}

func (d *DiscordNotifier) embed(color int, title, desc string) discordEmbed {
	return discordEmbed{
		Title:       title,
		Description: desc,
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Footer:      &discordFooter{Text: "JobPilot Bot"},
	}
}

func (d *DiscordNotifier) jobAppliedPayload(ev Event) *discordWebhookPayload {
	p := d.basePayload()
	desc := fmt.Sprintf("**%s** @ **%s**", ev.JobTitle, ev.Company)
	if ev.Location != "" {
		desc += "\n📍 " + ev.Location
	}
	if ev.Provider != "" {
		desc += "\n🔧 " + ev.Provider
	}
	e := d.embed(colorGreen, "✅ Application Submitted", desc)
	if ev.Reason != "" {
		e.Fields = append(e.Fields, discordField{Name: "Note", Value: ev.Reason})
	}
	p.Embeds = []discordEmbed{e}
	return p
}

func (d *DiscordNotifier) jobFailedPayload(ev Event) *discordWebhookPayload {
	p := d.basePayload()
	desc := fmt.Sprintf("**%s** @ **%s**", ev.JobTitle, ev.Company)
	if ev.Provider != "" {
		desc += "\n🔧 " + ev.Provider
	}
	e := d.embed(colorRed, "❌ Application Failed", desc)
	e.Fields = append(e.Fields, discordField{Name: "Reason", Value: ev.Reason})
	p.Embeds = []discordEmbed{e}
	return p
}

func (d *DiscordNotifier) replyReceivedPayload(ev Event) *discordWebhookPayload {
	p := d.basePayload()
	desc := fmt.Sprintf("**%s** replied", ev.ReplyFrom)
	if ev.Company != "" {
		desc += fmt.Sprintf(" — **%s** @ **%s**", ev.JobTitle, ev.Company)
	}
	if ev.ReplySubject != "" {
		desc += "\n✉️ " + ev.ReplySubject
	}
	e := d.embed(colorGreen, "📬 Reply Received — respond now!", desc)
	p.Embeds = []discordEmbed{e}
	return p
}

func (d *DiscordNotifier) captchaPayload(ev Event) *discordWebhookPayload {
	p := d.basePayload()
	desc := "A CAPTCHA was encountered during automation."
	if ev.CAPTCHAURL != "" {
		desc += "\n\n**URL:** " + ev.CAPTCHAURL
	}
	e := d.embed(colorOrange, "🤖 CAPTCHA Detected", desc)
	p.Embeds = []discordEmbed{e}
	return p
}

func (d *DiscordNotifier) errorPayload(ev Event) *discordWebhookPayload {
	p := d.basePayload()
	desc := ev.Message
	if desc == "" {
		desc = "An unexpected error occurred."
	}
	e := d.embed(colorRed, "🚨 Error", desc)
	p.Embeds = []discordEmbed{e}
	return p
}

func (d *DiscordNotifier) runStartedPayload(ev Event) *discordWebhookPayload {
	p := d.basePayload()
	e := d.embed(colorBlue, "⚡ Run Started", "Job application run has begun.")
	if ev.Message != "" {
		e.Description = ev.Message
	}
	p.Embeds = []discordEmbed{e}
	return p
}

func (d *DiscordNotifier) runCompletePayload(ev Event) *discordWebhookPayload {
	p := d.basePayload()
	desc := fmt.Sprintf(
		"**Applied:** %d\n**Failed:** %d\n**Skipped:** %d",
		ev.TotalApplied, ev.TotalFailed, ev.TotalSkipped,
	)
	if ev.RunDuration > 0 {
		desc += "\n**Duration:** " + FormatDuration(ev.RunDuration)
	}
	if len(ev.Errors) > 0 {
		errBlock := ""
		for i, e := range ev.Errors {
			if i >= 5 {
				errBlock += fmt.Sprintf("\n… and %d more", len(ev.Errors)-5)
				break
			}
			errBlock += "\n• " + e
		}
		desc += "\n**Errors:**" + errBlock
	}
	e := d.embed(colorGreen, "✅ Run Complete", desc)
	p.Embeds = []discordEmbed{e}
	return p
}

func (d *DiscordNotifier) summaryPayload(ev Event) *discordWebhookPayload {
	p := d.basePayload()
	label := "📊 Daily Summary"
	if ev.Kind == EventWeeklySummary {
		label = "📈 Weekly Summary"
	}
	desc := fmt.Sprintf(
		"**Total Applied:** %d\n**Total Failed:** %d\n**Total Skipped:** %d",
		ev.TotalApplied, ev.TotalFailed, ev.TotalSkipped,
	)
	e := d.embed(colorPurple, label, desc)
	p.Embeds = []discordEmbed{e}
	return p
}

func (d *DiscordNotifier) customPayload(ev Event) *discordWebhookPayload {
	p := d.basePayload()
	title := ev.Title
	if title == "" {
		title = "ℹ️ Notification"
	}
	desc := ev.Message
	if desc == "" {
		desc = "(no message)"
	}
	e := d.embed(colorBlue, title, desc)
	for k, v := range ev.Fields {
		e.Fields = append(e.Fields, discordField{Name: k, Value: v, Inline: true})
	}
	p.Embeds = []discordEmbed{e}
	return p
}
