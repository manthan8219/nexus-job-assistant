package ui

// Package ui — form_resume.go
// Resume Path field support: the JobPilot-generated resume library picker, path
// autocomplete, async analysis commands, and handleResumePathKey. The handler
// returns ok=false for unhandled keys so they fall through to shared navigation.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/nexusdir"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// handleResumePathKey handles the Resume Path field: the JobPilot-generated resume
// library picker (ctrl+j/k), path autocomplete, and the blur/analyse behaviour
// on navigation. Returns ok=false for unhandled keys so they fall through to
// the shared field navigation + textinput forwarding.
func (m FormModel) handleResumePathKey(key string) (FormModel, tea.Cmd, bool) {
	m.loadResumeLibrary()

	// JobPilot-generated resume library (ctrl+j/k · enter to use)
	if len(m.resumeLib) > 0 {
		switch key {
		case "ctrl+j":
			m.resumeLibFocus = true
			m.acSuggestions = nil
			m.acIdx = -1
			if m.resumeLibIdx < len(m.resumeLib)-1 {
				m.resumeLibIdx++
			}
			return m, nil, true
		case "ctrl+k":
			m.resumeLibFocus = true
			m.acSuggestions = nil
			m.acIdx = -1
			if m.resumeLibIdx > 0 {
				m.resumeLibIdx--
			}
			return m, nil, true
		case "enter":
			if m.resumeLibFocus {
				m, cmd := m.applyResumeLibrarySelection()
				return m, cmd, true
			}
		}
	}

	// Autocomplete navigation (resume path only)
	if len(m.acSuggestions) > 0 {
		switch key {
		case "down", "ctrl+n":
			if m.acIdx < len(m.acSuggestions)-1 {
				m.acIdx++
			}
			return m, nil, true
		case "up", "ctrl+p":
			if m.acIdx > 0 {
				m.acIdx--
			}
			return m, nil, true
		case "esc":
			m.acSuggestions = nil
			m.acIdx = -1
			return m, nil, true
		case "tab", "enter":
			// Select highlighted item, or the first suggestion if none highlighted yet.
			idx := m.acIdx
			if idx < 0 {
				idx = 0
			}
			sel := m.acSuggestions[idx]
			m.inputs[fResumePath].SetValue(sel)
			m.acSuggestions = nil
			m.acIdx = -1
			if strings.HasSuffix(sel, "/") {
				// Directory — re-expand so the user can keep drilling down.
				m.updateAC(sel)
				return m, nil, true
			}
			// File selected — blur field, analyze only if path changed, move next.
			m.inputs[fResumePath].Blur()
			m.focused = m.nextVisibleField(fResumePath, +1)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			cmds := []tea.Cmd{textinput.Blink, m.saveCmd()}
			if !m.skipResumeCheck && m.resumePathChanged(sel) {
				var c tea.Cmd
				m, c = m.startResumeAnalysis(sel)
				cmds = append(cmds, c)
			}
			return m, tea.Batch(cmds...), true
		}
	}

	// Resume path — trigger async analysis on blur
	switch key {
	case "tab", "down", "shift+tab", "up", "enter":
		// Dismiss autocomplete on blur.
		m.acSuggestions = nil
		m.acIdx = -1
		path := strings.TrimSpace(m.inputs[fResumePath].Value())
		if !isCustomField(m.focused) {
			m.inputs[m.focused].Blur()
		}
		dir := +1
		if key == "shift+tab" || key == "up" {
			dir = -1
		}
		m.focused = m.nextVisibleField(fResumePath, dir)
		if !isCustomField(m.focused) {
			m.inputs[m.focused].Focus()
		}
		cmds := []tea.Cmd{textinput.Blink, m.saveCmd()}
		if path != "" && !m.skipResumeCheck && m.resumePathChanged(path) {
			var c tea.Cmd
			m, c = m.startResumeAnalysis(path)
			cmds = append(cmds, c)
		} else if path == "" {
			m.resumeAnalyzing = false
			m.resumeAnalysisDone = false
			m.lastAnalyzedPath = ""
		}
		return m, tea.Batch(cmds...), true
	}
	return m, nil, false
}

// loadResumeLibrary refreshes the list of JobPilot-generated resume PDFs from disk.
func (m *FormModel) loadResumeLibrary() {
	vers, err := resume.ListVersions()
	if err != nil {
		m.resumeLib = nil
		return
	}
	m.resumeLib = vers
	if m.resumeLibIdx >= len(m.resumeLib) {
		m.resumeLibIdx = max(0, len(m.resumeLib)-1)
	}
}

// applyResumeLibrarySelection sets the resume path to the picked JobPilot PDF,
// blurs the field, moves on, and kicks off analysis when the path changed.
func (m FormModel) applyResumeLibrarySelection() (FormModel, tea.Cmd) {
	if m.resumeLibIdx < 0 || m.resumeLibIdx >= len(m.resumeLib) {
		return m, nil
	}
	sel := m.resumeLib[m.resumeLibIdx].PDFPath
	m.inputs[fResumePath].SetValue(sel)
	m.resumeLibFocus = false
	m.acSuggestions = nil
	m.acIdx = -1
	m.inputs[fResumePath].Blur()
	m.focused = m.nextVisibleField(fResumePath, +1)
	if !isCustomField(m.focused) {
		m.inputs[m.focused].Focus()
	}
	cmds := []tea.Cmd{textinput.Blink, m.saveCmd()}
	if !m.skipResumeCheck && m.resumePathChanged(sel) {
		var c tea.Cmd
		m, c = m.startResumeAnalysis(sel)
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

// renderResumeLibrary renders the JobPilot-generated resume picker below the path
// input. Always reads disk so Config stays in sync after Resume → New resume.
func (m FormModel) renderResumeLibrary() string {
	vers, err := resume.ListVersions()
	if err != nil || len(vers) == 0 {
		return "\n    " + mutedStyle.Render("No JobPilot PDFs yet — generate one under Resume → New resume")
	}
	idx := m.resumeLibIdx
	if idx < 0 || idx >= len(vers) {
		idx = 0
	}
	var b strings.Builder
	b.WriteString("\n    " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render(
		fmt.Sprintf("JobPilot generated (%d)  ·  ctrl+j/k pick  ·  enter use  ·  or type your own path", len(vers)),
	))
	limit := len(vers)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		v := vers[i]
		line := v.DisplayLine()
		prefix := "  "
		style := mutedStyle
		if i == idx && m.resumeLibFocus {
			prefix = "▶ "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
		} else if i == idx {
			prefix = "· "
			style = primaryStyle
		}
		b.WriteString("\n    " + style.Render(prefix+line))
	}
	if len(vers) > limit {
		b.WriteString("\n    " + mutedStyle.Render(fmt.Sprintf("  … +%d more in ~/.nexus/resumes/", len(vers)-limit)))
	}
	return b.String()
}

// updateAC refreshes path autocomplete suggestions: the directories and
// .pdf/.doc(x) files that match the current prefix.
func (m *FormModel) updateAC(input string) {
	if input == "" {
		m.acSuggestions = nil
		m.acIdx = -1
		return
	}

	// Expand leading ~
	expanded := input
	if input == "~" || input == "~/" {
		expanded = nexusdir.UserHome() + "/"
	} else if strings.HasPrefix(input, "~/") {
		expanded = filepath.Join(nexusdir.UserHome(), input[2:])
		if strings.HasSuffix(input, "/") {
			expanded += "/"
		}
	}

	// Split into the directory to list and the prefix to filter by.
	var dir, prefix string
	if strings.HasSuffix(expanded, "/") {
		dir = expanded
		prefix = ""
	} else {
		dir = filepath.Dir(expanded)
		prefix = filepath.Base(expanded)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		m.acSuggestions = nil
		return
	}

	var suggestions []string
	for _, e := range entries {
		name := e.Name()
		// Skip hidden files unless the user explicitly typed a dot.
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		full := filepath.Join(dir, name)
		if e.IsDir() {
			suggestions = append(suggestions, full+"/")
		} else {
			ext := strings.ToLower(filepath.Ext(name))
			if ext == ".pdf" || ext == ".docx" || ext == ".doc" {
				suggestions = append(suggestions, full)
			}
		}
		if len(suggestions) >= 8 {
			break
		}
	}

	m.acSuggestions = suggestions
	m.acIdx = -1
}

// analyzeResumeCmd validates the resume and, when AI Assist is on, builds a career profile.
func analyzeResumeCmd(path string, gen int, ai resume.AIOptions) tea.Cmd {
	return func() tea.Msg {
		r := resume.AnalyzeFull(path, ai)
		return ResumeAnalysisDoneMsg{Gen: gen, Result: r, Path: path}
	}
}

func resumeAnalysisStartCmd(path string, gen int) tea.Cmd {
	return func() tea.Msg { return ResumeAnalysisStartMsg{Gen: gen, Path: path} }
}

// aiOptions builds the resume.AIOptions snapshot from current form state.
func (m FormModel) aiOptions() resume.AIOptions {
	return resume.AIOptions{
		Enabled:       m.aiAssist,
		Provider:      m.aiProviderValue(),
		LocalURL:      m.inputs[fLocalLLMURL].Value(),
		LocalModel:    m.inputs[fLocalLLMModel].Value(),
		AnthropicKey:  m.inputs[fAnthropicKey].Value(),
		OpenAIKey:     m.inputs[fOpenAIKey].Value(),
		GoogleKey:     m.inputs[fGoogleKey].Value(),
		DeepSeekKey:   m.inputs[fDeepSeekKey].Value(),
		GroqKey:       m.inputs[fGroqKey].Value(),
		MistralKey:    m.inputs[fMistralKey].Value(),
		TogetherKey:   m.inputs[fTogetherKey].Value(),
		OpenRouterKey: m.inputs[fOpenRouterKey].Value(),
		XAIKey:        m.inputs[fXAIKey].Value(),
	}
}

// resumePathChanged reports whether path differs from the last analyzed file.
func (m FormModel) resumePathChanged(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	return filepath.Clean(path) != filepath.Clean(m.lastAnalyzedPath)
}

// startResumeAnalysis kicks off analysis (used on path change or manual refresh).
func (m FormModel) startResumeAnalysis(path string) (FormModel, tea.Cmd) {
	path = strings.TrimSpace(path)
	if path == "" || m.skipResumeCheck {
		return m, nil
	}
	m.resumeAnalysisGen++
	m.resumeAnalyzing = true
	m.resumeAnalysisDone = false
	gen := m.resumeAnalysisGen
	return m, tea.Batch(
		resumeAnalysisStartCmd(path, gen),
		analyzeResumeCmd(path, gen, m.aiOptions()),
		resumeSpinnerTickCmd(),
	)
}

// resumeSpinnerTickCmd fires every 80ms to animate the spinner.
func resumeSpinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return resumeSpinnerTickMsg(t)
	})
}

// resumeStatusSuffix returns the inline badge shown beside the resume path.
func (m FormModel) resumeStatusSuffix() string {
	if m.skipResumeCheck {
		return "  " + mutedStyle.Render("(validation skipped)")
	}
	switch {
	case m.resumeAnalyzing:
		frame := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPurple)).
			Render(spinnerFrames[m.spinnerFrame])
		label := "Analyzing resume..."
		if m.aiAssist {
			label = "AI analyzing resume — open Resume tab for live status..."
		}
		return "  " + frame + " " + mutedStyle.Render(label)
	case m.resumeAnalysisDone && m.resumeAnalysisResult.Valid:
		msg := m.resumeAnalysisResult.Message
		if m.resumeAnalysisResult.Profile != nil && m.resumeAnalysisResult.Profile.Error == "" && m.resumeAnalysisResult.Profile.Summary != "" {
			msg += " · see Resume tab"
		}
		return "  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).
			Render("✓ "+msg)
	case m.resumeAnalysisDone && !m.resumeAnalysisResult.Valid:
		return "  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).
			Render("✗ "+m.resumeAnalysisResult.Err)
	}
	return ""
}
