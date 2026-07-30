package ui

// Package ui — form_llm.go
// Local LLM picker and runtime-setup widget for the Config form.
// Handles the fLocalLLMModel field: online model list, offline "install Ollama"
// setup menu, background pull command, and async status messages.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/localllm"
)

// ── Async messages ────────────────────────────────────────────────────────────

// localLLMStatusMsg is the async result of pinging the Ollama runtime.
type localLLMStatusMsg struct {
	Installed []string
	Err       error
}

// localLLMPullDoneMsg is the async result of pulling (installing) a local model.
type localLLMPullDoneMsg struct {
	Model string
	Err   error
}

// ── Commands ──────────────────────────────────────────────────────────────────

// refreshLocalLLMCmd pings the Ollama runtime and lists installed models.
func refreshLocalLLMCmd(baseURL string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		client := localllm.NewClient(baseURL)
		if err := client.Ping(ctx); err != nil {
			return localLLMStatusMsg{Err: err}
		}
		installed, err := client.ListInstalled(ctx)
		if err != nil {
			return localLLMStatusMsg{Err: err}
		}
		return localLLMStatusMsg{Installed: installed}
	}
}

// pullLocalLLMCmd pulls (installs) a model via the Ollama runtime.
func pullLocalLLMCmd(baseURL, model string) tea.Cmd {
	return func() tea.Msg {
		client := localllm.NewClient(baseURL)
		ctx := context.Background()
		if err := client.Ping(ctx); err != nil {
			return localLLMPullDoneMsg{Model: model, Err: err}
		}
		err := client.Pull(ctx, model, nil)
		return localLLMPullDoneMsg{Model: model, Err: err}
	}
}

// ── Key handlers ──────────────────────────────────────────────────────────────

// handleLLMPickerKey navigates the online model list.
// Pressing up at the top / down at the bottom leaves the field.
func (m FormModel) handleLLMPickerKey(key string) (FormModel, tea.Cmd) {
	n := len(m.llmOptions)
	switch key {
	case "up", "k":
		if n == 0 || m.llmCursor <= 0 {
			m.focused = m.nextVisibleField(m.focused, -1)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			return m, tea.Batch(textinput.Blink, m.saveCmd())
		}
		m.llmCursor--
		return m, nil

	case "down", "j":
		if n == 0 || m.llmCursor >= n-1 {
			m.focused = m.nextVisibleField(m.focused, +1)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			return m, tea.Batch(textinput.Blink, m.saveCmd())
		}
		m.llmCursor++
		return m, nil

	case " ", "enter":
		if m.llmInstalling || n == 0 {
			return m, nil
		}
		sel := m.llmOptions[m.llmCursor]
		if !sel.Fits {
			m.err = fmt.Errorf("%s needs ~%dGB RAM (this machine: %dGB)", sel.DisplayName, sel.MinRAMGB, m.llmMachine.RAMGB)
			return m, nil
		}
		if sel.Installed {
			m.inputs[fLocalLLMModel].SetValue(sel.Name)
			m.notifyBanner = "✓ Using " + sel.DisplayName
			return m, m.saveCmd()
		}
		m.err = nil
		m.llmInstalling = true
		m.llmStatus = "installing " + sel.Name + "…"
		url := m.inputs[fLocalLLMURL].Value()
		return m, tea.Batch(pullLocalLLMCmd(url, sel.Name), resumeSpinnerTickCmd())

	default:
		return m, nil
	}
}

// handleLLMSetupKey handles the offline "install runtime" menu.
func (m FormModel) handleLLMSetupKey(key string) (FormModel, tea.Cmd) {
	opts := localllm.SetupOptions()
	n := len(opts)
	switch key {
	case "up", "k":
		if m.llmSetupCursor <= 0 {
			m.focused = m.nextVisibleField(m.focused, -1)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			return m, tea.Batch(textinput.Blink, m.saveCmd())
		}
		m.llmSetupCursor--
		return m, nil

	case "down", "j":
		if m.llmSetupCursor >= n-1 {
			m.focused = m.nextVisibleField(m.focused, +1)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			return m, tea.Batch(textinput.Blink, m.saveCmd())
		}
		m.llmSetupCursor++
		return m, nil

	case " ", "enter":
		return m, m.runLLMSetupOption(opts[m.llmSetupCursor])

	default:
		return m, nil
	}
}

// runLLMSetupOption executes the selected runtime-setup action.
func (m FormModel) runLLMSetupOption(opt localllm.RuntimeOption) tea.Cmd {
	url := m.inputs[fLocalLLMURL].Value()
	switch opt.ID {
	case "start-ollama":
		return func() tea.Msg {
			if err := localllm.StartOllama(); err != nil {
				return localLLMStatusMsg{Err: err}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			client := localllm.NewClient(url)
			if err := client.Ping(ctx); err != nil {
				return localLLMStatusMsg{Err: err}
			}
			installed, err := client.ListInstalled(ctx)
			if err != nil {
				return localLLMStatusMsg{Err: err}
			}
			return localLLMStatusMsg{Installed: installed}
		}
	case "install-ollama", "install-lmstudio":
		download := opt.DownloadURL
		return func() tea.Msg {
			if err := localllm.OpenURL(download); err != nil {
				return localLLMPullDoneMsg{Err: err}
			}
			return localLLMStatusMsg{Err: fmt.Errorf("opened %s — install, start it, then Retry", download)}
		}
	case "retry":
		return refreshLocalLLMCmd(url)
	default:
		return nil
	}
}

// ── State helpers ─────────────────────────────────────────────────────────────

// refreshLLMOptions rebuilds the model recommendation list from the probe +
// the currently-installed set. Preserves the cursor on the current model or
// falls back to the "best" model for this machine.
func (m *FormModel) refreshLLMOptions(installed []string) {
	recs := localllm.Recommend(m.llmMachine, installed)
	m.llmOptions = localllm.TopFits(recs, 7)
	cur := strings.TrimSpace(m.inputs[fLocalLLMModel].Value())
	m.llmCursor = 0
	for i, r := range m.llmOptions {
		if cur != "" && (r.Name == cur || strings.HasPrefix(cur, r.Name+":")) {
			m.llmCursor = i
			return
		}
	}
	for i, r := range m.llmOptions {
		if r.Best {
			m.llmCursor = i
			return
		}
	}
}

// ── Render helpers ────────────────────────────────────────────────────────────

// renderLocalLLMPicker renders the fLocalLLMModel field.
// When active it shows either the online model list or the offline setup menu.
func (m FormModel) renderLocalLLMPicker(active bool) string {
	selected := strings.TrimSpace(m.inputs[fLocalLLMModel].Value())
	hwText := fmt.Sprintf("machine: %dGB RAM · %s", m.llmMachine.RAMGB, m.llmMachine.CPU)
	if m.llmMachine.GPUName != "" {
		if m.llmMachine.GPUVRAMGB > 0 {
			hwText += fmt.Sprintf(" · %s (%dGB VRAM)", m.llmMachine.GPUName, m.llmMachine.GPUVRAMGB)
		} else {
			hwText += " · " + m.llmMachine.GPUName
		}
	}
	hw := mutedStyle.Render(hwText)

	if !active {
		if m.llmOffline {
			return mutedStyle.Render("— runtime not running") + "  " + hw
		}
		if selected == "" {
			return mutedStyle.Render("— not selected") + "  " + hw
		}
		return primaryStyle.Render(selected) + "  " + hw
	}

	if m.llmInstalling {
		frame := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Render(spinnerFrames[m.spinnerFrame])
		return frame + " " + primaryStyle.Render(m.llmStatus) + "\n    " + hw
	}
	if m.llmOffline {
		return m.renderLLMSetupMenu(hw)
	}
	if len(m.llmOptions) == 0 {
		return errorStyle.Render("no suitable models for this machine") +
			"\n    " + hw +
			"\n    " + mutedStyle.Render("shift+tab / ↑ leave · tab next")
	}

	var b strings.Builder
	b.WriteString(hw)
	for i, r := range m.llmOptions {
		label := fmt.Sprintf("%s (%s)", r.DisplayName, r.Size)
		meta := r.Notes
		if r.Best {
			meta = "★ best for this machine · " + meta
		}
		if r.Installed {
			meta = "installed · " + meta
		} else {
			meta = "enter to install · " + meta
		}
		mark := "  "
		var line string
		if i == m.llmCursor {
			mark = "▶ "
			line = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(label) +
				"  " + mutedStyle.Render(meta)
		} else if r.Installed {
			line = primaryStyle.Render(label) + "  " + mutedStyle.Render(meta)
		} else {
			line = mutedStyle.Render(label + "  " + meta)
		}
		b.WriteString("\n    " + mark + line)
	}
	b.WriteString("\n    " + mutedStyle.Render("↑↓ move · enter install/use · ↑ on first / tab leaves"))
	return b.String()
}

// renderLLMSetupMenu renders the offline "install a runtime" action menu.
func (m FormModel) renderLLMSetupMenu(hw string) string {
	var b strings.Builder
	b.WriteString(errorStyle.Render("Local runtime not reachable"))
	b.WriteString("\n    " + hw)
	if m.llmStatus != "" {
		b.WriteString("\n    " + mutedStyle.Render(m.llmStatus))
	}
	b.WriteString("\n    " + mutedStyle.Render("Install or start a runtime, then retry:"))
	for i, opt := range localllm.SetupOptions() {
		hint := opt.Hint
		if opt.ID == "start-ollama" && !localllm.OllamaInstalled() {
			hint = "ollama not on PATH — use Install instead"
		}
		label := opt.Label
		mark := "  "
		var line string
		if i == m.llmSetupCursor {
			mark = "▶ "
			line = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(label) +
				"  " + mutedStyle.Render(hint)
		} else {
			line = mutedStyle.Render(label + "  " + hint)
		}
		b.WriteString("\n    " + mark + line)
	}
	b.WriteString("\n    " + mutedStyle.Render(
		"↑↓ move · enter run · ↑ on first / shift+tab / esc leaves · tab next",
	))
	return b.String()
}

// renderAISectionHint explains the two AI setup paths shown above the AI section.
func (m FormModel) renderAISectionHint() string {
	if !m.aiAssist {
		return "    " + mutedStyle.Render("Enable AI Assist to set up a local LLM or cloud API keys.") + "\n"
	}
	return "    " + mutedStyle.Render("Choose Local LLM (runs on this machine) or API Keys (Anthropic / OpenAI).") + "\n"
}

// renderAIAssist renders the yes/no toggle for enabling AI improvements.
func (m FormModel) renderAIAssist(active bool) string {
	if !active {
		if m.aiAssist {
			return primaryStyle.Render("Yes")
		}
		return mutedStyle.Render("No")
	}
	yesLabel, noLabel := "Yes", "No"
	if m.aiAssistCursor == 0 {
		yesLabel = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render("[ Yes ]")
		noLabel = primaryStyle.Render("No")
	} else {
		yesLabel = primaryStyle.Render("Yes")
		noLabel = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render("[ No ]")
	}
	help := mutedStyle.Render("   use AI to improve applications · ←→ pick · enter confirm · tab next")
	return yesLabel + "   " + noLabel + help
}

// renderAIProvider renders the Local LLM vs API Keys selector.
func (m FormModel) renderAIProvider(active bool) string {
	if !active {
		if m.aiProvider == "api" {
			return primaryStyle.Render("API Keys")
		}
		return primaryStyle.Render("Local LLM")
	}
	localLabel, apiLabel := "Local LLM", "API Keys"
	if m.aiProviderCursor == 0 {
		localLabel = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render("[ Local LLM ]")
		apiLabel = primaryStyle.Render("API Keys")
	} else {
		localLabel = primaryStyle.Render("Local LLM")
		apiLabel = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render("[ API Keys ]")
	}
	help := mutedStyle.Render("   ←→ pick · enter confirm · tab next")
	return localLabel + "   " + apiLabel + help
}
