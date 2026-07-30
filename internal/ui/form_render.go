package ui

// Package ui — form_render.go
// renderField is the per-field render switch dispatched from View (form_view.go).
// Each case renders one Config field's widget; custom widgets delegate to their
// own render helpers (form_widgets.go, form_llm.go, form_job_titles.go, …).

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

// renderField renders the widget for a single field index.
func (m FormModel) renderField(i int, active bool) string {
	switch i {

	case fLinkedInID:
		id := m.inputs[fLinkedInID].Value()
		if active {
			preview := ""
			if id != "" {
				preview = "  " + mutedStyle.Render("→ linkedin.com/in/"+id)
			}
			return m.inputs[i].View() + preview
		}
		if id == "" {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(id) + "  " + mutedStyle.Render("→ linkedin.com/in/"+id)

	case fResumePath:
		path := m.inputs[fResumePath].Value()
		suffix := m.resumeStatusSuffix()
		if active {
			line := m.inputs[i].View() + suffix
			if len(m.acSuggestions) > 0 {
				line += m.renderAC()
			}
			line += m.renderResumeLibrary()
			return line
		}
		line := ""
		if path == "" {
			line = mutedStyle.Render("—")
		} else {
			line = primaryStyle.Render(path) + suffix
		}
		// Compact teaser when not focused — full picker only while editing.
		return line + m.renderResumeLibraryTeaser()

	case fJobTitles:
		return m.renderJobTitlesField(active)

	case fLocations:
		line := m.renderTagField(m.locationTags, m.inputs[fLocations], active)
		if active && len(m.acSuggestions) > 0 {
			line += m.renderAC()
		}
		return line

	case fWorkType:
		return m.renderWorkType(active)

	case fAIAssist:
		return m.renderAIAssist(active)

	case fAIProvider:
		return m.renderAIProvider(active)

	case fCurrency:
		return m.renderCurrency(active)

	case fMinSalary:
		return m.renderSalary(active)

	case fLinkedInKey, fIndeedKey, fAnthropicKey, fOpenAIKey:
		return m.renderProviderKeyField(i, active)

	case fDiscordWebhook:
		webhook := m.inputs[fDiscordWebhook].Value()
		if active {
			help := mutedStyle.Render("Discord: Server Settings → Integrations → Webhooks → New Webhook → Copy URL  •  ctrl+x clears  •  ctrl+t test")
			return m.inputs[i].View() + "\n    " + help
		}
		if webhook == "" {
			return mutedStyle.Render("—")
		}
		return mutedStyle.Render(maskDots(len(webhook)))

	case fTelegramBotToken:
		if active {
			help := mutedStyle.Render("Telegram: message @BotFather → /newbot → copy the token  •  ctrl+x clears  •  ctrl+t test")
			return m.inputs[i].View() + "\n    " + help
		}
		if m.inputs[fTelegramBotToken].Value() == "" {
			return mutedStyle.Render("—")
		}
		return mutedStyle.Render(maskDots(len(m.inputs[fTelegramBotToken].Value())))

	case fTelegramChatID:
		if active {
			help := mutedStyle.Render("Telegram: message @userinfobot or add bot to group, then copy the chat ID  •  ctrl+t test")
			return m.inputs[i].View() + "\n    " + help
		}
		val := m.inputs[fTelegramChatID].Value()
		if val == "" {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(val)

	case fNotifyChannels:
		return m.renderNotifyChannels(active)

	case fApplyConsent:
		return m.renderApplyConsent(active)

	case fWorkAuth:
		return m.renderWorkAuth(active)

	case fCoverLetterMode:
		return m.renderCoverLetterMode(active)

	case fLocalLLMURL:
		if active {
			help := mutedStyle.Render("Ollama default http://localhost:11434  ·  install from ollama.com if needed")
			return m.inputs[i].View() + "\n    " + help
		}
		val := m.inputs[i].Value()
		if val == "" {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(val)

	case fLocalLLMModel:
		return m.renderLocalLLMPicker(active)

	case fGmailPassword, fHunterKey, fApolloKey:
		if active {
			clue := "  " + mutedStyle.Render("ctrl+x clears")
			return m.inputs[i].View() + clue
		}
		val := m.inputs[i].Value()
		if val == "" {
			return mutedStyle.Render("—")
		}
		return mutedStyle.Render(maskDots(len(val)))

	case fScraperTargets:
		if m.scraperOffline || !scraper.Running() {
			return m.renderScraperSetupMenu()
		}
		// Show installed backends; if active also show full catalog to install more
		installedSet := make(map[string]bool)
		for _, id := range m.scraperInstalled {
			installedSet[id] = true
		}
		var parts []string
		for _, b := range scraper.Catalog {
			if installedSet[b.ID] {
				parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Render("● "+b.Name))
			}
		}
		if len(parts) == 0 {
			return m.renderScraperSetupMenu()
		}
		out := strings.Join(parts, "  ")
		if m.scraperStatus != "" {
			out += "  " + mutedStyle.Render(m.scraperStatus)
		}
		if active {
			out += "\n" + m.renderBackendCatalog()
		}
		return out

	default:
		if active {
			return m.inputs[i].View()
		}
		val := m.inputs[i].Value()
		if val == "" {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(val)
	}
}
