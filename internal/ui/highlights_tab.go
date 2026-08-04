package ui

// Package ui - highlights_tab.go
// The Inbox/Highlights tab: shows hiring-related email signals (interviews,
// offers, rejections, recruiter outreach, assessments, application
// acknowledgements) discovered by the inbox scan (see internal/inbox).

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/inbox"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// ── Messages ──────────────────────────────────────────────────────────────────

type highlightsLoadedMsg struct {
	hs  []inbox.Highlight
	err error
}

type highlightsScanDoneMsg struct {
	hs  []inbox.Highlight
	err error
}

// ── Model ─────────────────────────────────────────────────────────────────────

// HighlightsTabModel is the TUI view for hiring-email signals.
type HighlightsTabModel struct {
	width, height int
	highlights    []inbox.Highlight
	cursor        int
	scanning      bool
	statusLine    string
	errLine       string

	cfg *config.Config
	st  *store.Store
}

func NewHighlightsTabModel() HighlightsTabModel {
	return HighlightsTabModel{cursor: 0}
}

func (m *HighlightsTabModel) SetConfig(cfg *config.Config) { m.cfg = cfg }
func (m *HighlightsTabModel) SetStore(st *store.Store)     { m.st = st }

// CapturesKeys returns true so the tab keeps keys while focused.
func (m HighlightsTabModel) CapturesKeys() bool { return true }

// Focus re-activates the tab when the user presses enter to leave chromeNav.
func (m HighlightsTabModel) Focus() HighlightsTabModel { return m }

func (m HighlightsTabModel) Init() tea.Cmd { return m.cmdLoad() }

func (m HighlightsTabModel) cmdLoad() tea.Cmd {
	return func() tea.Msg {
		p, err := inbox.HighlightsPath()
		if err != nil {
			return highlightsLoadedMsg{err: err}
		}
		hs, err := inbox.LoadAll(p)
		return highlightsLoadedMsg{hs: hs, err: err}
	}
}

func (m HighlightsTabModel) cmdScan() tea.Cmd {
	return func() tea.Msg {
		if m.cfg == nil {
			return highlightsScanDoneMsg{err: fmt.Errorf("config not loaded")}
		}
		fetcher := outreach.NewGmailIMAPFetcher(m.cfg)
		if fetcher == nil {
			return highlightsScanDoneMsg{err: fmt.Errorf("inbox scan needs your Email + Gmail app password (Config -> Outreach)")}
		}
		days, max := inbox.DefaultScanDays, inbox.DefaultScanMax
		if m.cfg.InboxScanDays > 0 {
			days = m.cfg.InboxScanDays
		}
		if m.cfg.InboxScanMax > 0 {
			max = m.cfg.InboxScanMax
		}
		hs, err := inbox.Scan(context.Background(), days, max, fetcher, m.st)
		if err != nil {
			return highlightsScanDoneMsg{err: err}
		}
		p, perr := inbox.HighlightsPath()
		if perr != nil {
			return highlightsScanDoneMsg{err: perr}
		}
		for _, h := range hs {
			_ = inbox.Upsert(p, h)
		}
		all, _ := inbox.LoadAll(p)
		return highlightsScanDoneMsg{hs: all}
	}
}

func (m HighlightsTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case highlightsLoadedMsg:
		m.scanning = false
		if msg.err != nil {
			m.errLine = msg.err.Error()
		} else {
			m.errLine = ""
			m.highlights = msg.hs
			m.statusLine = fmt.Sprintf("%d signal(s)", len(msg.hs))
			if m.cursor >= len(m.highlights) {
				m.cursor = len(m.highlights) - 1
			}
		}

	case highlightsScanDoneMsg:
		m.scanning = false
		if msg.err != nil {
			m.errLine = msg.err.Error()
		} else {
			m.errLine = ""
			m.highlights = msg.hs
			m.statusLine = fmt.Sprintf("scan complete - %d signal(s)", len(msg.hs))
			if m.cursor >= len(m.highlights) {
				m.cursor = len(m.highlights) - 1
			}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.highlights)-1 {
				m.cursor++
			}
		case "r", "R":
			cmds = append(cmds, m.cmdLoad())
		case "s", "S":
			if !m.scanning {
				m.scanning = true
				m.statusLine = "scanning inbox..."
				cmds = append(cmds, m.cmdScan())
			}
		}
	}
	return m, tea.Batch(cmds...)
}

func (m HighlightsTabModel) FooterHint() string {
	return "↑↓ navigate  ·  s scan now  ·  r refresh  ·  esc tab mode  ·  ctrl+c quit"
}

func (m HighlightsTabModel) View() string {
	var b strings.Builder

	if !m.cfgOk() {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).
			Render("Inbox scan needs your Gmail configured (Config -> Outreach).") + "\n")
		return b.String()
	}

	if m.statusLine != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).Render(m.statusLine) + "\n\n")
	}
	if m.errLine != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(m.errLine) + "\n\n")
	}

	if len(m.highlights) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).
			Render("  No hiring signals yet. Press s to scan the inbox, or run 'nexus --scan-inbox'.") + "\n")
		return b.String()
	}

	colW := [5]int{11, 20, 42, 24, 8}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorGrey)).Render(
		fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
			colW[0], "Signal", colW[1], "Company", colW[2], "Subject", colW[3], "From", "Date"),
	)
	b.WriteString(header + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).Render("  "+strings.Repeat("-", maxI(10, m.width-6))) + "\n")

	visH := maxI(4, m.height-12)
	start := 0
	if m.cursor >= visH {
		start = m.cursor - visH + 1
	}
	for i := start; i < len(m.highlights) && i < start+visH; i++ {
		h := m.highlights[i]
		label := h.Signal.Label()
		badge := hlSignalBadge(h.Signal, label)
		company := h.Company
		if company == "" {
			company = h.Domain
		}
		row := fmt.Sprintf("%s  %-*s  %-*s  %-*s  %s",
			badge,
			colW[1], hlTruncate(company, colW[1]),
			colW[2], hlTruncate(h.Subject, colW[2]),
			colW[3], hlTruncate(h.From, colW[3]),
			h.Date.Format("2006-01-02"),
		)
		if i == m.cursor {
			prefix := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render("▶ ")
			b.WriteString(prefix + lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Render(row) + "\n")
		} else {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(textPrimary).Render(row) + "\n")
		}
	}
	return b.String()
}

func (m HighlightsTabModel) cfgOk() bool {
	return m.cfg != nil && m.cfg.Email != "" && m.cfg.GmailAppPassword != ""
}

// hlSignalBadge renders a short colored badge for a signal.
func hlSignalBadge(s inbox.Signal, label string) string {
	var color string
	switch s {
	case inbox.SignalInterview:
		color = colorGreen
	case inbox.SignalOffer:
		color = colorGreen
	case inbox.SignalRejection:
		color = colorRed
	case inbox.SignalRecruiter:
		color = colorOrange
	case inbox.SignalAssessment:
		color = colorPurple
	default:
		color = colorGrey
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(hlTruncate(label, 11))
}

// hlTruncate shortens s to at most n runes.
func hlTruncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}
