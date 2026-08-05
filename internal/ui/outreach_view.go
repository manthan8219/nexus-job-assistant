package ui

// Package ui — outreach_view.go
// View rendering for the Outreach tab: the step strip, next-action hint, the
// Setup / Email / LinkedIn sub-tab views, draft list, body wrapping, and URL
// trimming. The model + Update live in outreach_hub.go and key handlers in
// outreach_keys.go.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
)

func (m OutreachHubModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	var b strings.Builder
	b.WriteString(m.renderStepStrip())
	b.WriteString("\n")
	b.WriteString(m.renderNextAction())
	b.WriteString("\n\n")

	switch m.sub {
	case outreachSubSetup:
		b.WriteString(m.viewSetup(w))
	case outreachSubEmail:
		b.WriteString(m.viewEmail(w))
	case outreachSubLinkedIn:
		b.WriteString(m.viewLinkedIn(w))
	case outreachSubSent:
		b.WriteString(m.viewSent(w))
	}

	if m.errText != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render("⚠ " + m.errText))
	} else if m.status != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("✓ " + m.status))
	}
	return b.String()
}

func (m OutreachHubModel) renderStepStrip() string {
	emailOK := outreach.AllOK(outreach.EmailReady(m.effectiveCfg()))
	liOK := outreach.AllOK(outreach.LinkedInReady(m.effectiveCfg()))
	done := [outreachSubCount]bool{
		m.consent,
		emailOK && len(outreach.Pending(m.items, outreach.ChannelEmail)) > 0,
		liOK && len(outreach.Pending(m.items, outreach.ChannelLinkedIn)) > 0,
		len(m.logEntries) > 0,
	}
	parts := make([]string, 0, 6)
	for i, label := range outreachSubLabels {
		num := fmt.Sprintf("%d.", i+1)
		mark := "○"
		style := mutedStyle
		if done[i] {
			mark = "✓"
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
		}
		if i == m.sub {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
			if !done[i] {
				mark = "●"
			}
		}
		parts = append(parts, style.Render(fmt.Sprintf("%s %s %s", mark, num, label)))
		if i < outreachSubCount-1 {
			parts = append(parts, mutedStyle.Render("  →  "))
		}
	}
	return strings.Join(parts, "")
}

func (m OutreachHubModel) renderNextAction() string {
	mode := outreach.NormalizeMode(m.runMode)
	line := ""
	switch {
	case m.ui == outConfirmAction:
		line = "Approve this action? [y] run  [n] cancel"
	case m.ui == outRunning:
		line = "Auto mode running — esc to stop"
	case m.sub == outreachSubSetup:
		if !m.consent {
			line = "Opt in, pick automation mode (Confirm / Queue / Auto), set caps — saves as you go."
		} else {
			line = "Mode: " + mode + " · tab → Email or LinkedIn to build & run queues."
		}
	case m.sub == outreachSubEmail:
		if m.building {
			line = "Finding contacts + drafting emails…"
		} else if len(outreach.Pending(m.items, outreach.ChannelEmail)) == 0 {
			line = "Press g — JobPilot builds an email queue from Jobs (Hunter finds To: addresses)."
		} else if mode == outreach.ModeAuto {
			line = "Queue ready · press a to send all remaining (or enter for one)."
		} else if mode == outreach.ModeQueue {
			line = "Queue ready · mash enter to send, send, send…"
		} else {
			line = "Queue ready · enter asks before each send."
		}
	case m.sub == outreachSubLinkedIn:
		if m.building {
			line = "Building LinkedIn queue…"
		} else if len(outreach.Pending(m.items, outreach.ChannelLinkedIn)) == 0 {
			line = "Press g — JobPilot queues companies, then opens LinkedIn in your browser."
		} else if mode == outreach.ModeAuto {
			line = "Queue ready · press a to open browser for each company automatically."
		} else if mode == outreach.ModeQueue {
			line = "Queue ready · enter opens the next LinkedIn search (message copied for paste if needed)."
		} else {
			line = "Queue ready · enter asks, then opens LinkedIn in the browser."
		}
	case m.sub == outreachSubSent:
		if m.logLoading {
			line = "Loading sent log…"
		} else {
			line = fmt.Sprintf("%d sent actions recorded. Press r to refresh.", len(m.logEntries))
		}
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render("→ " + line)
}

func (m OutreachHubModel) viewSetup(w int) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Automated outreach"))
	b.WriteString("\n")
	b.WriteString(primaryStyle.Render("JobPilot prepares follow-ups from jobs you already applied to, then runs them."))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Email: finds recruiter addresses + sends via Gmail.  LinkedIn: opens browser searches for you to message."))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("Controls"))
	b.WriteString("\n")
	b.WriteString(m.renderSetupRow(setupConsent, "Consent", m.consentLabel()))
	b.WriteString(m.renderSetupRow(setupRunMode, "Automation mode", outreach.ModeLabel(m.runMode)))
	b.WriteString(m.renderSetupRow(setupMaxEmail, "Max emails / day", strconv.Itoa(m.maxEmail)))
	b.WriteString(m.renderSetupRow(setupMaxLI, "Max LinkedIn opens / day", strconv.Itoa(m.maxLI)))
	if m.setupInput.Focused() {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Edit: ") + m.setupInput.View() + "\n")
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Confirm = ask each time · Queue = tap Enter repeatedly · Auto = run the whole queue"))
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("Email checklist"))
	b.WriteString("\n" + m.renderChecks(outreach.EmailReady(m.effectiveCfg())))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("LinkedIn checklist"))
	b.WriteString("\n" + m.renderChecks(outreach.LinkedInReady(m.effectiveCfg())))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Secrets → Config → Outreach (Gmail app password, Hunter.io key)."))
	_ = w
	return b.String()
}

func (m OutreachHubModel) consentLabel() string {
	if m.consent {
		return "Yes — run automated follow-ups I start from this tab"
	}
	return "No — outreach blocked"
}

func (m OutreachHubModel) renderSetupRow(focus int, label, value string) string {
	cursor := "  "
	style := mutedStyle
	if m.setupFocus == focus {
		cursor = "▸ "
		style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	}
	return cursor + style.Render(label) + "  " + primaryStyle.Render(value) + "\n"
}

func (m OutreachHubModel) renderChecks(checks []outreach.Check) string {
	var b strings.Builder
	for _, c := range checks {
		mark := lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render("✗")
		if c.OK {
			mark = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("✓")
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", mark, c.Label))
		if !c.OK && c.FixHint != "" {
			b.WriteString(mutedStyle.Render("      → "+c.FixHint) + "\n")
		}
	}
	return b.String()
}

func (m OutreachHubModel) viewEmail(w int) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Email automation"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("g builds a queue from Jobs · Hunter fills To: · Enter/Auto sends via Gmail (no copy-paste)."))
	b.WriteString("\n\n")
	pend := outreach.Pending(m.items, outreach.ChannelEmail)
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Pending %d · total %d", len(pend), len(m.filtered()))))
	b.WriteString("\n\n")
	list := m.filtered()
	if len(list) == 0 {
		b.WriteString(mutedStyle.Render("Empty. Press g to generate the queue."))
		return b.String()
	}
	b.WriteString(m.renderDraftList(list, w))
	if it, ok := m.selected(); ok {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Selected") + "\n")
		b.WriteString(primaryStyle.Render(fmt.Sprintf("%s — %s", it.Company, it.Role)) + "\n")
		to := it.ContactEmail
		if to == "" {
			to = "(missing — press e, or rebuild with Hunter key)"
		}
		if m.ui == outEditContact {
			b.WriteString(labelStyle.Render("To: ") + m.contactInput.View() + "\n")
		} else {
			b.WriteString(mutedStyle.Render("To: ") + primaryStyle.Render(to) + "\n")
		}
		b.WriteString(mutedStyle.Render("Subject: ") + primaryStyle.Render(it.Subject) + "\n")
		statusText := string(it.Status)
		if it.Status == outreach.StatusFollowUpDue {
			statusText = fmt.Sprintf("follow-up #%d %s", it.FollowUpStep, outreach.FollowUpDueIn(it, time.Now()))
		} else if it.Status == outreach.StatusReplied {
			statusText = "replied ✓ — sequence stopped"
		}
		b.WriteString(mutedStyle.Render("Status: ") + primaryStyle.Render(statusText) + "\n")
		b.WriteString("\n" + labelStyle.Render("Body") + "\n")
		b.WriteString(wrapBody(it.Body, w))
	}
	if m.ui == outConfirmAction && m.pending.Channel == outreach.ChannelEmail {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true).
			Render(fmt.Sprintf("Send email to %s now?  [y]  [n]", m.pending.ContactEmail)))
	}
	return b.String()
}

func (m OutreachHubModel) viewLinkedIn(w int) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("LinkedIn automation"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("g builds a queue · Enter/Auto opens LinkedIn people-search in your browser (message placed on clipboard)."))
	b.WriteString("\n\n")
	pend := outreach.Pending(m.items, outreach.ChannelLinkedIn)
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Pending %d · total %d", len(pend), len(m.filtered()))))
	b.WriteString("\n\n")
	list := m.filtered()
	if len(list) == 0 {
		b.WriteString(mutedStyle.Render("Empty. Press g to generate the queue."))
		return b.String()
	}
	b.WriteString(m.renderDraftList(list, w))
	if it, ok := m.selected(); ok {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Selected") + "\n")
		b.WriteString(primaryStyle.Render(fmt.Sprintf("%s — %s", it.Company, it.Role)) + "\n")
		b.WriteString(mutedStyle.Render("Status: ") + primaryStyle.Render(string(it.Status)) + "\n")
		if it.LinkedInURL != "" {
			b.WriteString(mutedStyle.Render("Opens: ") + primaryStyle.Render(trimURL(it.LinkedInURL, w-8)) + "\n")
		}
		b.WriteString("\n" + labelStyle.Render("Message (auto-copied when browser opens)") + "\n")
		b.WriteString(wrapBody(it.Body, w))
	}
	if m.ui == outConfirmAction && m.pending.Channel == outreach.ChannelLinkedIn {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true).
			Render(fmt.Sprintf("Open LinkedIn for %s?  [y]  [n]", m.pending.Company)))
	}
	return b.String()
}

func (m OutreachHubModel) renderDraftList(list []outreach.Item, w int) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Queue") + "\n")
	limit := 8
	start := 0
	if m.cursor >= limit {
		start = m.cursor - limit + 1
	}
	for i := start; i < len(list) && i < start+limit; i++ {
		it := list[i]
		cur := "  "
		style := primaryStyle
		if i == m.cursor {
			cur = "▸ "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
		}
		line := fmt.Sprintf("[%s] %s · %s", it.Status, it.Company, it.Role)
		if it.ContactEmail != "" {
			line += "  <" + it.ContactEmail + ">"
		}
		if len(line) > w-4 {
			line = line[:max(0, w-4)]
		}
		b.WriteString(cur + style.Render(line) + "\n")
	}
	return b.String()
}

func wrapBody(s string, w int) string {
	if w < 40 {
		w = 40
	}
	maxW := w - 2
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		for len(line) > maxW {
			b.WriteString(primaryStyle.Render(line[:maxW]) + "\n")
			line = line[maxW:]
		}
		b.WriteString(primaryStyle.Render(line) + "\n")
	}
	return b.String()
}

func (m OutreachHubModel) viewSent(w int) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Sent outreach"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Audit log of every email sent and LinkedIn action taken by JobPilot."))
	b.WriteString("\n\n")
	if m.logLoading {
		b.WriteString(mutedStyle.Render("Loading…"))
		return b.String()
	}
	if len(m.logEntries) == 0 {
		b.WriteString(mutedStyle.Render("No sent outreach yet. Send emails or open LinkedIn from the Email / LinkedIn tabs."))
		return b.String()
	}

	const dateW, chanW, statusW, nameW = 10, 9, 8, 20
	compW := w - dateW - chanW - statusW - nameW - 10
	if compW < 10 {
		compW = 10
	}

	hdr := fmt.Sprintf("%-*s  %-*s  %-*s  %-*s  %s",
		dateW, "Date", chanW, "Channel", statusW, "Status", nameW, "Contact", "Company / Role")
	b.WriteString(mutedStyle.Render(hdr))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(strings.Repeat("─", min(w-2, 90))))
	b.WriteString("\n")

	visH := m.height - 14
	if visH < 3 {
		visH = 3
	}
	start := 0
	if m.logCursor >= visH {
		start = m.logCursor - visH + 1
	}
	end := start + visH
	if end > len(m.logEntries) {
		end = len(m.logEntries)
	}

	for i := start; i < end; i++ {
		e := m.logEntries[i]
		date := e.CreatedAt.Format("2006-01-02")
		if e.SentAt.Year() > 2000 {
			date = e.SentAt.Format("2006-01-02")
		}
		ch := e.Channel
		if len(ch) > chanW {
			ch = ch[:chanW]
		}
		status := e.Status
		if len(status) > statusW {
			status = status[:statusW]
		}
		contact := e.ContactName
		if contact == "" {
			contact = e.ContactEmail
		}
		if len(contact) > nameW {
			contact = contact[:nameW-1] + "…"
		}
		company := e.Company
		if e.Role != "" {
			company = e.Company + " · " + e.Role
		}
		if len(company) > compW {
			company = company[:compW-1] + "…"
		}
		row := fmt.Sprintf("%-*s  %-*s  %-*s  %-*s  %s",
			dateW, date, chanW, ch, statusW, status, nameW, contact, company)
		if i == m.logCursor {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render("▸ " + row))
		} else {
			b.WriteString("  " + row)
		}
		b.WriteString("\n")
	}
	if len(m.logEntries) > visH {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ↕ showing %d–%d of %d  ·  r refresh", start+1, end, len(m.logEntries))))
	}
	return b.String()
}

func trimURL(u string, n int) string {
	if n < 16 {
		n = 16
	}
	if len(u) <= n {
		return u
	}
	return u[:n-3] + "..."
}
