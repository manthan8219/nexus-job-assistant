package ui

// Package ui — companies_view.go
// The Companies tab rendering: the company list view, the scraped-jobs detail
// view, column helpers and badges. The model + Update live in companies_tab.go
// and the modal key handlers in companies_keys.go.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func (m CompaniesTabModel) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	var b strings.Builder
	b.WriteString("\n  " + labelStyle.Render("COMPANIES") + "  " + m.resultsSummary() + "\n")
	b.WriteString("  " + mutedStyle.Render("OpenJobs + ATS boards + India priority + Y Combinator + manual. JOBS = discovered roles recorded · enter opens them.") + "\n\n")

	if m.adding {
		b.WriteString("  " + labelStyle.Render("ADD COMPANY") + "\n")
		labels := []string{"Name", "Website", "Board URL", "Countries", "ATS hint"}
		for i := 0; i < caCount; i++ {
			mark := " "
			if i == m.addFocus {
				mark = "▸"
			}
			b.WriteString(fmt.Sprintf("  %s %-10s %s\n", mark, labels[i], m.add[i].View()))
		}
		b.WriteString("\n  " + mutedStyle.Render("tab fields · enter save · esc cancel") + "\n")
		if m.err != "" {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(m.err) + "\n")
		}
		return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(b.String())
	}

	if m.detail {
		return m.detailView(w, h)
	}

	b.WriteString("  " + m.search.View() + "\n")
	b.WriteString("  " + m.country.View() + "\n")
	if m.status != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render(m.status) + "\n")
	} else {
		b.WriteString("\n")
	}
	if m.err != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(m.err) + "\n")
	}

	headerH := lipgloss.Height(b.String())
	listH := h - headerH
	if listH < 5 {
		listH = 5
	}

	var list strings.Builder
	if m.loading {
		list.WriteString(mutedStyle.Render("  Loading…"))
	} else if len(m.items) == 0 {
		list.WriteString(mutedStyle.Render("  No match. Press c → India for hire-country, or a to add a company.") + "\n")
	} else {
		list.WriteString(mutedStyle.Render(fmt.Sprintf("  %-26s  %-12s  %-14s  %-8s  %5s  %s", "NAME", "ATS", "BOARD", "COUNTRY", "JOBS", "SOURCE")) +
			mutedStyle.Render(fmt.Sprintf("   %d/%d", m.cursor+1, len(m.items))) + "\n")
		rows := listH - 1
		if rows < 3 {
			rows = 3
		}
		start := m.cursor - rows/2
		if start < 0 {
			start = 0
		}
		end := start + rows
		if end > len(m.items) {
			end = len(m.items)
		}
		if end-start < rows && start > 0 {
			start = max(0, end-rows)
		}
		for i := start; i < end; i++ {
			c := m.items[i]
			ats := c.ATS
			if ats == "" {
				ats = "—"
			}
			countries := strings.Join(c.HireCountryCodes, ",")
			if countries == "" {
				countries = "—"
			}
			jobsCell := mutedStyle.Render(fmt.Sprintf("%5s", "—"))
			if n := m.jobCount(c); n > 0 {
				jobsCell = lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorOrange)).Bold(true).
					Render(fmt.Sprintf("%5d", n))
			}
			line := fmt.Sprintf("  %-26s  %-12s  %-14s  %-8s  ", truncate(c.Name, 26), ats, truncate(c.Board, 14), truncate(countries, 8)) +
				jobsCell + "  " + primaryStyle.Render(truncate(c.Source, 14))
			if i == m.cursor {
				line = lipgloss.NewStyle().
					Background(lipgloss.AdaptiveColor{Light: "#EDE9FE", Dark: "#1E1B4B"}).
					Width(max(20, w-1)).
					Render(line)
			} else {
				line = primaryStyle.Render(line)
			}
			list.WriteString(line + "\n")
		}
	}

	listStr := lipgloss.NewStyle().Height(listH).MaxHeight(listH).Width(w).Render(list.String())
	out := b.String() + listStr
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(out)
}

// detailView renders the scraped jobs recorded for the selected company.
func (m CompaniesTabModel) detailView(w, h int) string {
	c := m.detailCompany
	var b strings.Builder

	b.WriteString("\n  " + labelStyle.Render("◂ "+strings.ToUpper(truncate(c.Name, 40))))

	applied, skipped, failed := 0, 0, 0
	for _, j := range m.detailJobs {
		switch j.Status {
		case store.StatusApplied:
			applied++
		case store.StatusSkipped:
			skipped++
		case store.StatusFailed:
			failed++
		}
	}
	countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true)
	if m.detailLoading {
		b.WriteString("  " + mutedStyle.Render("loading discovered jobs…") + "\n")
	} else {
		word := "jobs"
		if len(m.detailJobs) == 1 {
			word = "job"
		}
		b.WriteString("  " + countStyle.Render(fmt.Sprintf("%d discovered %s", len(m.detailJobs), word)) +
			mutedStyle.Render(fmt.Sprintf("  ·  %d applied  ·  %d skipped  ·  %d failed", applied, skipped, failed)) + "\n")
	}

	var meta []string
	if c.ATS != "" {
		s := c.ATS
		if c.Board != "" {
			s += "/" + c.Board
		}
		meta = append(meta, s)
	}
	if c.Website != "" {
		meta = append(meta, c.Website)
	}
	if len(c.HireCountryCodes) > 0 {
		meta = append(meta, "hires: "+strings.Join(c.HireCountryCodes, ", "))
	}
	if len(meta) > 0 {
		b.WriteString("  " + mutedStyle.Render(truncate(strings.Join(meta, "  ·  "), max(20, w-4))) + "\n")
	}
	b.WriteString("\n")

	headerH := lipgloss.Height(b.String())
	listH := h - headerH - 2
	if listH < 5 {
		listH = 5
	}

	var list strings.Builder
	switch {
	case m.detailLoading:
		list.WriteString(mutedStyle.Render("  Loading…"))
	case len(m.detailJobs) == 0:
		list.WriteString(mutedStyle.Render("  No discovered jobs recorded for this company yet. Run a search — matching roles will show up here."))
	default:
		cw := companyJobColWidths(w)
		list.WriteString(mutedStyle.Render(fmt.Sprintf("  %-*s  %-10s  %-4s  %-*s  %-9s  %s",
			cw.role, "ROLE", "STATUS", "FIT", cw.location, "LOCATION", "POSTED", "PROVIDER")) +
			mutedStyle.Render(fmt.Sprintf("   %d/%d", m.detailCursor+1, len(m.detailJobs))) + "\n")
		rows := listH - 1
		if rows < 3 {
			rows = 3
		}
		start := m.detailCursor - rows/2
		if start < 0 {
			start = 0
		}
		end := start + rows
		if end > len(m.detailJobs) {
			end = len(m.detailJobs)
		}
		if end-start < rows && start > 0 {
			start = max(0, end-rows)
		}
		for i := start; i < end; i++ {
			list.WriteString(renderCompanyJobRow(m.detailJobs[i], cw, i == m.detailCursor, w) + "\n")
		}
	}

	listStr := lipgloss.NewStyle().Height(listH).MaxHeight(listH).Width(w).Render(list.String())
	out := b.String() + listStr
	if !m.detailLoading && m.detailCursor < len(m.detailJobs) {
		if u := strings.TrimSpace(m.detailJobs[m.detailCursor].URL); u != "" {
			out += "\n  " + mutedStyle.Render(truncate(u, max(20, w-4)))
		}
	}
	if m.err != "" {
		out += "\n  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(m.err)
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(out)
}

type companyJobColW struct{ role, location int }

func companyJobColWidths(w int) companyJobColW {
	avail := w - 47
	if avail < 40 {
		avail = 40
	}
	role := avail * 55 / 100
	if role < 20 {
		role = 20
	}
	location := avail - role
	if location < 12 {
		location = 12
	}
	return companyJobColW{role: role, location: location}
}

func renderCompanyJobRow(j store.Application, cw companyJobColW, selected bool, width int) string {
	fit := "  —"
	if j.FitScore > 0 {
		fit = fmt.Sprintf("%3d", j.FitScore)
	}
	posted := j.PostedAt
	if posted.IsZero() {
		posted = j.AppliedAt
	}
	date := "—"
	if !posted.IsZero() {
		date = posted.Format("Jan 02")
	}
	prov := j.Provider
	if prov == "" {
		prov = "—"
	}
	base := fmt.Sprintf("  %-*s  %s  %-4s  %-*s  %-9s  %s",
		cw.role, truncate(j.Role, cw.role),
		statusBadge(j.Status),
		fit,
		cw.location, truncate(jobLocationLabel(j), cw.location),
		date,
		truncate(prov, 12),
	)
	if selected {
		return lipgloss.NewStyle().
			Background(lipgloss.AdaptiveColor{Light: "#EDE9FE", Dark: "#1E1B4B"}).
			Width(max(20, width-1)).
			Render(base)
	}
	return primaryStyle.Render(base)
}

// jobLocationLabel shows the job location, tagging remote roles with a marker.
func jobLocationLabel(j store.Application) string {
	loc := strings.TrimSpace(j.Location)
	if loc == "" {
		if j.Remote {
			return "Remote"
		}
		return "—"
	}
	if j.Remote && !strings.Contains(strings.ToLower(loc), "remote") {
		return loc + " · R"
	}
	return loc
}

func (m CompaniesTabModel) resultsSummary() string {
	q := strings.TrimSpace(m.search.Value())
	ctry := strings.TrimSpace(m.country.Value())
	n := len(m.items)
	countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true)
	var filter string
	switch {
	case q != "" && ctry != "":
		filter = fmt.Sprintf(" matching %q in %s", q, ctry)
	case ctry != "":
		filter = fmt.Sprintf(" hiring in %s", ctry)
	case q != "":
		filter = fmt.Sprintf(" matching %q", q)
	}
	if m.loading {
		return mutedStyle.Render("loading…")
	}
	if n == 0 {
		return countStyle.Render("0 results") + mutedStyle.Render(filter+fmt.Sprintf(" · %d in database", m.total))
	}
	word := "results"
	if n == 1 {
		word = "result"
	}
	return countStyle.Render(fmt.Sprintf("%d %s", n, word)) + mutedStyle.Render(filter+fmt.Sprintf(" · %d in database", m.total))
}

func (m CompaniesTabModel) FooterHint() string {
	if m.adding {
		return "adding company  ·  tab fields  ·  enter save  ·  esc cancel  ·  ctrl+c quit"
	}
	if m.detail {
		return "discovered jobs  ·  j/k move  ·  o open in browser  ·  esc/q back  ·  ctrl+c quit"
	}
	if m.CapturesKeys() {
		return "filtering  ·  enter apply  ·  esc done  ·  ctrl+c quit"
	}
	if m.refreshing {
		return "fetching from network…  ·  ctrl+c quit"
	}
	return "/ name search  ·  c country (India)  ·  enter view jobs  ·  a add  ·  r reload  ·  R refetch from network  ·  j/k move  ·  esc tab mode  ·  ctrl+c quit"
}
