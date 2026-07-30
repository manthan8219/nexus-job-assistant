package ui

// Package ui — form_widgets.go
// Custom (non-textinput) Config widgets and their key handlers: the inline tag
// pill field, work-type checkboxes, notification channel checkboxes, and the
// currency / salary preset pickers. Each key handler returns (model, cmd, ok)
// where ok=false lets navigation keys fall through to the shared field nav.

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
)

// renderTagField renders inline tag pills + text input (single line).
func (m FormModel) renderTagField(tags []string, input textinput.Model, active bool) string {
	if !active {
		if len(tags) == 0 {
			return mutedStyle.Render("—")
		}
		styled := make([]string, len(tags))
		for i, t := range tags {
			styled[i] = primaryStyle.Render(t)
		}
		return strings.Join(styled, mutedStyle.Render("  ·  "))
	}

	// Active: inline [tag ×] pills using text characters (no borders — borders break line height)
	var parts []string
	for _, tag := range tags {
		pill := tagRemoveStyle.Render("[") +
			tagStyle.Render(tag) +
			tagRemoveStyle.Render(" ×]")
		parts = append(parts, pill)
	}
	parts = append(parts, input.View())
	hint := mutedStyle.Render("  ↑↓ pick · enter add · backspace remove")
	return strings.Join(parts, " ") + hint
}

// renderWorkType renders the inline checkbox row.
func (m FormModel) renderWorkType(active bool) string {
	if !active {
		val := m.workTypeValue()
		if val == "" {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(val)
	}
	parts := make([]string, 3)
	for i, opt := range wtOptions {
		box := "[ ]"
		boxColor := colorGrey
		if m.wtSelected[i] {
			box = "[✓]"
			boxColor = colorGreen
		}
		boxStr := lipgloss.NewStyle().Foreground(lipgloss.Color(boxColor)).Render(box)
		if i == m.wtCursor {
			parts[i] = boxStr + " " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(opt)
		} else {
			parts[i] = boxStr + " " + primaryStyle.Render(opt)
		}
	}
	return strings.Join(parts, "  ") + mutedStyle.Render("   h/l move · enter/space toggle · tab next")
}

// handleWorkTypeKey handles the Work Type checkbox row. Returns ok=false for
// navigation keys so they fall through to field navigation.
func (m FormModel) handleWorkTypeKey(key string) (FormModel, tea.Cmd, bool) {
	switch key {
	case "left", "h":
		m.wtCursor = (m.wtCursor - 1 + 3) % 3
		return m, nil, true
	case "right", "l":
		m.wtCursor = (m.wtCursor + 1) % 3
		return m, nil, true
	case " ", "enter":
		m.wtSelected[m.wtCursor] = !m.wtSelected[m.wtCursor]
		return m, m.saveCmd(), true
	case "tab", "down", "shift+tab", "up":
		return m, nil, false // fall through to nav
	default:
		return m, nil, true
	}
}

// renderNotifyChannels renders the notification channel checkbox row with
// per-channel credential warnings when a channel is selected but unconfigured.
// Channel list comes from notifier.Available() so new integrations appear automatically.
func (m FormModel) renderNotifyChannels(active bool) string {
	avail := notifier.Available()
	ncfg := m.notifyConfigSnapshot()

	if !active {
		var on []string
		for i, ch := range avail {
			if i < len(m.ncSelected) && m.ncSelected[i] {
				on = append(on, ch.DisplayName)
			}
		}
		if len(on) == 0 {
			return mutedStyle.Render("— none selected")
		}
		return primaryStyle.Render(strings.Join(on, ", "))
	}

	parts := make([]string, len(avail))
	for i, ch := range avail {
		selected := i < len(m.ncSelected) && m.ncSelected[i]
		box := "[ ]"
		boxColor := colorGrey
		if selected {
			box = "[✓]"
			boxColor = colorGreen
		}
		boxStr := lipgloss.NewStyle().Foreground(lipgloss.Color(boxColor)).Render(box)
		label := ch.DisplayName
		if i == m.ncCursor {
			label = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(label)
		} else {
			label = primaryStyle.Render(label)
		}
		if selected && !ch.Configured(ncfg) {
			warn := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render(" ⚠ " + ch.WarnMsg)
			parts[i] = boxStr + " " + label + warn
		} else {
			parts[i] = boxStr + " " + label
		}
	}
	return strings.Join(parts, "   ") + mutedStyle.Render("   ←→/h/l move · enter/space toggle · ctrl+t test · tab next")
}

// handleNotifyChannelsKey handles the notification channel checkbox row.
// Returns ok=false for navigation keys so they fall through to field navigation.
func (m FormModel) handleNotifyChannelsKey(key string) (FormModel, tea.Cmd, bool) {
	n := len(m.ncSelected)
	if n == 0 {
		switch key {
		case "tab", "down", "shift+tab", "up", "enter":
			return m, nil, false // fall through to nav
		default:
			return m, nil, true
		}
	}
	switch key {
	case "left", "h":
		m.ncCursor = (m.ncCursor - 1 + n) % n
		return m, nil, true
	case "right", "l":
		m.ncCursor = (m.ncCursor + 1) % n
		return m, nil, true
	case " ", "enter":
		m.ncSelected[m.ncCursor] = !m.ncSelected[m.ncCursor]
		return m, m.saveCmd(), true
	case "tab", "down", "shift+tab", "up":
		return m, nil, false // fall through to nav
	default:
		return m, nil, true
	}
}

// notifyConfigSnapshot builds a NotifyConfig from current form values for
// credential checks against the notifier registry.
func (m FormModel) notifyConfigSnapshot() *notifier.NotifyConfig {
	return &notifier.NotifyConfig{
		DiscordWebhookURL: m.inputs[fDiscordWebhook].Value(),
		TelegramBotToken:  m.inputs[fTelegramBotToken].Value(),
		TelegramChatID:    m.inputs[fTelegramChatID].Value(),
	}
}

// renderCurrency renders all currencies inline with the selected one highlighted.
func (m FormModel) renderCurrency(active bool) string {
	cur := currencies[m.currencyIdx]
	if !active {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(cur.Code)
	}
	var parts []string
	for i, c := range currencies {
		if i == m.currencyIdx {
			parts = append(parts,
				lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorPurple)).
					Bold(true).
					Render("["+c.Code+"]"),
			)
		} else {
			parts = append(parts, mutedStyle.Render(c.Code))
		}
	}
	return strings.Join(parts, "  ") + mutedStyle.Render("   h/l to pick")
}

// handleCurrencyKey handles the currency cycle selector. Returns ok=false for
// navigation keys so they fall through to field navigation.
func (m FormModel) handleCurrencyKey(key string) (FormModel, tea.Cmd, bool) {
	switch key {
	case "left", "h":
		m.currencyIdx = (m.currencyIdx - 1 + len(currencies)) % len(currencies)
		m.salaryPreset = 0
		m.salaryCustom = ""
		return m, m.saveCmd(), true
	case "right", "l":
		m.currencyIdx = (m.currencyIdx + 1) % len(currencies)
		m.salaryPreset = 0
		m.salaryCustom = ""
		return m, m.saveCmd(), true
	case "tab", "down", "shift+tab", "up", "enter":
		return m, nil, false // fall through to nav
	default:
		return m, nil, true
	}
}

// renderSalary renders the preset salary picker or custom input.
func (m FormModel) renderSalary(active bool) string {
	cur := currencies[m.currencyIdx]

	if !active {
		if m.salaryCustom != "" {
			return primaryStyle.Render(cur.Symbol + m.salaryCustom)
		}
		if m.salaryPreset < 0 || m.salaryPreset >= len(cur.Presets) {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(formatSalary(cur, cur.Presets[m.salaryPreset]))
	}

	// Custom typing mode
	if m.salaryCustom != "" || m.salaryPreset < 0 {
		cursor := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render("▋")
		val := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(cur.Symbol + m.salaryCustom)
		return val + cursor + mutedStyle.Render("   backspace · esc for presets")
	}

	// Preset mode
	var parts []string
	for i, p := range cur.Presets {
		label := formatSalary(cur, p)
		if i == m.salaryPreset {
			parts = append(parts,
				lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorPurple)).
					Bold(true).
					Render("["+label+"]"),
			)
		} else {
			parts = append(parts,
				lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorGrey)).
					Render(label),
			)
		}
	}
	return strings.Join(parts, "  ") + mutedStyle.Render("   h/l pick · type to enter custom")
}

// handleSalaryKey handles the salary preset picker and custom-typing mode.
// Returns ok=false for navigation keys so they fall through to field navigation.
func (m FormModel) handleSalaryKey(key string) (FormModel, tea.Cmd, bool) {
	presets := currencies[m.currencyIdx].Presets
	switch key {
	case "left", "h":
		if m.salaryCustom == "" {
			if m.salaryPreset > 0 {
				m.salaryPreset--
			}
			return m, m.saveCmd(), true
		}
		return m, nil, true
	case "right", "l":
		if m.salaryCustom == "" {
			if m.salaryPreset < len(presets)-1 {
				m.salaryPreset++
			}
			return m, m.saveCmd(), true
		}
		return m, nil, true
	case "backspace":
		if m.salaryCustom != "" {
			m.salaryCustom = m.salaryCustom[:len(m.salaryCustom)-1]
		}
		return m, m.saveCmd(), true
	case "esc":
		// Cancel custom input, revert to first preset
		m.salaryCustom = ""
		if m.salaryPreset < 0 {
			m.salaryPreset = 0
		}
		return m, m.saveCmd(), true
	case "tab", "down", "shift+tab", "up", "enter":
		return m, nil, false // fall through to nav
	default:
		// Digit → accumulate into custom value
		if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
			m.salaryCustom += key
			m.salaryPreset = -1
			return m, m.saveCmd(), true
		}
		return m, nil, true
	}
}
