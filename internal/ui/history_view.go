package ui

// Package ui — history_view.go
// The Jobs/History tab's main view: the job list table (columns, badges, row
// styling) plus the outcome funnel line. Update + selection/filtering live in
// history.go; the detail pane in history_detail.go.

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func (m HistoryModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	// Reserve room for title, search, header, help.
	h := m.height - 12
	if h < 5 {
		h = 5
	}

	if m.detail {
		if app, ok := m.selectedApp(); ok {
			var b strings.Builder
			if !m.detailReady {
				b.WriteString(m.detailContent(app, w))
			} else {
				b.WriteString(m.detailVP.View())
			}
			b.WriteString("\n")
			b.WriteString(mutedStyle.Render("  " + m.detailHelp()))
			return b.String()
		}
	}

	var b strings.Builder
	filtered := m.filtered()

	title := labelStyle.Render("JOBS")
	count := mutedStyle.Render(fmt.Sprintf("(%d shown · %d total)", len(filtered), len(m.apps)))
	b.WriteString("\n  " + title + "  " + count + "\n")
	b.WriteString("  " + mutedStyle.Render("Discovered & applied roles — search, open a row for full JD + fit.") + "\n")
	if funnel := m.funnelLine(); funnel != "" {
		b.WriteString("  " + funnel + "\n")
	}
	b.WriteString("\n")

	searchLine := m.search.View()
	if !m.searching && strings.TrimSpace(m.search.Value()) == "" {
		searchLine = mutedStyle.Render("/  press / to search jobs…")
	}
	b.WriteString("  " + searchLine + "\n\n")

	if m.loading {
		b.WriteString(mutedStyle.Render("  Loading..."))
		return b.String()
	}

	if len(m.apps) == 0 {
		b.WriteString(mutedStyle.Render("  No jobs yet. Start the engine from the Dashboard tab."))
		return b.String()
	}

	if len(filtered) == 0 {
		b.WriteString(mutedStyle.Render("  No jobs match this search. Esc clears · / edits query."))
		return b.String()
	}

	cw := colWidths(w)
	header := labelStyle.Render(fmt.Sprintf("  %-*s  %-*s  %-*s  %-10s  %-4s  %s",
		cw.company, "Company",
		cw.role, "Role",
		cw.provider, "Provider",
		"Status", "Fit", "Date",
	))
	b.WriteString(header + "\n")
	b.WriteString(mutedStyle.Render("  "+strings.Repeat("─", w-6)) + "\n")

	visible := visibleRows(filtered, m.cursor, h)
	for _, app := range visible {
		isSelected := indexOf(filtered, app) == m.cursor
		row := renderAppRow(app, cw, isSelected)
		b.WriteString(row + "\n")
	}

	b.WriteString("\n")
	if m.enrichStatus != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render(m.enrichStatus) + "\n")
	}
	help := "  / search  •  j/k navigate  •  enter details  •  o outcome  •  u refresh desc  •  U backfill  •  r reload"
	if m.searching {
		help = "  typing search…  •  enter open job  •  esc leave search  •  ctrl+u clear"
	} else if m.enriching {
		help = "  backfill running…  •  j/k navigate  •  see Logs for progress  •  u/U locked"
	}
	b.WriteString(mutedStyle.Render(help))

	return b.String()
}

// funnelLine renders the outcome funnel under the title — the answer to
// "is anyone answering me?" at a glance.
func (m HistoryModel) funnelLine() string {
	if len(m.outcomes) == 0 {
		return ""
	}
	type seg struct {
		key   store.Outcome
		label string
	}
	parts := []string{}
	for _, s := range []seg{
		{store.OutcomeReplied, "replied"},
		{store.OutcomeInterview, "interview"},
		{store.OutcomeOffer, "offer"},
		{store.OutcomeRejected, "rejected"},
		{store.OutcomeGhosted, "ghosted"},
	} {
		if n := m.outcomes[s.key]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %s %d", outcomeDot(s.key), s.label, n))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return primaryStyle.Render("pipeline: ") + strings.Join(parts, mutedStyle.Render("  ·  "))
}

type colW struct{ company, role, provider, status, date int }

func colWidths(w int) colW {
	avail := w - 10
	return colW{
		company:  max(15, avail*25/100),
		role:     max(20, avail*35/100),
		provider: 12,
		status:   10,
		date:     10,
	}
}

func renderRow(company, role, provider, status, date string, cw colW, _ bool) string {
	return mutedStyle.Render(fmt.Sprintf("%-*s  %-*s  %-*s  %-*s  %s",
		cw.company, truncate(company, cw.company),
		cw.role, truncate(role, cw.role),
		cw.provider, truncate(provider, cw.provider),
		cw.status, status,
		date,
	))
}

func renderAppRow(app store.Application, cw colW, selected bool) string {
	statusStyled := statusBadge(app.Status)
	if app.Outcome != store.OutcomeNone && app.Outcome != "" {
		statusStyled = outcomeBadge(app.Outcome)
	}
	date := app.AppliedAt.Format("Jan 02")

	fit := "  —"
	if app.FitScore > 0 {
		fit = fmt.Sprintf("%3d", app.FitScore)
	}
	base := fmt.Sprintf("  %-*s  %-*s  %-*s  %-10s  %-4s  %s",
		cw.company, truncate(app.Company, cw.company),
		cw.role, truncate(app.Role, cw.role),
		cw.provider, truncate(app.Provider, cw.provider),
		statusStyled,
		fit,
		mutedStyle.Render(date),
	)

	if selected {
		return lipgloss.NewStyle().
			Background(lipgloss.AdaptiveColor{Light: "#EDE9FE", Dark: "#1E1B4B"}).
			Render(base)
	}
	return primaryStyle.Render(base)
}

func statusBadge(s store.Status) string {
	switch s {
	case store.StatusApplied:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("applied   ")
	case store.StatusSkipped:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreyMid)).Render("skipped   ")
	case store.StatusFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render("failed    ")
	case store.StatusQueued:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Render("queued    ")
	default:
		return string(s)
	}
}

// outcomeBadge renders the pipeline stage in the status column (10 cells).
func outcomeBadge(o store.Outcome) string {
	style := func(color string, bold bool) lipgloss.Style {
		s := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
		return s.Bold(bold)
	}
	switch o {
	case store.OutcomeReplied:
		return style(colorGreen, false).Render("replied   ")
	case store.OutcomeInterview:
		return style(colorPurple, true).Render("interview ")
	case store.OutcomeOffer:
		return style(colorGreen, true).Render("offer 🏆  ")
	case store.OutcomeRejected:
		return style(colorRed, false).Render("rejected  ")
	case store.OutcomeGhosted:
		return style(colorGreyMid, false).Render("ghosted   ")
	default:
		return string(o)
	}
}

// outcomeDot is a small colored marker used in the funnel line.
func outcomeDot(o store.Outcome) string {
	color := colorGreyMid
	switch o {
	case store.OutcomeReplied, store.OutcomeOffer:
		color = colorGreen
	case store.OutcomeInterview:
		color = colorPurple
	case store.OutcomeRejected:
		color = colorRed
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("●")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func visibleRows(apps []store.Application, cursor, height int) []store.Application {
	if len(apps) == 0 {
		return nil
	}
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > len(apps) {
		end = len(apps)
		start = max(0, end-height)
	}
	return apps[start:end]
}

func indexOf(apps []store.Application, target store.Application) int {
	for i, a := range apps {
		if a.ID == target.ID {
			return i
		}
	}
	return -1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return t.Format("Jan 02")
	}
}

var _ = timeAgo // used later
