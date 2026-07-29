package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthanmanthan/nexus/internal/store"
	"github.com/manthanmanthan/nexus/internal/textutil"
)

type RefreshHistoryMsg struct{ Apps []store.Application }

// HistoryEnrichRequestMsg asks App to backfill description(+fit) for one or all missing.
type HistoryEnrichRequestMsg struct {
	All bool // false = selected only
	App store.Application
}

type historyEnrichDoneMsg struct {
	Updated int
	Failed  int
	Err     error
	Status  string
}

// historyEnrichProgressMsg streams one log line while backfill runs (UI stays interactive).
type historyEnrichProgressMsg struct {
	Line string
	Next tea.Cmd
}

type HistoryModel struct {
	width        int
	height       int
	apps         []store.Application
	cursor       int
	loading      bool
	detail       bool // show detail pane for selected row
	detailVP     viewport.Model
	detailReady  bool
	enriching    bool
	enrichStatus string
	search       textinput.Model
	searching    bool // search box focused
}

func NewHistoryModel() HistoryModel {
	ti := textinput.New()
	ti.Placeholder = "company, role, provider, status, location…"
	ti.CharLimit = 120
	ti.Width = 48
	ti.Prompt = "/ "
	return HistoryModel{loading: true, search: ti}
}

// CapturesKeys is true while search is focused or a job detail is open.
func (m HistoryModel) CapturesKeys() bool {
	return m.searching || m.detail
}

func (m HistoryModel) Init() tea.Cmd { return nil }

func (m HistoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = max(24, msg.Width-18)
		m = m.syncDetailViewport()
		if m.detail {
			m = m.refreshDetailContent(false)
		}

	case RefreshHistoryMsg:
		m.apps = msg.Apps
		m.loading = false
		m = m.clampCursor()
		if m.detail {
			m = m.refreshDetailContent(false)
		}

	case historyEnrichDoneMsg:
		m.enriching = false
		if msg.Err != nil {
			m.enrichStatus = "enrich failed: " + msg.Err.Error()
		} else if msg.Status != "" {
			m.enrichStatus = msg.Status
		} else {
			m.enrichStatus = fmt.Sprintf("updated %d · failed %d", msg.Updated, msg.Failed)
		}
		return m, nil

	case tea.KeyMsg:
		if m.detail {
			switch msg.String() {
			case "esc", "q", "backspace":
				m.detail = false
				return m, nil
			case "u":
				if m.enriching {
					return m, nil
				}
				if app, ok := m.selectedApp(); ok {
					m.enriching = true
					m.enrichStatus = "fetching description…"
					return m, func() tea.Msg {
						return HistoryEnrichRequestMsg{All: false, App: app}
					}
				}
				return m, nil
			}
			m = m.syncDetailViewport()
			var cmd tea.Cmd
			m.detailVP, cmd = m.detailVP.Update(msg)
			return m, cmd
		}

		if m.searching {
			switch msg.String() {
			case "esc":
				m.searching = false
				m.search.Blur()
				return m, nil
			case "enter":
				m.searching = false
				m.search.Blur()
				m = m.clampCursor()
				if len(m.filtered()) > 0 {
					m.detail = true
					m = m.syncDetailViewport()
					m = m.refreshDetailContent(true)
				}
				return m, nil
			case "ctrl+u":
				m.search.SetValue("")
				m = m.clampCursor()
				return m, nil
			case "up", "down", "ctrl+j", "ctrl+k":
				// Leave search focus and navigate results.
				m.searching = false
				m.search.Blur()
				if msg.String() == "down" || msg.String() == "ctrl+j" {
					m.moveCursor(+1)
				} else {
					m.moveCursor(-1)
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m = m.clampCursor()
			return m, cmd
		}

		switch msg.String() {
		case "/":
			m.searching = true
			return m, m.search.Focus()
		case "esc":
			if strings.TrimSpace(m.search.Value()) != "" {
				m.search.SetValue("")
				m = m.clampCursor()
				return m, nil
			}
		case "j", "down":
			m.moveCursor(+1)
		case "k", "up":
			m.moveCursor(-1)
		case "g":
			m.cursor = 0
		case "G":
			f := m.filtered()
			m.cursor = max(0, len(f)-1)
		case "enter", " ":
			if len(m.filtered()) > 0 {
				m.detail = true
				m = m.syncDetailViewport()
				m = m.refreshDetailContent(true)
			}
		case "u":
			if m.enriching {
				return m, nil
			}
			if app, ok := m.selectedApp(); ok {
				m.enriching = true
				m.enrichStatus = "fetching description for selected…"
				return m, func() tea.Msg {
					return HistoryEnrichRequestMsg{All: false, App: app}
				}
			}
		case "U":
			if m.enriching {
				return m, nil
			}
			m.enriching = true
			m.enrichStatus = "backfilling all empty descriptions…"
			return m, func() tea.Msg {
				return HistoryEnrichRequestMsg{All: true}
			}
		}
	}
	return m, nil
}

func (m *HistoryModel) moveCursor(delta int) {
	f := m.filtered()
	if len(f) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(f) {
		m.cursor = len(f) - 1
	}
}

func (m HistoryModel) clampCursor() HistoryModel {
	f := m.filtered()
	if len(f) == 0 {
		m.cursor = 0
		return m
	}
	if m.cursor >= len(f) {
		m.cursor = len(f) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	return m
}

func (m HistoryModel) filtered() []store.Application {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if q == "" {
		return m.apps
	}
	tokens := strings.Fields(q)
	var out []store.Application
	for _, a := range m.apps {
		if jobMatchesQuery(a, tokens) {
			out = append(out, a)
		}
	}
	return out
}

func jobMatchesQuery(a store.Application, tokens []string) bool {
	hay := strings.ToLower(strings.Join([]string{
		a.Company,
		a.Role,
		a.Provider,
		string(a.Status),
		a.Location,
		a.URL,
		a.Reason,
		a.FitSummary,
		fmt.Sprintf("%d", a.FitScore),
	}, " "))
	for _, tok := range tokens {
		if !strings.Contains(hay, tok) {
			return false
		}
	}
	return true
}

func (m HistoryModel) selectedApp() (store.Application, bool) {
	f := m.filtered()
	if len(f) == 0 || m.cursor < 0 || m.cursor >= len(f) {
		return store.Application{}, false
	}
	return f[m.cursor], true
}

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
	b.WriteString("  " + mutedStyle.Render("Scraped & applied roles — search, open a row for full JD + fit.") + "\n\n")

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
	help := "  / search  •  j/k navigate  •  enter details  •  u refresh desc  •  U backfill empty  •  r reload"
	if m.searching {
		help = "  typing search…  •  enter open job  •  esc leave search  •  ctrl+u clear"
	} else if m.enriching {
		help = "  backfill running…  •  j/k navigate  •  see Logs for progress  •  u/U locked"
	}
	b.WriteString(mutedStyle.Render(help))

	return b.String()
}

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
	parts := []string{"j/k scroll", "esc back", "u refresh description"}
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
	default:
		return string(s)
	}
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
