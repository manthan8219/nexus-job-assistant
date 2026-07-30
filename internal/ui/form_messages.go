package ui

// Package ui — form_messages.go
// Bubble Tea message types exchanged between the Config form's async commands
// (resume analysis, job-title suggestions, test notifications) and its Update.

import (
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

// SavedMsg is emitted after the config file is written.
type SavedMsg struct{ Cfg *config.Config }

// ErrMsg carries a save error back to the form.
type ErrMsg struct{ err error }

// ProfileCompleteMsg is fired when all required fields are filled.
type ProfileCompleteMsg struct{ Cfg *config.Config }

// jobTitlesSuggestMsg is the async result of LLM job-title expansion.
type jobTitlesSuggestMsg struct {
	Gen    int
	Intent string
	Titles []string
	Err    error
}

// TestNotifyMsg is fired when the user requests a test notification.
type TestNotifyMsg struct{ Cfg *config.Config }

// ResumeAnalysisStartMsg is fired when resume analysis begins (for the Resume tab).
type ResumeAnalysisStartMsg struct {
	Gen  int
	Path string
}

// ResumeAnalysisDoneMsg carries the result of async resume analysis.
// Gen matches resumeAnalysisGen so stale results from old paths are ignored.
type ResumeAnalysisDoneMsg struct {
	Gen       int
	Result    resume.Result
	Path      string // set when restoring from cache or after analyze
	FromCache bool
}

// resumeSpinnerTickMsg drives the spinner animation while analysis runs.
type resumeSpinnerTickMsg time.Time

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
