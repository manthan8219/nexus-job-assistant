package ui

// Package ui — history_detail.go
// The Jobs/History tab's detail pane: the scrollable job-description viewport
// and the full JD + fit rendering for the selected row. The list view lives in
// history_view.go; the model + Update in history.go.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
	"github.com/manthan8219/nexus-job-assistant/internal/textutil"
)

func (m HistoryModel) syncDetailViewport() HistoryModel {
	vh := m.height - 3
	if vh < 5 {
		vh = 5
	}
	vw := m.width - 2
	if vw < 40 {
		vw = 40
	}
	if !m.detailReady {
		m.detailVP = viewport.New(vw, vh)
		m.detailReady = true
	} else {
		m.detailVP.Width = vw
		m.detailVP.Height = vh
	}
	return m
}

func (m HistoryModel) refreshDetailContent(gotoTop bool) HistoryModel {
	app, ok := m.selectedApp()
	if !ok {
		return m
	}
	m = m.syncDetailViewport()
	w := m.width
	if w == 0 {
		w = 80
	}
	m.detailVP.SetContent(m.detailContent(app, w))
	if gotoTop {
		m.detailVP.GotoTop()
	}
	return m
}

func (m HistoryModel) detailHelp() string {
	parts := []string{"j/k scroll", "esc back", "o outcome", "u refresh description"}
	if m.detailReady && m.detailVP.TotalLineCount() > m.detailVP.Height {
		pct := int(m.detailVP.ScrollPercent() * 100)
		parts = append(parts, fmt.Sprintf("%d%%", pct))
		if !m.detailVP.AtBottom() {
			parts = append(parts, "more ↓")
		}
	}
	if m.enriching {
		parts = append(parts, "backfill running…")
	}
	return strings.Join(parts, "  ·  ")
}

func (m HistoryModel) detailContent(app store.Application, w int) string {
	var b strings.Builder
	b.WriteString("\n")

	b.WriteString("  " + labelStyle.Render("JOB DETAILS") + "\n\n")

	b.WriteString("  " + lipgloss.NewStyle().Bold(true).Render(app.Role) + "\n")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(app.Company) + "\n\n")

	type field struct{ label, value string }
	fields := []field{
		{"Provider", app.Provider},
		{"Status", string(app.Status)},
		{"Location", locationStr(app)},
		{"Applied", app.AppliedAt.Format("Jan 02, 2006 15:04")},
	}
	if app.Outcome != store.OutcomeNone && app.Outcome != "" {
		outcomeVal := string(app.Outcome)
		if !app.OutcomeAt.IsZero() && app.OutcomeAt.Year() > 1 {
			outcomeVal += " · " + app.OutcomeAt.Format("Jan 02, 2006")
		}
		fields = append(fields, field{"Outcome", outcomeVal})
	}
	if app.FitScore > 0 {
		fields = append(fields, field{"Fit", fmt.Sprintf("%d / 100 shortlist chance", app.FitScore)})
	}
	if !app.PostedAt.IsZero() && app.PostedAt.Year() > 1 {
		fields = append(fields, field{"Posted", app.PostedAt.Format("Jan 02, 2006")})
	}
	if app.Reason != "" {
		fields = append(fields, field{"Reason", app.Reason})
	}

	labelW := 10
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		lbl := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreyMid)).Width(labelW).Render(f.label)
		val := primaryStyle.Render(f.value)
		b.WriteString("  " + lbl + "  " + val + "\n")
	}

	b.WriteString("\n")
	urlLbl := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreyMid)).Width(labelW).Render("URL")
	urlVal := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render(app.URL)
	b.WriteString("  " + urlLbl + "  " + urlVal + "\n")

	if strings.TrimSpace(app.FitSummary) != "" {
		b.WriteString("\n  " + labelStyle.Render("WHY THIS SCORE") + "\n\n")
		sum := wrapText(app.FitSummary, w-6)
		for _, line := range strings.Split(sum, "\n") {
			b.WriteString("  " + primaryStyle.Render(line) + "\n")
		}
	}

	if strings.TrimSpace(app.Description) != "" {
		b.WriteString("\n  " + labelStyle.Render("DESCRIPTION") + "\n\n")
		desc := wrapText(textutil.HTMLToPlain(app.Description), w-6)
		for _, line := range strings.Split(desc, "\n") {
			b.WriteString("  " + mutedStyle.Render(line) + "\n")
		}
	} else {
		b.WriteString("\n  " + labelStyle.Render("DESCRIPTION") + "\n\n")
		b.WriteString("  " + mutedStyle.Render("No job description was scraped for this posting.") + "\n")
		b.WriteString("  " + mutedStyle.Render("Fit score (if any) used title / company / location only.") + "\n")
	}

	b.WriteString("\n")
	return b.String()
}

func locationStr(app store.Application) string {
	if app.Location == "" && app.Remote {
		return "Remote"
	}
	if app.Location != "" && app.Remote {
		return app.Location + " (Remote)"
	}
	return app.Location
}

// wrapText wraps s at word boundaries to maxWidth.
func wrapText(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	var out strings.Builder
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out.WriteString("\n")
			continue
		}
		line := ""
		for _, w := range words {
			if line == "" {
				line = w
			} else if len(line)+1+len(w) <= maxWidth {
				line += " " + w
			} else {
				out.WriteString(line + "\n")
				line = w
			}
		}
		if line != "" {
			out.WriteString(line + "\n")
		}
	}
	return strings.TrimRight(out.String(), "\n")
}
