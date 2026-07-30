package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/usage"
)

type RefreshUsageMsg struct{ Snap usage.Snapshot }

type usageTickMsg struct{}

// logsManualRefreshMsg bubbles up from LogsModel when the user presses "r".
type logsManualRefreshMsg struct{}

type LogsModel struct {
	width     int
	height    int
	lines     []string // all lines (raw, styled)
	rawLines  []string // all lines (unstyled, for filtering)
	viewport  viewport.Model
	ready     bool
	atBottom  bool
	usage     usage.Snapshot
	usageOK   bool
	filtering bool   // true = filter input active
	filter    string // current filter string
}

func NewLogsModel() LogsModel {
	return LogsModel{atBottom: true}
}

func (m LogsModel) Init() tea.Cmd { return nil }

func (m LogsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m = m.resizeViewport()
		m.rebuildContent()
		return m, nil

	case RefreshUsageMsg:
		m.usage = msg.Snap
		m.usageOK = true
		m = m.resizeViewport()
		m.rebuildContent()
		return m, nil

	case AppendLogMsg:
		ts := time.Now().Format("15:04:05")
		line := mutedStyle.Render(ts) + "  " + colorizeLog(msg.Line)
		m.lines = append(m.lines, line)
		m.rawLines = append(m.rawLines, msg.Line)
		if len(m.lines) > 500 {
			m.lines = m.lines[len(m.lines)-500:]
			m.rawLines = m.rawLines[len(m.rawLines)-500:]
		}
		if m.ready {
			m.rebuildContent()
			if m.atBottom {
				m.viewport.GotoBottom()
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "c":
			if m.filtering {
				m.filter = ""
				m.filtering = false
				m.rebuildContent()
				return m, nil
			}
			m.lines = nil
			m.rawLines = nil
			m.filter = ""
			if m.ready {
				m.rebuildContent()
			}
			return m, nil
		case "r":
			return m, func() tea.Msg { return logsManualRefreshMsg{} }
		case "G", "end":
			if m.ready {
				m.viewport.GotoBottom()
				m.atBottom = true
			}
			return m, nil
		case "/":
			m.filtering = true
			return m, nil
		case "esc":
			if m.filtering {
				m.filtering = false
				m.filter = ""
				m.rebuildContent()
				return m, nil
			}
		case "backspace":
			if m.filtering && len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.rebuildContent()
				return m, nil
			}
		}
		if m.filtering {
			// append typed chars to filter
			if k := msg.String(); len(k) == 1 {
				m.filter += k
				m.rebuildContent()
				if m.ready {
					m.viewport.GotoBottom()
				}
			}
			return m, nil
		}
		if m.ready {
			wasAtBottom := m.viewport.AtBottom()
			m.viewport, cmd = m.viewport.Update(msg)
			m.atBottom = wasAtBottom && m.viewport.AtBottom()
		}
		return m, cmd
	}
	return m, nil
}

func (m LogsModel) resizeViewport() LogsModel {
	usageLines := 7
	vh := m.height - 8 - usageLines
	if vh < 5 {
		vh = 5
	}
	vw := m.width - 4
	if vw < 40 {
		vw = 40
	}
	if !m.ready {
		m.viewport = viewport.New(vw, vh)
		m.ready = true
	} else {
		m.viewport.Width = vw
		m.viewport.Height = vh
	}
	return m
}

func (m *LogsModel) rebuildContent() {
	if len(m.lines) == 0 {
		m.viewport.SetContent(mutedStyle.Render("  No logs yet. Engine runs and Jobs backfill (u/U) both stream here."))
		return
	}
	if m.filter == "" {
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		return
	}
	var filtered []string
	f := strings.ToLower(m.filter)
	for i, raw := range m.rawLines {
		if strings.Contains(strings.ToLower(raw), f) {
			filtered = append(filtered, m.lines[i])
		}
	}
	if len(filtered) == 0 {
		m.viewport.SetContent(mutedStyle.Render("  No lines match \"" + m.filter + "\""))
		return
	}
	m.viewport.SetContent(strings.Join(filtered, "\n"))
}

func (m LogsModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}

	var b strings.Builder
	b.WriteString("\n  " + labelStyle.Render("LOGS") +
		"  " + mutedStyle.Render(fmt.Sprintf("(%d lines)", len(m.lines))) + "\n\n")
	b.WriteString(m.renderUsagePanel(w))
	b.WriteString("\n")

	var filterBar string
	if m.filtering {
		cursor := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Render("▌")
		filterBar = "  " + mutedStyle.Render("filter: ") + primaryStyle.Render(m.filter) + cursor + "\n"
	} else if m.filter != "" {
		filterBar = "  " + mutedStyle.Render("filter: ") + primaryStyle.Render(m.filter) + "  " + mutedStyle.Render("esc clear  •  / new filter") + "\n"
	}
	help := mutedStyle.Render("  ↑↓ scroll  •  G end  •  / filter  •  c clear")

	if !m.ready {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreyMid)).Render("  Initializing..."))
		return b.String()
	}

	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(w - 6).
		Render(m.viewport.View())

	scrollPct := ""
	if len(m.lines) > 0 {
		pct := int(m.viewport.ScrollPercent() * 100)
		scrollPct = mutedStyle.Render(fmt.Sprintf(" %d%%", pct))
	}

	b.WriteString("  " + border + scrollPct + "\n\n")
	if filterBar != "" {
		b.WriteString(filterBar)
	}
	b.WriteString(help)
	return b.String()
}

func (m LogsModel) renderUsagePanel(w int) string {
	title := labelStyle.Render("USAGE")
	if !m.usageOK {
		return "  " + title + "\n  " + mutedStyle.Render("Collecting local storage + process stats…") + "\n"
	}
	s := m.usage
	var b strings.Builder
	b.WriteString("  " + title)
	if !s.CollectedAt.IsZero() {
		b.WriteString("  " + mutedStyle.Render(s.CollectedAt.Format("15:04:05")))
	}
	b.WriteString("\n")

	if s.Err != "" {
		b.WriteString("  " + mutedStyle.Render("error: "+s.Err) + "\n")
		return b.String()
	}

	dir := s.DataDir
	if len(dir) > 42 {
		dir = "…" + dir[len(dir)-41:]
	}
	b.WriteString("  " + mutedStyle.Render("Storage") + "  " +
		primaryStyle.Render(usage.Bytes(s.TotalBytes)) + " total" +
		mutedStyle.Render("  ·  "+dir) + "\n")
	b.WriteString("  " + mutedStyle.Render("  jobs DB") + " " + primaryStyle.Render(usage.Bytes(s.DBBytes)) +
		mutedStyle.Render("  ·  resumes ") + primaryStyle.Render(usage.Bytes(s.ResumesBytes)) +
		mutedStyle.Render("  ·  config/meta ") + primaryStyle.Render(usage.Bytes(s.MetaBytes)))
	if s.OtherBytes > 0 {
		b.WriteString(mutedStyle.Render("  ·  other ") + primaryStyle.Render(usage.Bytes(s.OtherBytes)))
	}
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  ·  %d jobs", s.JobCount)) + "\n")

	b.WriteString("  " + mutedStyle.Render("Process") + "  heap " +
		primaryStyle.Render(usage.Bytes(int64(s.HeapAlloc))) +
		mutedStyle.Render("  ·  reserved ") + primaryStyle.Render(usage.Bytes(int64(s.SysBytes))) +
		mutedStyle.Render(fmt.Sprintf("  ·  %d goroutines", s.Goroutines)) + "\n")

	mode := s.AIMode
	if mode == "" {
		mode = "off"
	}
	b.WriteString("  " + mutedStyle.Render("Fit AI") + "   " +
		primaryStyle.Render(mode) + "  " +
		mutedStyle.Render(usage.FitCostHint(mode)) + "\n")
	b.WriteString("  " + mutedStyle.Render("Scoring") + " " +
		mutedStyle.Render("title + location + company + full JD + resume → one sequential LLM call per job") + "\n")
	_ = w
	return b.String()
}

// colorizeLog adds color to log lines based on their content.
func colorizeLog(line string) string {
	switch {
	case strings.HasPrefix(line, "  ✓"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render(line)
	case strings.HasPrefix(line, "  ✗"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(line)
	case strings.HasPrefix(line, "  ~"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreyMid)).Render(line)
	case strings.HasPrefix(line, "  →"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurpleMuted)).Render(line)
	case strings.HasPrefix(line, "[greenhouse]"), strings.HasPrefix(line, "  [greenhouse]"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Render(line)
	default:
		return primaryStyle.Render(line)
	}
}
