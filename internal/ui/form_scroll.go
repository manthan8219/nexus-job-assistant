package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// fitFormHeight keeps Config inside the terminal body by showing a window
// around the focused field (tab/↑↓ naturally scroll the form).
func (m FormModel) fitFormHeight(content string) string {
	h := m.height
	if h <= 0 {
		return content
	}
	// Reserve one row for the scroll hint when truncated.
	lines := strings.Split(content, "\n")
	// Trim a single trailing empty line from strings.Builder.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= h {
		return content
	}

	focus := 0
	for i, line := range lines {
		if strings.Contains(line, "▶ ") {
			focus = i
			break
		}
	}

	viewH := h - 1
	if viewH < 5 {
		viewH = h
	}
	start := focus - viewH/3
	if start < 0 {
		start = 0
	}
	end := start + viewH
	if end > len(lines) {
		end = len(lines)
		start = end - viewH
		if start < 0 {
			start = 0
		}
	}

	out := strings.Join(lines[start:end], "\n")
	hint := mutedStyle.Render(fmt.Sprintf("  ↕ fields  ·  showing %d–%d of %d", start+1, end, len(lines)))
	return out + "\n" + hint
}

// renderResumeLibraryTeaser is a one-line summary for the unfocused Resume Path.
func (m FormModel) renderResumeLibraryTeaser() string {
	vers, err := resume.ListVersions()
	if err != nil || len(vers) == 0 {
		return ""
	}
	latest := vers[0].DisplayLine()
	if len(latest) > 64 {
		latest = latest[:63] + "…"
	}
	return "\n    " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render(
		fmt.Sprintf("JobPilot generated (%d)  ·  focus field to pick  ·  latest: %s", len(vers), latest),
	)
}
