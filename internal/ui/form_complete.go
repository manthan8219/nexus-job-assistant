package ui

// Package ui — form_complete.go
// Profile-completion checks and focus helpers for the Config form.

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// IsComplete returns true when all required fields have values.
func (m FormModel) IsComplete() bool {
	return len(m.MissingFields()) == 0
}

// MissingFields returns human-readable names of unfilled required fields.
func (m FormModel) MissingFields() []string {
	var missing []string
	req := []struct {
		idx  int
		name string
	}{
		{fFirstName, "First Name"},
		{fLastName, "Last Name"},
		{fEmail, "Email"},
		{fPhone, "Phone"},
		{fLinkedInID, "LinkedIn ID"},
		{fResumePath, "Resume Path"},
	}
	for _, r := range req {
		val := strings.TrimSpace(m.inputs[r.idx].Value())
		if val == "" {
			missing = append(missing, r.name)
		} else if r.idx == fResumePath && m.resumeAnalysisDone && !m.resumeAnalysisResult.Valid {
			missing = append(missing, "Resume (invalid file)")
		}
	}
	if len(m.jobTitleTags) == 0 {
		missing = append(missing, "Target Job Titles")
	}
	wtAny := false
	for _, s := range m.wtSelected {
		if s {
			wtAny = true
		}
	}
	if !wtAny {
		missing = append(missing, "Work Type")
	}
	if m.salaryPreset < 0 && m.salaryCustom == "" {
		missing = append(missing, "Min Salary")
	}
	return missing
}

// BlurAll removes focus from every text input field.
func (m FormModel) BlurAll() FormModel {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	return m
}

// FocusCurrent focuses the current field, advancing to the next visible one if
// the current field is hidden (e.g. AI fields when AI Assist is off).
func (m FormModel) FocusCurrent() (FormModel, tea.Cmd) {
	if !m.fieldVisible(m.focused) {
		m.focused = m.nextVisibleField(m.focused, +1)
	}
	if isCustomField(m.focused) {
		return m, nil
	}
	return m, m.inputs[m.focused].Focus()
}
