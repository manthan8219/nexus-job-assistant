package ui

// Package ui — form_keys.go
// Keyboard dispatch for the Config form. handleKey routes a key to the focused
// field's handler (form_job_titles.go, form_llm.go, form_scraper.go,
// form_widgets.go, form_resume.go, form_locations.go, form_apply_safety.go).
// Each handler returns (model, cmd, ok); when ok is false the key falls through
// to the shared navigation tail, handleNavAndInput — exactly preserving the
// original single-function fall-through semantics. Visibility/lock helpers live
// here too.

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/localllm"
)

// handleKey dispatches a key press to the focused field's handler. Each
// per-field handler returns (model, cmd, ok); when ok is false the key falls
// through to the shared field-navigation tail (handleNavAndInput).
func (m FormModel) handleKey(msg tea.KeyMsg) (FormModel, tea.Cmd) {
	key := msg.String()

	if m.focused == fJobTitles {
		if r, cmd, ok := m.handleJobTitlesKey(key); ok {
			return r, cmd
		}
	}
	if m.focused == fLocations {
		if r, cmd, ok := m.handleLocationsKey(key); ok {
			return r, cmd
		}
	}
	if m.focused == fAIAssist {
		if r, cmd, ok := m.handleAIAssistKey(key); ok {
			return r, cmd
		}
	}
	if m.focused == fAIProvider {
		if r, cmd, ok := m.handleAIProviderKey(key); ok {
			return r, cmd
		}
	}
	if m.focused == fLocalLLMModel {
		if r, cmd, ok := m.handleLocalLLMKey(key); ok {
			return r, cmd
		}
	}
	if m.focused == fScraperTargets {
		if r, cmd, ok := m.handleScraperKey(key); ok {
			return r, cmd
		}
	}
	if m.focused == fWorkType {
		if r, cmd, ok := m.handleWorkTypeKey(key); ok {
			return r, cmd
		}
	}
	if m.focused == fNotifyChannels {
		if r, cmd, ok := m.handleNotifyChannelsKey(key); ok {
			return r, cmd
		}
	}
	if m.focused == fCurrency {
		if r, cmd, ok := m.handleCurrencyKey(key); ok {
			return r, cmd
		}
	}
	if m.focused == fMinSalary {
		if r, cmd, ok := m.handleSalaryKey(key); ok {
			return r, cmd
		}
	}
	// Apply Safety custom widgets (consent / work auth / cover letter).
	if r, cmd, ok := m.updateApplySafetyKeys(key); ok {
		return r, cmd
	}
	// Resume path: library picker, autocomplete, and blur/analyse on nav.
	if m.focused == fResumePath {
		if r, cmd, ok := m.handleResumePathKey(key); ok {
			return r, cmd
		}
	}
	// Shared field navigation + token-field shortcuts + text input forwarding.
	return m.handleNavAndInput(msg)
}

// handleNavAndInput is the shared tail of key handling: tab/enter/↓ and
// shift+tab/↑ field navigation (with local-LLM refresh and resume analysis when
// needed), ctrl+x to clear a masked field, ctrl+t to test a notifier, and
// finally forwarding unhandled keys to the active textinput.
func (m FormModel) handleNavAndInput(msg tea.KeyMsg) (FormModel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "tab", "enter", "down":
		if !isCustomField(m.focused) {
			m.inputs[m.focused].Blur()
		}
		m.focused = m.nextVisibleField(m.focused, +1)
		if !isCustomField(m.focused) {
			m.inputs[m.focused].Focus()
		}
		cmds := []tea.Cmd{textinput.Blink, m.saveCmd()}
		if m.aiAssist && m.aiProvider == "local" {
			cmds = append(cmds, refreshLocalLLMCmd(m.inputs[fLocalLLMURL].Value()))
		}
		if m.needsAIProfile() {
			path := strings.TrimSpace(m.inputs[fResumePath].Value())
			var c tea.Cmd
			m, c = m.startResumeAnalysis(path)
			cmds = append(cmds, c)
		}
		return m, tea.Batch(cmds...)

	case "shift+tab", "up":
		if !isCustomField(m.focused) {
			m.inputs[m.focused].Blur()
		}
		m.focused = m.nextVisibleField(m.focused, -1)
		if !isCustomField(m.focused) {
			m.inputs[m.focused].Focus()
		}
		return m, tea.Batch(textinput.Blink, m.saveCmd())
	}

	// ctrl+x clears a masked/token field
	if key == "ctrl+x" && isMaskedField(m.focused) {
		m.inputs[m.focused].Reset()
		return m, tea.Batch(textinput.Blink, m.saveCmd())
	}

	// ctrl+t sends a test notification for Discord/Telegram fields
	if key == "ctrl+t" && isNotifyField(m.focused) {
		return m, func() tea.Msg { return TestNotifyMsg{Cfg: m.toConfig()} }
	}

	// Forward to textinput — auto-save as you type (no ctrl+s)
	if !isCustomField(m.focused) {
		var cmd tea.Cmd
		m.inputs[m.focused], cmd = m.inputs[m.focused].Update(tea.KeyMsg(msg))
		if m.focused == fResumePath {
			m.resumeLibFocus = false
			m.updateAC(m.inputs[fResumePath].Value())
		}
		if m.focused == fLocations {
			m.updateLocationAC(m.inputs[fLocations].Value())
		}
		return m, tea.Batch(cmd, m.saveCmd())
	}
	return m, nil
}

// isCustomField reports whether f is rendered by a custom widget (not a plain
// textinput), so navigation knows to skip textinput forwarding for it.
func isCustomField(f int) bool {
	return f == fWorkType || f == fCurrency || f == fMinSalary || f == fNotifyChannels || f == fAIAssist || f == fAIProvider || f == fLocalLLMModel || f == fApplyConsent || f == fWorkAuth || f == fCoverLetterMode
}

// applyAIAssistChoice commits the highlighted AI Assist yes/no cursor to state.
func (m *FormModel) applyAIAssistChoice() {
	m.aiAssist = m.aiAssistCursor == 0
	if !m.aiAssist {
		return
	}
	// Enabling AI — default to local LLM if no backend chosen yet.
	if m.aiProvider != "local" && m.aiProvider != "api" {
		m.aiProvider = "local"
		m.aiProviderCursor = 0
	}
}

// needsAIProfile is true when Assist is on but we have no usable cached profile.
func (m FormModel) needsAIProfile() bool {
	if !m.aiAssist {
		return false
	}
	p := m.resumeAnalysisResult.Profile
	return p == nil || p.Summary == ""
}

// applyAIProviderChoice commits the highlighted Local LLM / API Keys cursor to
// state, defaulting the local LLM URL when switching to Local.
func (m *FormModel) applyAIProviderChoice() {
	if m.aiProviderCursor == 0 {
		m.aiProvider = "local"
		if strings.TrimSpace(m.inputs[fLocalLLMURL].Value()) == "" {
			m.inputs[fLocalLLMURL].SetValue(localllm.DefaultURL)
		}
	} else {
		m.aiProvider = "api"
	}
}

// fieldVisible returns false for AI credential fields that don't apply
// given the current Use AI Assist / AI Backend choices.
func (m FormModel) fieldVisible(f int) bool {
	switch f {
	case fAIProvider:
		return m.aiAssist
	case fAnthropicKey, fOpenAIKey:
		return m.aiAssist && m.aiProvider == "api"
	case fLocalLLMURL, fLocalLLMModel:
		return m.aiAssist && m.aiProvider == "local"
	case fCoverLetterText:
		return m.coverLetterMode == "template"
	default:
		return true
	}
}

// nextVisibleField walks dir (+1/-1) until a visible, unlocked field is found.
func (m FormModel) nextVisibleField(from, dir int) int {
	next := from
	for i := 0; i < fieldCount; i++ {
		next = (next + dir + fieldCount) % fieldCount
		if !m.fieldVisible(next) {
			continue
		}
		if m.resumeInvalid() && isLockedByResume(next) {
			return fResumePath
		}
		return next
	}
	return from
}

// maskDots returns a string of n bullet characters, capped at a visible max.
func maskDots(n int) string {
	if n <= 0 {
		return "—"
	}
	const max = 32
	if n > max {
		n = max
	}
	dots := make([]rune, n)
	for i := range dots {
		dots[i] = '•'
	}
	return string(dots)
}

// isMaskedField reports whether f holds a secret that should be echoed as dots.
func isMaskedField(f int) bool {
	return f == fLinkedInKey || f == fIndeedKey || f == fAnthropicKey || f == fOpenAIKey ||
		f == fGmailPassword || f == fHunterKey || f == fApolloKey ||
		f == fDiscordWebhook || f == fTelegramBotToken
}

// isNotifyField reports whether f supports the ctrl+t test-notification shortcut.
func isNotifyField(f int) bool {
	return f == fDiscordWebhook || f == fTelegramBotToken || f == fTelegramChatID || f == fNotifyChannels
}

// resumeInvalid returns true when analysis finished and the file is not a valid resume.
// While still analyzing (or not yet triggered) this returns false so fields stay open.
// Always returns false when skipResumeCheck is enabled.
func (m FormModel) resumeInvalid() bool {
	return !m.skipResumeCheck && m.resumeAnalysisDone && !m.resumeAnalysisResult.Valid
}

// isLockedByResume reports whether f is a job-preference field that stays locked
// until a valid resume path is set.
func isLockedByResume(f int) bool {
	switch f {
	case fCity, fYearsExp, fJobTitles, fWorkType, fLocations, fCurrency, fMinSalary:
		return true
	}
	return false
}
