package ui

// Package ui — form_update.go
// The Config form's Bubble Tea Update loop: dispatches messages (window size,
// keys, async results) to handlers. Key handling lives in form_keys.go.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// Update routes a Bubble Tea message to the appropriate handler.
func (m FormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		m.saved = false
		m.err = nil
		m.notifyBanner = ""
		return m.handleKey(msg)

	case SavedMsg:
		m.saved = true
		return m, nil
	case ErrMsg:
		m.err = msg.err
		return m, nil

	case jobTitlesSuggestMsg:
		if msg.Gen != m.jobTitlesSuggestGen {
			return m, nil
		}
		m.jobTitlesSuggesting = false
		if msg.Err != nil {
			m.jobTitlesSuggestErr = msg.Err.Error()
			m.err = msg.Err
			return m, nil
		}
		m.jobTitlesSuggestErr = ""
		m.err = nil
		if strings.TrimSpace(msg.Intent) != "" {
			m.jobIntent = strings.TrimSpace(msg.Intent)
		}
		m.inputs[fJobTitles].SetValue("")
		// No existing titles → just use AI list.
		if len(m.jobTitleTags) == 0 {
			m.jobTitleTags = mergeJobTitleTags(nil, msg.Titles)
			m.jobTitleCursor = 0
			m.jobTitlesPending = nil
			m.notifyBanner = fmt.Sprintf("✓ Set %d job titles from your description", len(msg.Titles))
			return m, m.saveCmd()
		}
		// Ask add vs replace.
		m.jobTitlesPending = mergeJobTitleTags(nil, msg.Titles)
		m.notifyBanner = ""
		m.focused = fJobTitles
		return m, nil

	case ResumeAnalysisDoneMsg:
		if msg.Gen == m.resumeAnalysisGen {
			m.resumeAnalyzing = false
			m.resumeAnalysisDone = true
			m.resumeAnalysisResult = msg.Result
			m.pendingResumeAnalyze = false
			path := msg.Path
			if path == "" {
				path = strings.TrimSpace(m.inputs[fResumePath].Value())
			}
			m.lastAnalyzedPath = path
			if !msg.FromCache && path != "" {
				_ = resume.SaveCache(path, m.aiAssist, msg.Result)
			}
			// Re-save so IsComplete() re-evaluates with the new analysis result.
			return m, m.saveCmd()
		}
		return m, nil

	case resumeSpinnerTickMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		if m.resumeAnalyzing || m.llmInstalling || m.jobTitlesSuggesting {
			return m, resumeSpinnerTickCmd()
		}
		return m, nil

	case localLLMStatusMsg:
		if msg.Err != nil {
			m.llmOffline = true
			m.llmStatus = msg.Err.Error()
			m.refreshLLMOptions(nil)
		} else {
			m.llmOffline = false
			m.llmStatus = ""
			m.refreshLLMOptions(msg.Installed)
		}
		return m, nil

	case localLLMPullDoneMsg:
		m.llmInstalling = false
		if msg.Err != nil {
			m.err = msg.Err
			m.llmStatus = msg.Err.Error()
			return m, nil
		}
		m.inputs[fLocalLLMModel].SetValue(msg.Model)
		m.llmStatus = "installed " + msg.Model
		m.notifyBanner = "✓ Local model ready: " + msg.Model
		return m, tea.Batch(m.saveCmd(), refreshLocalLLMCmd(m.inputs[fLocalLLMURL].Value()))

	case scraperStatusMsg:
		m.scraperInstalling = false
		if msg.err != nil {
			m.scraperStatus = msg.err.Error()
			m.scraperOffline = true
		} else {
			m.scraperOffline = !msg.running
			if msg.installed != nil {
				m.scraperInstalled = msg.installed
			}
			if msg.running {
				m.scraperStatus = "ready"
			} else {
				m.scraperStatus = "failed to start"
			}
		}
		return m, nil
	}

	// Forward to active textinput — auto-save on every edit
	if !isCustomField(m.focused) {
		var cmd tea.Cmd
		m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
		return m, tea.Batch(cmd, m.saveCmd())
	}
	return m, nil
}
