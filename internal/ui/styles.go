package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shared color palette — chosen to work on both light and dark terminals.
const (
	colorPurple      = "#6D28D9"
	colorPurpleMuted = "#7C3AED"
	colorGrey        = "#6B7280"
	colorGreyMid     = "#9CA3AF"
	colorGreen       = "#059669"
	colorRed         = "#DC2626"
	colorOrange      = "#D97706"
)

// Adaptive text colors — flip based on terminal background.
var (
	textPrimary   = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#F9FAFB"}
	textSecondary = lipgloss.AdaptiveColor{Light: "#4B5563", Dark: "#9CA3AF"}
	textMuted     = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#6B7280"}
	borderColor   = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#374151"}
)

// Tab bar + chrome
var (
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPurple)).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorGrey)).
				Padding(0, 2)

	appTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPurple)).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGrey))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPurpleMuted)).
			Bold(true)

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGrey))

	primaryStyle = lipgloss.NewStyle().
			Foreground(textPrimary)
)

// placeholderView renders a centered stub screen for tabs not yet implemented.
func placeholderView(title, subtitle string, width, height int) string {
	content := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorPurpleMuted)).
		Bold(true).
		Render(title) +
		"\n\n" +
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGreyMid)).
			Render(subtitle)

	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 20
	}

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// divider returns a horizontal rule of given width.
func divider(width int) string {
	if width <= 0 {
		width = 80
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorGrey)).
		Render(strings.Repeat("─", width))
}
