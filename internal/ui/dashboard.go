package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Messages sent from dashboard to app
type StartEngineMsg struct {
	DryRun    bool
	AutoApply bool
}
type StopEngineMsg struct{}

// DashRecent is one recent application line for Mission Control.
type DashRecent struct {
	Label  string // "Role @ Company"
	Status string // applied | skipped | failed
}

type DashboardModel struct {
	width  int
	height int
	status string // "idle" | "running" | "done" | "error"
	errMsg string
	applied int
	skipped int
	failed  int
	lastJob string
	dryRun  bool
	providers []string
	progress  map[string]string
	autoApply bool
	hasConsent bool

	// Mission Control snapshot (filled by AppModel)
	resumePath   string
	resumeReady  bool
	hasTitles    bool
	aiOn         bool
	maxPerDay    int
	appliedToday int
	recent       []DashRecent
	liveFeed     []DashRecent // realtime finds / applies this run
	foundCount   int
}

func NewDashboardModel() DashboardModel {
	return DashboardModel{status: "idle", maxPerDay: 25}
}

func (m DashboardModel) Init() tea.Cmd { return nil }

func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ":
			if m.status == "running" {
				return m, func() tea.Msg { return StopEngineMsg{} }
			}
			auto := m.autoApply && m.hasConsent
			if m.autoApply && !m.hasConsent {
				m.errMsg = "Auto Apply needs Apply Consent in Config"
				m.autoApply = false
				auto = false
			}
			return m, func() tea.Msg { return StartEngineMsg{DryRun: m.dryRun, AutoApply: auto} }
		case "d":
			if m.status != "running" {
				m.dryRun = !m.dryRun
				m.errMsg = ""
			}
		case "a":
			if m.status != "running" {
				if !m.hasConsent {
					m.errMsg = "Give Apply Consent in Config → Apply Safety first"
					m.autoApply = false
					return m, nil
				}
				m.errMsg = ""
				m.autoApply = !m.autoApply
			}
		}
	}
	return m, nil
}

func (m DashboardModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + labelStyle.Render("MISSION CONTROL") + "\n")
	b.WriteString("  " + mutedStyle.Render("Your job-hunt command center — ready check, then one action") + "\n")

	// ── TODAY ────────────────────────────────────────────────────────────
	b.WriteString("\n  " + labelStyle.Render("TODAY") + "\n")
	cap := m.maxPerDay
	if cap <= 0 {
		cap = 25
	}
	todayLine := fmt.Sprintf("%s applied today  ·  daily cap %d  ·  lifetime %s / %s / %s",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true).Render(fmt.Sprintf("%d", m.appliedToday)),
		cap,
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render(fmt.Sprintf("%d applied", m.applied)),
		mutedStyle.Render(fmt.Sprintf("%d skipped", m.skipped)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(fmt.Sprintf("%d failed", m.failed)),
	)
	b.WriteString("  " + todayLine + "\n")

	resumeName := "(none)"
	if m.resumePath != "" {
		resumeName = filepath.Base(m.resumePath)
	}
	b.WriteString("  " + mutedStyle.Render("Active resume:") + " " + primaryStyle.Render(resumeName) + "\n")

	// ── READY ────────────────────────────────────────────────────────────
	b.WriteString("\n  " + labelStyle.Render("READY") + "\n")
	checks := []struct {
		ok   bool
		good string
		bad  string
	}{
		{m.resumeReady, "Resume ready", "Resume missing or invalid — set in Config"},
		{m.hasTitles, "Target titles set", "Config → describe the job you want (AI fills titles)"},
		{m.hasConsent, "Apply consent given", "Give Apply Consent in Config → Apply Safety"},
		{m.aiOn, "AI Assist on", "AI Assist off (optional — better answers when on)"},
	}
	allRequired := m.resumeReady && m.hasTitles && m.hasConsent
	for _, c := range checks {
		mark := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("✓")
		text := primaryStyle.Render(c.good)
		if !c.ok {
			mark = lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render("○")
			text = mutedStyle.Render(c.bad)
		}
		b.WriteString("  " + mark + " " + text + "\n")
	}

	// ── MODE ─────────────────────────────────────────────────────────────
	b.WriteString("\n  " + labelStyle.Render("MODE") + "\n")
	modeName, modeHint := m.modeCopy()
	b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true).Render(modeName) + "\n")
	b.WriteString("  " + mutedStyle.Render(modeHint) + "\n")

	dryLabel, dryColor := "OFF", colorRed
	if m.dryRun {
		dryLabel, dryColor = "ON", colorGreen
	}
	autoLabel, autoColor := "PAUSED", colorRed
	if m.autoApply && m.hasConsent {
		autoLabel, autoColor = "ARMED", colorGreen
	}
	consentLabel, consentColor := "REQUIRED", colorOrange
	if m.hasConsent {
		consentLabel, consentColor = "OK", colorGreen
	}
	b.WriteString("  " +
		mutedStyle.Render("Dry run") + " " + lipgloss.NewStyle().Foreground(lipgloss.Color(dryColor)).Bold(true).Render(dryLabel) + mutedStyle.Render(" [d]") +
		"  │  " +
		mutedStyle.Render("Auto apply") + " " + lipgloss.NewStyle().Foreground(lipgloss.Color(autoColor)).Bold(true).Render(autoLabel) + mutedStyle.Render(" [a]") +
		"  │  " +
		mutedStyle.Render("Consent") + " " + lipgloss.NewStyle().Foreground(lipgloss.Color(consentColor)).Bold(true).Render(consentLabel) +
		"\n")

	// ── NEXT ACTION ──────────────────────────────────────────────────────
	b.WriteString("\n  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render("→ "+m.nextAction(allRequired)) + "\n")

	// Status + start/stop
	dot, dotColor := statusDot(m.status)
	actionHint := "[enter] start"
	actionColor := colorGreen
	if m.status == "running" {
		actionHint = "[enter] stop"
		actionColor = colorRed
	}
	b.WriteString("\n  " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(dotColor)).Bold(true).Render(dot+" "+statusLabel(m.status)) +
		"    " +
		lipgloss.NewStyle().Foreground(lipgloss.Color(actionColor)).Bold(true).Render(actionHint) +
		"\n")
	if m.errMsg != "" {
		b.WriteString("  " + errorStyle.Render(m.errMsg) + "\n")
	}

	// ── PROVIDERS ────────────────────────────────────────────────────────
	b.WriteString("\n  " + labelStyle.Render("PROVIDERS") + "  ")
	if len(m.providers) == 0 {
		b.WriteString(mutedStyle.Render("none configured") + "\n")
	} else if m.status == "running" || (m.status == "done" && len(m.progress) > 0) {
		b.WriteString("\n")
		const cols = 4
		for i, p := range m.providers {
			icon, color := providerIcon(m.progress[p])
			cell := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(icon) + " " + primaryStyle.Render(p)
			if i == 0 {
				b.WriteString("    " + cell)
			} else if i%cols == 0 {
				b.WriteString("\n    " + cell)
			} else {
				b.WriteString("   " + cell)
			}
		}
		b.WriteString("\n")
	} else {
		var provs []string
		for _, p := range m.providers {
			provs = append(provs, lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("✓")+" "+primaryStyle.Render(p))
		}
		b.WriteString(strings.Join(provs, "  ") + "\n")
	}

	// ── LIVE FEED ────────────────────────────────────────────────────────
	b.WriteString("\n  " + labelStyle.Render("LIVE") + "\n")
	if m.foundCount > 0 || len(m.liveFeed) > 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render(
			fmt.Sprintf("%d jobs discovered this run", m.foundCount),
		) + "\n")
	}
	if len(m.liveFeed) == 0 {
		if m.status == "running" {
			b.WriteString("  " + mutedStyle.Render("Searching… jobs will appear here as providers return") + "\n")
		} else {
			b.WriteString("  " + mutedStyle.Render("Start a run — finds stream here in real time") + "\n")
		}
	} else {
		limit := len(m.liveFeed)
		if limit > 10 {
			limit = 10
		}
		for i := 0; i < limit; i++ {
			r := m.liveFeed[i]
			st := liveStatusStyle(r.Status)
			b.WriteString("  " + st + "  " + primaryStyle.Render(r.Label) + "\n")
		}
		if len(m.liveFeed) > limit {
			b.WriteString("  " + mutedStyle.Render(fmt.Sprintf("… +%d more", len(m.liveFeed)-limit)) + "\n")
		}
	}

	// ── RECENT ───────────────────────────────────────────────────────────
	b.WriteString("\n  " + labelStyle.Render("RECENT") + "\n")
	if len(m.recent) == 0 {
		b.WriteString("  " + mutedStyle.Render("No applications yet — start a dry run to populate Jobs") + "\n")
	} else {
		for _, r := range m.recent {
			st := mutedStyle.Render(r.Status)
			switch r.Status {
			case "applied":
				st = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("applied")
			case "failed":
				st = lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render("failed")
			case "skipped":
				st = mutedStyle.Render("skipped")
			}
			b.WriteString("  " + st + "  " + primaryStyle.Render(r.Label) + "\n")
		}
	}
	if m.lastJob != "" && m.status == "running" {
		b.WriteString("  " + mutedStyle.Render("Live:") + " " + primaryStyle.Render(m.lastJob) + "\n")
	}

	return b.String()
}

func (m DashboardModel) modeCopy() (name, hint string) {
	switch {
	case m.status == "running":
		return "Running", "Engine is searching / applying — press enter to stop"
	case m.dryRun:
		return "Dry run", "Searches boards and logs matches — does not submit applications"
	case m.autoApply && m.hasConsent:
		return "Auto apply (armed)", "Will submit real applications within your daily/run caps"
	default:
		return "Queue only", "Finds jobs and records “apply manually” links — safe default"
	}
}

func (m DashboardModel) nextAction(allRequired bool) string {
	if m.status == "running" {
		return "Running… watch Providers below, or press enter to stop"
	}
	if !m.resumeReady {
		return "Next: set a valid Resume Path in Config (or pick a Nexus PDF)"
	}
	if !m.hasTitles {
		return "Next: in Config, describe the job you want — AI fills titles"
	}
	if !m.hasConsent {
		return "Next: open Config → Apply Safety → set Apply Consent to Yes"
	}
	if m.dryRun {
		return "Next: press enter to dry-run (safe). Turn dry run off [d] when ready to queue/apply"
	}
	if m.autoApply && m.hasConsent {
		return "Next: press enter to run with Auto Apply armed — real submissions"
	}
	return "Next: press enter to search & queue (manual links). Press [a] only when you want Auto Apply"
}

func statusDot(status string) (dot, color string) {
	switch status {
	case "running":
		return "●", colorGreen
	case "done":
		return "●", colorPurple
	case "error":
		return "●", colorRed
	default:
		return "○", colorGrey
	}
}

func statusLabel(status string) string {
	switch status {
	case "running":
		return "Running"
	case "done":
		return "Done"
	case "error":
		return "Error"
	default:
		return "Idle"
	}
}

func providerIcon(status string) (string, string) {
	switch {
	case status == "searching":
		return "●", colorGreen
	case strings.HasPrefix(status, "done:"):
		return "✓", colorPurple
	case status == "done":
		return "✓", colorGrey
	case status == "error":
		return "✗", colorRed
	default:
		return "○", colorGrey
	}
}

func liveStatusStyle(status string) string {
	switch status {
	case "found":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render("found")
	case "applied":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("applied")
	case "failed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render("failed")
	case "queued":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Render("queued")
	case "dry-run":
		return mutedStyle.Render("dry-run")
	case "skipped":
		return mutedStyle.Render("skipped")
	default:
		return mutedStyle.Render(status)
	}
}

func (m *DashboardModel) pushLive(item DashRecent) {
	m.liveFeed = append([]DashRecent{item}, m.liveFeed...)
	if len(m.liveFeed) > 40 {
		m.liveFeed = m.liveFeed[:40]
	}
}
