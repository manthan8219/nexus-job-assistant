package ui

// Package ui — form_view.go
// Top-level rendering for the Config form: the View loop that walks every
// visible field, plus the small provider-status, masked-key, and autocomplete
// renderers. The per-field render switch lives in form_render.go.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the entire Config form: an onboarding banner while required
// fields are missing, then each visible field grouped under its section header.
func (m FormModel) View() string {
	var b strings.Builder

	// Onboarding banner — shown until profile is complete
	missing := m.MissingFields()
	if len(missing) > 0 {
		banner := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPurple)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorPurpleMuted)).
			Padding(0, 2).
			Render(
				lipgloss.NewStyle().Bold(true).Render("Complete your profile to start applying") + "\n" +
					mutedStyle.Render("Still needed: ") +
					lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(strings.Join(missing, "  ·  ")),
			)
		b.WriteString("\n  " + banner + "\n")
	}

	locked := m.resumeInvalid()

	currentSection := -1
	for i := 0; i < fieldCount; i++ {
		if !m.fieldVisible(i) {
			continue
		}
		for si, sec := range formSections {
			if sec.start == i && si != currentSection {
				currentSection = si
				b.WriteString(sectionStyle.Render(fmt.Sprintf("  %s", sec.title)) + "\n")
				// Before the key-input fields, show always-active providers.
				if sec.start == fLinkedInKey {
					b.WriteString(m.renderProviderStatusSection())
				}
				if sec.start == fAIAssist {
					b.WriteString(m.renderAISectionHint())
				}
			}
		}

		fieldLocked := locked && isLockedByResume(i)
		active := i == m.focused && !fieldLocked

		var prefix, label, widget string
		if fieldLocked {
			lockedStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorGreyMid)).
				Faint(true)
			prefix = "  "
			label = lockedStyle.Render(fmt.Sprintf("%-22s", fieldLabels[i]))
			widget = lockedStyle.Render("🔒 fix resume path to unlock")
		} else {
			lbl := labelInactiveStyle
			prefix = "  "
			if active {
				lbl = labelActiveStyle
				prefix = "▶ "
			}
			label = lbl.Render(fmt.Sprintf("%-22s", fieldLabels[i]))
			widget = m.renderField(i, active)
		}
		b.WriteString(fmt.Sprintf("%s%s  %s\n", prefix, label, widget))
	}

	b.WriteString("\n")
	if m.saved {
		b.WriteString(savedStyle.Render("  ✓ Auto-saved") + "\n")
	}
	if m.notifyBanner != "" {
		b.WriteString(savedStyle.Render("  "+m.notifyBanner) + "\n")
	}
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  ✗ %v", m.err)) + "\n")
	}
	return m.fitFormHeight(b.String())
}

// renderProviderKeyField renders a masked API key field with an active/inactive badge.
func (m FormModel) renderProviderKeyField(i int, active bool) string {
	val := m.inputs[i].Value()
	var badge string
	if val != "" {
		badge = "  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("● Active")
	} else {
		badge = "  " + mutedStyle.Render("○ Inactive — enter key to activate")
	}
	if active {
		clue := "  " + mutedStyle.Render("ctrl+x clears")
		return m.inputs[i].View() + badge + clue
	}
	if val == "" {
		return mutedStyle.Render("—") + badge
	}
	// Show masked value + badge when inactive
	return mutedStyle.Render(maskDots(len(m.inputs[i].Value()))) + badge
}

// renderProviderStatusSection renders the always-active providers (no user key needed)
// as a status list above the key input fields.
func (m FormModel) renderProviderStatusSection() string {
	var b strings.Builder
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	b.WriteString("\n")
	for _, p := range allProviders {
		if p.keyField != -1 {
			continue // shown as form fields below
		}
		b.WriteString(fmt.Sprintf("  %-22s  %s\n",
			mutedStyle.Render(p.name),
			activeStyle.Render("● Always active"),
		))
	}
	return b.String()
}

// renderAC renders the autocomplete dropdown below the resume path input.
func (m FormModel) renderAC() string {
	var b strings.Builder
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorPurple)).
		Bold(true)
	dirStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorPurpleMuted))

	for i, s := range m.acSuggestions {
		isDir := strings.HasSuffix(s, "/")
		label := s
		if isDir {
			label = dirStyle.Render(s)
		} else {
			label = primaryStyle.Render(s)
		}

		if i == m.acIdx {
			b.WriteString("\n    " + selectedStyle.Render("▶ ") + label)
		} else {
			b.WriteString("\n      " + label)
		}
	}
	b.WriteString("\n    " + mutedStyle.Render("↑↓ navigate · enter/tab select · esc dismiss"))
	return b.String()
}
