package notifier

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
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
			n := NewEmailNotifier(c.Email, c.GmailAppPassword)
			n.perJob = c.EmailPerJob
			return n
		},
	})
}

// defaultSMTPServer is Gmail's SMTP submission endpoint.
const defaultSMTPServer = "smtp.gmail.com:587"

// EmailNotifier sends run updates to the user's own inbox via Gmail SMTP.
//
// It consolidates: per-job applied/failed events are withheld and delivered
// once in the run-complete digest. Instant alerts (CAPTCHA, reply, error) are
// always emailed immediately. perJob opts back into per-job emails.
type EmailNotifier struct {
	from     string
	password string
	server   string // overridable for tests
	perJob   bool   // email each applied/failed job individually (off by default)
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
	switch ev.Kind {
	case EventRunStarted:
		// No email for run start — the digest lands at run completion.
		return nil
	case EventJobApplied, EventJobFailed:
		// Consolidated into the run digest by default; individual emails only
		// when the user opts in via EmailPerJob.
		if !e.perJob {
			return nil
		}
		return e.sendEvent(ctx, ev)
	case EventRunComplete, EventDailySummary, EventWeeklySummary,
		EventCAPTCHA, EventReplyReceived, EventError, EventCustom:
		return e.sendEvent(ctx, ev)
	default:
		return nil
	}
}

// sendEvent renders and delivers one email for the event. The message is
// multipart/alternative: a plain-text fallback plus an HTML (styled) body.
func (e *EmailNotifier) sendEvent(ctx context.Context, ev Event) error {
	subject, textBody := e.render(ev)
	if subject == "" {
		return nil
	}
	htmlBody := e.renderHTML(ev)
	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	var raw strings.Builder
	fmt.Fprintf(&raw, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\n",
		e.from, e.from, subject, ts.Format(time.RFC1123Z))
	const boundary = "nx-alt-7e2f9c"
	fmt.Fprintf(&raw, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&raw, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", boundary, textBody)
	if htmlBody != "" {
		fmt.Fprintf(&raw, "--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n", boundary, htmlBody)
	}
	fmt.Fprintf(&raw, "--%s--\r\n", boundary)
	payload := raw.String()

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
	if _, err := w.Write([]byte(payload)); err != nil {
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
		body = e.renderApplied(ev)
	case EventJobFailed:
		subject = "❌ Application failed: " + ev.JobTitle
		body = e.renderFailed(ev)
	case EventRunComplete:
		if ev.DryRun {
			subject = fmt.Sprintf("⚡ Daily job digest — %d jobs found", dailyMatchCount(ev))
			body = e.renderDryRunComplete(ev)
		} else {
			subject = "⚡ Nexus run complete"
			body = e.renderRunComplete(ev)
		}
	case EventDailySummary, EventWeeklySummary:
		subject = "📊 Nexus " + strings.ReplaceAll(string(ev.Kind), "_", " ")
		body = e.renderSummary(ev)
	case EventCAPTCHA:
		subject = "⛔ CAPTCHA — complete manually to continue"
		body = e.renderCaptcha(ev)
	case EventReplyReceived:
		subject = "📩 Reply received"
		if ev.ReplySubject != "" {
			subject += ": " + ev.ReplySubject
		}
		body = e.renderReply(ev)
	case EventError:
		subject = "🚨 Nexus error"
		body = e.renderError(ev)
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

// renderApplied builds a detailed "applied" email with the job's key facts.
func (e *EmailNotifier) renderApplied(ev Event) string {
	var b strings.Builder
	b.WriteString("Nexus applied to this job:\n\n")
	line(&b, "Job", ev.JobTitle)
	line(&b, "Company", ev.Company)
	line(&b, "Location", ev.Location)
	line(&b, "Source", ev.Provider)
	if !ev.PostedAt.IsZero() {
		line(&b, "Posted", ago(ev.PostedAt))
	}
	if ev.JobURL != "" {
		b.WriteString("\n  Apply page: " + ev.JobURL + "\n")
	}
	return b.String()
}

// renderFailed builds a detailed "failed" email with the reason and the link
// to finish the application manually.
func (e *EmailNotifier) renderFailed(ev Event) string {
	var b strings.Builder
	b.WriteString("Nexus could not apply to this job:\n\n")
	line(&b, "Job", ev.JobTitle)
	line(&b, "Company", ev.Company)
	line(&b, "Location", ev.Location)
	line(&b, "Source", ev.Provider)
	line(&b, "Reason", ev.Reason)
	if ev.JobURL != "" {
		b.WriteString("\n  Open it and apply manually:\n  " + ev.JobURL + "\n")
	}
	return b.String()
}

// renderRunComplete builds a consolidated digest of a completed (real) run,
// listing each applied and failed job.
func (e *EmailNotifier) renderRunComplete(ev Event) string {
	var b strings.Builder
	b.WriteString("Nexus finished searching and applying.\n\n")
	line(&b, "Scraped", fmt.Sprintf("%d", ev.Found))
	line(&b, "Applied", fmt.Sprintf("%d", ev.TotalApplied))
	line(&b, "Failed", fmt.Sprintf("%d", ev.TotalFailed))
	line(&b, "Skipped", fmt.Sprintf("%d", ev.TotalSkipped))
	line(&b, "Duration", FormatDuration(ev.RunDuration))
	line(&b, "Finished", ev.Timestamp.Format("Mon Jan 2 15:04"))
	appendJobLists(&b, ev.Jobs)
	appendErrors(&b, ev.Errors)
	return b.String()
}

// renderDryRunComplete builds a summary of a dry run (nothing was submitted),
// listing the jobs that matched the search.
func (e *EmailNotifier) renderDryRunComplete(ev Event) string {
	var b strings.Builder
	b.WriteString("Your daily dry run finished. Nothing was submitted.\n\n")
	line(&b, "Scraped", fmt.Sprintf("%d", dailyMatchCount(ev)))
	line(&b, "Matched", fmt.Sprintf("%d", len(ev.Jobs)))
	line(&b, "Submitted", "0 (dry run)")
	line(&b, "Duration", FormatDuration(ev.RunDuration))
	line(&b, "Finished", ev.Timestamp.Format("Mon Jan 2 15:04"))
	appendJobLists(&b, ev.Jobs)
	appendErrors(&b, ev.Errors)
	return b.String()
}

// renderSummary builds a daily / weekly digest.
func (e *EmailNotifier) renderSummary(ev Event) string {
	var b strings.Builder
	b.WriteString("Nexus run summary\n\n")
	if ev.Found > 0 {
		line(&b, "Scraped", fmt.Sprintf("%d", ev.Found))
	}
	line(&b, "Applied", fmt.Sprintf("%d", ev.TotalApplied))
	line(&b, "Failed", fmt.Sprintf("%d", ev.TotalFailed))
	line(&b, "Skipped", fmt.Sprintf("%d", ev.TotalSkipped))
	if ev.RunDuration > 0 {
		line(&b, "Duration", FormatDuration(ev.RunDuration))
	}
	line(&b, "Generated", ev.Timestamp.Format("Mon Jan 2 15:04"))
	appendJobLists(&b, ev.Jobs)
	appendErrors(&b, ev.Errors)
	return b.String()
}

// dailyMatchCount is how many jobs the daily (dry) run surfaced — used in the
// nudge subject "⚡ Daily job digest — N jobs found".
func dailyMatchCount(ev Event) int {
	if ev.Found > 0 {
		return ev.Found
	}
	return ev.Scanned
}

// renderCaptcha builds an instant alert telling the user to finish a job
// manually (CAPTCHA / anti-bot compliance, AGENTS.md 14).
func (e *EmailNotifier) renderCaptcha(ev Event) string {
	var b strings.Builder
	b.WriteString("Nexus hit a CAPTCHA while automating this job and stopped:\n\n")
	line(&b, "Job", ev.JobTitle)
	line(&b, "Company", ev.Company)
	line(&b, "Reason", ev.Reason)
	url := ev.CAPTCHAURL
	if url == "" {
		url = ev.JobURL
	}
	if url != "" {
		b.WriteString("\n  Open it and complete the CAPTCHA manually:\n  " + url + "\n")
	}
	return b.String()
}

// renderReply builds an instant alert that a human replied.
func (e *EmailNotifier) renderReply(ev Event) string {
	var b strings.Builder
	b.WriteString("Someone replied to your outreach / application:\n\n")
	line(&b, "From", ev.ReplyFrom)
	line(&b, "Subject", ev.ReplySubject)
	b.WriteString("\n  Open Nexus → Outreach to review it and continue the conversation.\n")
	return b.String()
}

// renderError builds an instant alert for a run error.
func (e *EmailNotifier) renderError(ev Event) string {
	msg := ev.Message
	if msg == "" {
		msg = ev.Reason
	}
	if msg == "" {
		msg = "unknown error"
	}
	return "Nexus hit a problem:\n\n  " + msg + "\n"
}

// appendJobLists writes the per-job sections (applied / needs-manual-action /
// matched-in-dry-run) of a digest, grouped by status.
func appendJobLists(b *strings.Builder, jobs []JobEvent) {
	var applied, manual, found []JobEvent
	for _, j := range jobs {
		switch j.Status {
		case "applied":
			applied = append(applied, j)
		case "failed":
			manual = append(manual, j)
		case "found":
			found = append(found, j)
		}
	}
	if len(applied) > 0 {
		b.WriteString("\n  ✓ Applied:\n")
		for _, j := range applied {
			jobBullet(b, j, false)
		}
	}
	if len(manual) > 0 {
		b.WriteString("\n  ✗ Needs manual action:\n")
		for _, j := range manual {
			jobBullet(b, j, true)
		}
	}
	if len(found) > 0 {
		b.WriteString("\n  ✓ Matched jobs (dry run — nothing submitted):\n")
		for _, j := range found {
			jobBullet(b, j, false)
		}
	}
}

// jobBullet writes one indented job row with its URL and optional reason.
func jobBullet(b *strings.Builder, j JobEvent, withReason bool) {
	title := j.Title
	if j.Company != "" {
		title += " @ " + j.Company
	}
	fmt.Fprintf(b, "    • %s\n", title)
	if j.URL != "" {
		fmt.Fprintf(b, "      %s\n", j.URL)
	}
	if withReason && j.Reason != "" {
		fmt.Fprintf(b, "      reason: %s\n", j.Reason)
	}
}

// line writes a two-column "  Label: value" row.
func line(b *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "  %-10s %s\n", label+":", value)
}

// appendErrors appends a "Problems" section when the event carries errors.
func appendErrors(b *strings.Builder, errs []string) {
	if len(errs) == 0 {
		return
	}
	b.WriteString("\n  Problems:\n")
	for _, e := range errs {
		fmt.Fprintf(b, "    - %s\n", e)
	}
}

// ago returns a short human duration like "2h ago" or "3d ago".
func ago(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// ── HTML rendering ──────────────────────────────────────────────────────────
// HTML bodies use inline styles only (email clients strip <style> blocks).
// Builders compose a digest with a header band, stat cards, and job-list
// sections; alert kinds render as a simpler attention card.

const (
	htmlAccent   = "#4F46E5" // indigo — Nexus brand
	htmlApplied  = "#059669" // green
	htmlFailed   = "#DC2626" // red
	htmlSkipped  = "#64748B" // slate
	htmlMatched  = "#D97706" // amber
	htmlCardBG   = "#F9FAFB"
	htmlBodyBG   = "#F4F5F7"
	htmlTextDark = "#1F2937"
	htmlTextGrey = "#6B7280"
	htmlBorder   = "#E5E7EB"
)

// statCard is one number+label cell in the stats row.
type statCard struct {
	label string
	value string
	color string
}

// htmlRow is one job inside a section list.
type htmlRow struct {
	title  string
	meta   string
	url    string
	reason string
}

// htmlSection is a titled list (e.g. "✓ Applied").
type htmlSection struct {
	title string
	color string
	rows  []htmlRow
}

// renderHTML builds a styled HTML body for an event; "" means no email anyway.
func (e *EmailNotifier) renderHTML(ev Event) string {
	switch ev.Kind {
	case EventRunComplete:
		if ev.DryRun {
			return htmlDigestDoc("Daily job digest", ev.Timestamp, htmlDryStats(ev), htmlJobSections(ev.Jobs),
				"This is a safe dry run — nothing was submitted. Open your dashboard to review the matches.")
		}
		return htmlDigestDoc("Run complete", ev.Timestamp, htmlRunStats(ev), htmlJobSections(ev.Jobs), "")
	case EventDailySummary, EventWeeklySummary:
		return htmlDigestDoc(strings.ReplaceAll(string(ev.Kind), "_", " "), ev.Timestamp, htmlRunStats(ev), htmlJobSections(ev.Jobs), "")
	case EventJobApplied:
		return htmlJobCard(ev, true)
	case EventJobFailed:
		return htmlJobCard(ev, false)
	case EventCAPTCHA:
		return htmlAlertDoc("⛔ CAPTCHA", "Nexus hit a CAPTCHA and stopped. Complete it manually to continue.",
			ev.JobTitle, ev.Company, ev.Reason, firstURL(ev.CAPTCHAURL, ev.JobURL))
	case EventReplyReceived:
		return htmlAlertDoc("📩 Reply received", "Someone replied to your outreach or application.",
			ev.ReplyFrom, ev.ReplySubject, "", "")
	case EventError:
		msg := ev.Message
		if msg == "" {
			msg = ev.Reason
		}
		if msg == "" {
			msg = "unknown error"
		}
		return htmlAlertDoc("🚨 Nexus error", "Nexus hit a problem while running.", "", "", msg, "")
	case EventCustom:
		return htmlAlertDoc("⚡ Nexus", ev.Message, "", "", "", "")
	default:
		return ""
	}
}

// htmlRunStats returns the stat cards for a real run / summary.
func htmlRunStats(ev Event) []statCard {
	return []statCard{
		{label: "scraped", value: itoa(ev.Found), color: htmlAccent},
		{label: "applied", value: itoa(ev.TotalApplied), color: htmlApplied},
		{label: "failed", value: itoa(ev.TotalFailed), color: htmlFailed},
		{label: "skipped", value: itoa(ev.TotalSkipped), color: htmlSkipped},
	}
}

// htmlDryStats returns the stat cards for the daily dry-run digest.
func htmlDryStats(ev Event) []statCard {
	return []statCard{
		{label: "scraped", value: itoa(dailyMatchCount(ev)), color: htmlAccent},
		{label: "matched", value: itoa(len(foundJobs(ev.Jobs))), color: htmlMatched},
		{label: "submitted", value: "0", color: htmlSkipped},
	}
}

func foundJobs(jobs []JobEvent) []JobEvent {
	var out []JobEvent
	for _, j := range jobs {
		if j.Status == "found" {
			out = append(out, j)
		}
	}
	return out
}

// htmlJobSections groups the digest's jobs into applied / failed / found lists.
func htmlJobSections(jobs []JobEvent) []htmlSection {
	var applied, failed, found []htmlRow
	for _, j := range jobs {
		r := htmlRow{title: j.Title, meta: j.Company, url: j.URL, reason: j.Reason}
		switch j.Status {
		case "applied":
			applied = append(applied, r)
		case "failed":
			failed = append(failed, r)
		case "found":
			found = append(found, r)
		}
	}
	var out []htmlSection
	if len(applied) > 0 {
		out = append(out, htmlSection{title: "✓ Applied", color: htmlApplied, rows: applied})
	}
	if len(failed) > 0 {
		out = append(out, htmlSection{title: "✗ Needs manual action", color: htmlFailed, rows: failed})
	}
	if len(found) > 0 {
		out = append(out, htmlSection{title: "✓ Matched — dry run", color: htmlMatched, rows: found})
	}
	return out
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func firstURL(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// htmlDigestDoc assembles a full digest email: header band, stats row,
// sections, optional note, and footer.
func htmlDigestDoc(titleLabel string, ts time.Time, stats []statCard, sections []htmlSection, note string) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\"></head><body style=\"margin:0;padding:0;background:" + htmlBodyBG + ";font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;\">")
	b.WriteString("<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"background:" + htmlBodyBG + ";padding:24px 0;\"><tr><td align=\"center\"><table role=\"presentation\" width=\"600\" cellpadding=\"0\" cellspacing=\"0\" style=\"max-width:600px;width:100%;background:#ffffff;border-radius:12px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.08);\">")

	fmt.Fprintf(&b, "<tr><td style=\"background:%s;padding:22px 28px;\">", htmlAccent)
	fmt.Fprintf(&b, "<div style=\"color:#ffffff;font-size:19px;font-weight:700;\">⚡ Nexus — %s</div>", esc(titleLabel))
	fmt.Fprintf(&b, "<div style=\"color:#C7D2FE;font-size:13px;margin-top:4px;\">%s</div></td></tr>",
		esc(ts.Format("Monday, Jan 2, 2006")))

	b.WriteString("<tr><td style=\"padding:20px 28px 6px;\">")
	htmlStatsRow(&b, stats)
	b.WriteString("</td></tr>")

	if len(sections) > 0 {
		b.WriteString("<tr><td style=\"padding:10px 28px 6px;\">")
		for _, s := range sections {
			fmt.Fprintf(&b, "<div style=\"font-size:15px;font-weight:700;color:%s;margin:14px 0 8px;\">%s</div>", s.color, esc(s.title))
			for _, r := range s.rows {
				htmlJobRow(&b, r)
			}
		}
		b.WriteString("</td></tr>")
	}

	if note != "" {
		fmt.Fprintf(&b, "<tr><td style=\"padding:14px 28px 4px;\"><div style=\"background:#FEF3C7;border:1px solid #FDE68A;border-radius:8px;padding:10px 12px;font-size:13px;color:#92400E;\">%s</div></td></tr>", esc(note))
	}

	b.WriteString("<tr><td style=\"background:" + htmlCardBG + ";border-top:1px solid " + htmlBorder + ";padding:14px 28px;\">")
	b.WriteString("<div style=\"font-size:12px;color:#9CA3AF;\">Sent by Nexus Job Assistant — your daily job-search nudge. Reply to this email or open the app to adjust notifications.</div>")
	b.WriteString("</td></tr></table></td></tr></table></body></html>")
	return b.String()
}

// htmlStatsRow renders the stat cards as equal columns.
func htmlStatsRow(b *strings.Builder, cards []statCard) {
	b.WriteString("<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\"><tr>")
	const gap = 8
	colW := 96/len(cards) - gap*2/len(cards)
	for i, c := range cards {
		if i > 0 {
			fmt.Fprintf(b, "<td width=\"%d\"></td>", gap)
		}
		fmt.Fprintf(b, "<td width=\"%d\" align=\"center\" style=\"background:%s;border-radius:10px;padding:12px 6px;\">", colW, cardBG(c.color))
		fmt.Fprintf(b, "<div style=\"font-size:24px;font-weight:700;color:%s;\">%s</div>", c.color, esc(c.value))
		fmt.Fprintf(b, "<div style=\"font-size:12px;color:%s;margin-top:2px;\">%s</div>", htmlTextGrey, esc(c.label))
		b.WriteString("</td>")
	}
	b.WriteString("</tr></table>")
}

// cardBG tints the stat-card background from the accent color.
func cardBG(color string) string {
	switch color {
	case htmlApplied:
		return "#ECFDF5"
	case htmlFailed:
		return "#FEF2F2"
	case htmlMatched:
		return "#FFFBEB"
	case htmlAccent:
		return "#EEF2FF"
	default:
		return htmlCardBG
	}
}

// htmlJobRow renders one job entry with an apply link and optional reason.
func htmlJobRow(b *strings.Builder, r htmlRow) {
	b.WriteString("<div style=\"border:1px solid " + htmlBorder + ";border-radius:10px;padding:10px 12px;margin-bottom:8px;\">")
	if r.url != "" {
		fmt.Fprintf(b, "<div style=\"font-size:14px;font-weight:600;color:%s;\"><a href=\"%s\" style=\"color:%s;text-decoration:none;\">%s</a></div>",
			htmlTextDark, esc(r.url), htmlAccent, esc(r.title))
	} else {
		fmt.Fprintf(b, "<div style=\"font-size:14px;font-weight:600;color:%s;\">%s</div>", htmlTextDark, esc(r.title))
	}
	if r.meta != "" {
		fmt.Fprintf(b, "<div style=\"font-size:12px;color:%s;margin-top:2px;\">%s</div>", htmlTextGrey, esc(r.meta))
	}
	if r.reason != "" {
		fmt.Fprintf(b, "<div style=\"font-size:12px;color:%s;margin-top:2px;\">reason: %s</div>", htmlFailed, esc(r.reason))
	}
	b.WriteString("</div>")
}

// htmlJobCard renders a single job (per-job opt-in emails).
func htmlJobCard(ev Event, applied bool) string {
	accent := htmlApplied
	label := "Applied"
	verb := "Nexus applied to this job"
	if !applied {
		accent = htmlFailed
		label = "Failed"
		verb = "Nexus could not apply to this job"
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\"></head><body style=\"margin:0;padding:0;background:" + htmlBodyBG + ";font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;\">")
	b.WriteString("<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"background:" + htmlBodyBG + ";padding:24px 0;\"><tr><td align=\"center\"><table role=\"presentation\" width=\"560\" cellpadding=\"0\" cellspacing=\"0\" style=\"max-width:560px;width:100%;background:#ffffff;border-radius:12px;overflow:hidden;\">")
	fmt.Fprintf(&b, "<tr><td style=\"background:%s;padding:20px 24px;\"><div style=\"color:#ffffff;font-size:17px;font-weight:700;\">%s</div></td></tr>", accent, esc(label))
	b.WriteString("<tr><td style=\"padding:20px 24px;\">")
	fmt.Fprintf(&b, "<div style=\"font-size:14px;color:%s;\">%s:</div>", htmlTextGrey, esc(verb))
	htmlJobRow(&b, htmlRow{title: ev.JobTitle, meta: ev.Company, url: ev.JobURL, reason: ev.Reason})
	if !applied && ev.JobURL != "" {
		fmt.Fprintf(&b, "<div style=\"margin-top:12px;font-size:13px;\"><a href=\"%s\" style=\"color:%s;font-weight:600;text-decoration:none;\">Open it and apply manually →</a></div>", esc(ev.JobURL), htmlAccent)
	}
	b.WriteString("</td></tr></table></td></tr></table></body></html>")
	return b.String()
}

// htmlAlertDoc renders a simple attention alert.
func htmlAlertDoc(title, lead, who, what, detail, url string) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><meta charset=\"utf-8\"></head><body style=\"margin:0;padding:0;background:" + htmlBodyBG + ";font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Arial,sans-serif;\">")
	b.WriteString("<table role=\"presentation\" width=\"100%\" cellpadding=\"0\" cellspacing=\"0\" style=\"background:" + htmlBodyBG + ";padding:24px 0;\"><tr><td align=\"center\"><table role=\"presentation\" width=\"560\" cellpadding=\"0\" cellspacing=\"0\" style=\"max-width:560px;width:100%;background:#ffffff;border-radius:12px;overflow:hidden;\">")
	fmt.Fprintf(&b, "<tr><td style=\"background:%s;padding:20px 24px;\"><div style=\"color:#ffffff;font-size:17px;font-weight:700;\">%s</div></td></tr>", htmlFailed, esc(title))
	b.WriteString("<tr><td style=\"padding:20px 24px;\">")
	fmt.Fprintf(&b, "<div style=\"font-size:14px;color:%s;\">%s</div>", htmlTextDark, esc(lead))
	if who != "" {
		fmt.Fprintf(&b, "<div style=\"font-size:13px;color:%s;margin-top:10px;\">%s</div>", htmlTextGrey, esc(who))
	}
	if what != "" {
		fmt.Fprintf(&b, "<div style=\"font-size:13px;color:%s;margin-top:2px;\">%s</div>", htmlTextGrey, esc(what))
	}
	if detail != "" {
		fmt.Fprintf(&b, "<div style=\"font-size:13px;color:%s;margin-top:10px;background:%s;border-radius:8px;padding:10px 12px;\">%s</div>", htmlTextDark, htmlCardBG, esc(detail))
	}
	if url != "" {
		fmt.Fprintf(&b, "<div style=\"margin-top:12px;font-size:13px;\"><a href=\"%s\" style=\"color:%s;font-weight:600;text-decoration:none;\">Open the page →</a></div>", esc(url), htmlAccent)
	}
	b.WriteString("</td></tr></table></td></tr></table></body></html>")
	return b.String()
}

// esc HTML-escapes a value so scraped platform text can't inject markup.
func esc(s string) string {
	return html.EscapeString(s)
}
