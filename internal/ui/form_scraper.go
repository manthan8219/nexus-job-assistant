package ui

// Package ui — form_scraper.go
// Career Scraper setup widget for the Config form.
// Handles the fScraperTargets field: venv install, backend catalog picker,
// background setup commands, and async status messages.

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

// ── Async messages ────────────────────────────────────────────────────────────

// scraperStatusMsg is the async result of scraper setup actions.
type scraperStatusMsg struct {
	installed []string
	running   bool
	err       error
}

// ── Commands ──────────────────────────────────────────────────────────────────

// runScraperSetupOption executes the selected scraper setup action.
func (m FormModel) runScraperSetupOption(opt scraper.SetupOption) tea.Cmd {
	switch opt.ID {
	case "install":
		return func() tea.Msg {
			err := scraper.Install(context.Background(), nil)
			if err != nil {
				return scraperStatusMsg{err: err}
			}
			// auto-start after install
			_ = scraper.Start("", "")
			return scraperStatusMsg{running: scraper.Running()}
		}
	case "start":
		return func() tea.Msg {
			_ = scraper.Start("", "")
			_ = scraper.WaitReady(15 * time.Second)
			return scraperStatusMsg{running: scraper.Running()}
		}
	case "retry":
		return func() tea.Msg {
			return scraperStatusMsg{running: scraper.Running()}
		}
	case "scan":
		return func() tea.Msg { return ScraperScanMsg{} }
	}
	return nil
}

// ── Render helpers ────────────────────────────────────────────────────────────

// renderBackendCatalog renders the installed/available backend catalog below
// the scraper field when it is active and the venv is ready.
func (m FormModel) renderBackendCatalog() string {
	installedSet := make(map[string]bool)
	for _, id := range m.scraperInstalled {
		installedSet[id] = true
	}
	var b strings.Builder
	for i, bk := range scraper.Catalog {
		isInstalled := installedSet[bk.ID]
		mark := "  "
		meta := bk.Notes
		if isInstalled {
			meta = "installed · " + meta
		} else {
			meta = "enter to install · " + meta
		}
		var line string
		if i == m.scraperBackendCursor {
			mark = "▶ "
			label := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(bk.Name)
			line = label + "  " + mutedStyle.Render(meta)
		} else if isInstalled {
			line = primaryStyle.Render(bk.Name) + "  " + mutedStyle.Render(meta)
		} else {
			line = mutedStyle.Render(bk.Name + "  " + meta)
		}
		b.WriteString("\n    " + mark + line)
	}
	b.WriteString("\n    " + mutedStyle.Render("↑↓ move · enter install · tab next"))
	return b.String()
}

// renderScraperSetupMenu renders either the venv-install menu (step 1) or the
// backend catalog picker (step 2) depending on whether the venv is ready.
func (m FormModel) renderScraperSetupMenu() string {
	var b strings.Builder

	venvReady := scraper.Installed()
	if !venvReady {
		// Step 1: venv not set up — show install/start/retry actions.
		b.WriteString(errorStyle.Render("Career Scraper not set up"))
		b.WriteString("\n    " + mutedStyle.Render("Needs Python 3.10+ on PATH"))
		if m.scraperStatus != "" {
			b.WriteString("\n    " + mutedStyle.Render(m.scraperStatus))
		}
		for i, opt := range scraper.SetupOptions() {
			mark := "  "
			var line string
			if i == m.scraperSetupCursor {
				mark = "▶ "
				line = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(opt.Label) +
					"  " + mutedStyle.Render(opt.Hint)
			} else {
				line = mutedStyle.Render(opt.Label + "  " + opt.Hint)
			}
			b.WriteString("\n    " + mark + line)
		}
		b.WriteString("\n    " + mutedStyle.Render("↑↓ move · enter run · tab next"))
		return b.String()
	}

	// Step 2: venv ready — show backend catalog picker.
	b.WriteString(primaryStyle.Render("Pick a scraper backend to install:"))
	if m.scraperStatus != "" {
		b.WriteString("\n    " + mutedStyle.Render(m.scraperStatus))
	}
	installedSet := make(map[string]bool)
	for _, id := range m.scraperInstalled {
		installedSet[id] = true
	}
	for i, bk := range scraper.Catalog {
		isInstalled := installedSet[bk.ID]
		mark := "  "
		meta := bk.Notes
		if isInstalled {
			meta = "installed · " + meta
		} else {
			meta = "enter to install · " + meta
		}
		var line string
		if i == m.scraperBackendCursor {
			mark = "▶ "
			label := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(bk.Name)
			line = label + "  " + mutedStyle.Render(meta)
		} else if isInstalled {
			line = primaryStyle.Render(bk.Name) + "  " + mutedStyle.Render(meta)
		} else {
			line = mutedStyle.Render(bk.Name + "  " + meta)
		}
		b.WriteString("\n    " + mark + line)
	}
	b.WriteString("\n    " + mutedStyle.Render("↑↓ move · enter install · tab next"))
	return b.String()
}
