package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

func mergeJobTitleTags(existing, add []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(existing)+len(add))
	for _, t := range existing {
		k := strings.ToLower(strings.TrimSpace(t))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, strings.TrimSpace(t))
	}
	for _, t := range add {
		k := strings.ToLower(strings.TrimSpace(t))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, strings.TrimSpace(t))
	}
	return out
}

func (m *FormModel) clampJobTitleCursor() {
	n := len(m.jobTitleTags)
	if n == 0 {
		m.jobTitleCursor = 0
		return
	}
	if m.jobTitleCursor < 0 {
		m.jobTitleCursor = 0
	}
	if m.jobTitleCursor >= n {
		m.jobTitleCursor = n - 1
	}
}

func (m FormModel) startJobTitlesSuggest(intent string) (FormModel, tea.Cmd) {
	intent = strings.TrimSpace(intent)
	if intent == "" {
		intent = strings.TrimSpace(m.jobIntent)
	}
	if intent == "" {
		m.jobTitlesSuggestErr = "Type what kind of job you want, then press enter"
		m.err = fmt.Errorf("%s", m.jobTitlesSuggestErr)
		return m, nil
	}
	if !m.aiAssist {
		m.jobTitlesSuggestErr = "Turn on AI Assist above to generate titles from a description"
		m.err = fmt.Errorf("%s", m.jobTitlesSuggestErr)
		return m, nil
	}
	m.jobIntent = intent
	m.jobTitlesSuggesting = true
	m.jobTitlesSuggestErr = ""
	m.jobTitlesPending = nil
	m.err = nil
	m.notifyBanner = ""
	m.jobTitlesSuggestGen++
	gen := m.jobTitlesSuggestGen
	ai := m.aiOptions()
	years := m.inputs[fYearsExp].Value()
	var hints []string
	if m.resumeAnalysisDone && m.resumeAnalysisResult.Profile != nil {
		hints = m.resumeAnalysisResult.Profile.SuitableRoles
	}
	return m, tea.Batch(suggestJobTitlesCmd(gen, intent, years, hints, ai), resumeSpinnerTickCmd())
}

func suggestJobTitlesCmd(gen int, intent, years string, hints []string, ai resume.AIOptions) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		titles, err := resume.SuggestJobTitles(ctx, ai, intent, years, hints)
		return jobTitlesSuggestMsg{Gen: gen, Intent: intent, Titles: titles, Err: err}
	}
}

func (m FormModel) renderJobTitlesField(active bool) string {
	if m.jobTitlesSuggesting {
		frame := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Render(spinnerFrames[m.spinnerFrame])
		intent := m.jobIntent
		if len(intent) > 60 {
			intent = intent[:57] + "…"
		}
		return frame + " " + primaryStyle.Render("Finding titles for: ") + mutedStyle.Render(intent)
	}

	if len(m.jobTitlesPending) > 0 {
		return m.renderJobTitlesPendingChoice(active)
	}

	if !active {
		return m.renderJobTitlesInactive()
	}
	return m.renderJobTitlesActive()
}

func (m FormModel) renderJobTitlesPendingChoice(active bool) string {
	var b strings.Builder
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true)
	b.WriteString(warn.Render(fmt.Sprintf(
		"AI suggested %d titles — you already have %d",
		len(m.jobTitlesPending), len(m.jobTitleTags),
	)))
	b.WriteString("\n")
	b.WriteString(primaryStyle.Render("Add to existing, or replace them all?"))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("New from AI"))
	b.WriteString("\n")
	for i, title := range m.jobTitlesPending {
		if i >= 8 {
			b.WriteString(mutedStyle.Render(fmt.Sprintf("  … +%d more", len(m.jobTitlesPending)-8)))
			b.WriteString("\n")
			break
		}
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  %2d. ", i+1)))
		b.WriteString(primaryStyle.Render(title))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Currently saved"))
	b.WriteString("\n")
	for i, title := range m.jobTitleTags {
		if i >= 6 {
			b.WriteString(mutedStyle.Render(fmt.Sprintf("  … +%d more", len(m.jobTitleTags)-6)))
			b.WriteString("\n")
			break
		}
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  %2d. ", i+1)))
		b.WriteString(primaryStyle.Render(title))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(warn.Render("[a] add to existing    [r] replace all    [esc] discard"))
	if !active {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("(focus Job Titles to choose)"))
	}
	return b.String()
}

func (m FormModel) renderJobTitlesInactive() string {
	n := len(m.jobTitleTags)
	if n == 0 {
		return mutedStyle.Render("—  (describe a role here to generate titles)")
	}
	show := n
	if show > 4 {
		show = 4
	}
	parts := make([]string, 0, show+1)
	for i := 0; i < show; i++ {
		parts = append(parts, primaryStyle.Render(m.jobTitleTags[i]))
	}
	line := mutedStyle.Render(fmt.Sprintf("%d titles · ", n)) + strings.Join(parts, mutedStyle.Render(" · "))
	if n > show {
		line += mutedStyle.Render(" · …")
	}
	if m.jobIntent != "" {
		line += "\n    " + mutedStyle.Render("intent: "+trimIntent(m.jobIntent, 70))
	}
	return line
}

func (m FormModel) renderJobTitlesActive() string {
	var b strings.Builder
	n := len(m.jobTitleTags)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	numStyle := mutedStyle

	if n == 0 {
		b.WriteString(mutedStyle.Render("No titles yet — describe the job you want below."))
		b.WriteString("\n")
	} else {
		b.WriteString(labelStyle.Render(fmt.Sprintf("Your titles (%d)", n)))
		b.WriteString("\n")
		mCopy := m
		mCopy.clampJobTitleCursor()
		cur := mCopy.jobTitleCursor
		for i, tag := range m.jobTitleTags {
			num := fmt.Sprintf("%2d.", i+1)
			if i == cur {
				b.WriteString(selectedStyle.Render(fmt.Sprintf("▸ %s %s", num, tag)))
			} else {
				b.WriteString(numStyle.Render(fmt.Sprintf("  %s ", num)) + primaryStyle.Render(tag))
			}
			b.WriteString("\n")
		}
		sel := m.jobTitleTags[cur]
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render(
			fmt.Sprintf("→ On: %s  (%d of %d)", sel, cur+1, n),
		))
		b.WriteString("\n")
	}

	b.WriteString(m.inputs[fJobTitles].View())
	b.WriteString("\n")

	var help string
	if m.aiAssist {
		help = "↑↓ select title  ·  backspace/x remove selected  ·  type title + enter to add  ·  ctrl+g = expand a description via AI"
	} else {
		help = "↑↓ select title  ·  backspace/x remove  ·  type title + enter to add  ·  enable AI Assist to expand a description"
	}
	b.WriteString(mutedStyle.Render(help))
	if m.jobIntent != "" {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("last intent: " + trimIntent(m.jobIntent, 70)))
	}
	if m.jobTitlesSuggestErr != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(m.jobTitlesSuggestErr))
	}
	return b.String()
}

func trimIntent(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// handleJobTitlesKey handles keys while the Job Titles tag field is focused:
// choosing add/replace for pending AI titles, cursor movement among tags,
// add/remove, and ctrl+g AI expansion. Returns ok=false to fall through to
// field navigation.
func (m FormModel) handleJobTitlesKey(key string) (FormModel, tea.Cmd, bool) {
	// Pending AI titles: choose add vs replace before anything else.
	if len(m.jobTitlesPending) > 0 {
		switch key {
		case "a", "A":
			n := len(m.jobTitlesPending)
			m.jobTitleTags = mergeJobTitleTags(m.jobTitleTags, m.jobTitlesPending)
			m.jobTitlesPending = nil
			m.jobTitleCursor = 0
			m.notifyBanner = fmt.Sprintf("✓ Added %d titles (kept existing)", n)
			return m, m.saveCmd(), true
		case "r", "R":
			n := len(m.jobTitlesPending)
			m.jobTitleTags = append([]string(nil), m.jobTitlesPending...)
			m.jobTitlesPending = nil
			m.jobTitleCursor = 0
			m.notifyBanner = fmt.Sprintf("✓ Replaced with %d new titles", n)
			return m, m.saveCmd(), true
		case "esc", "n", "N":
			m.jobTitlesPending = nil
			m.notifyBanner = "Discarded AI titles"
			return m, nil, true
		default:
			return m, nil, true // ignore other keys until chosen
		}
	}
	switch key {
	case "ctrl+g":
		m, cmd := m.startJobTitlesSuggest(strings.TrimSpace(m.inputs[fJobTitles].Value()))
		return m, cmd, true
	case "up", "k", "left", "h":
		// Vertical list: ↑/↓ move among titles when not typing.
		if m.inputs[fJobTitles].Value() == "" && len(m.jobTitleTags) > 0 {
			m.clampJobTitleCursor()
			if m.jobTitleCursor > 0 {
				m.jobTitleCursor--
				return m, nil, true
			}
			// First title: ↑ leaves field; hj/← stay put (don't type into input).
			if key == "up" {
				break // fall through to previous-field nav
			}
			return m, nil, true
		}
	case "down", "j", "right", "l":
		if m.inputs[fJobTitles].Value() == "" && len(m.jobTitleTags) > 0 {
			m.clampJobTitleCursor()
			if m.jobTitleCursor < len(m.jobTitleTags)-1 {
				m.jobTitleCursor++
				return m, nil, true
			}
			if key == "down" {
				break // fall through to next-field nav
			}
			return m, nil, true
		}
	case "enter":
		val := strings.TrimSpace(m.inputs[fJobTitles].Value())
		if val != "" {
			// Enter always adds the literal text as typed — ctrl+g is the
			// explicit "expand this via AI" action. (Enter used to hand
			// off to AI expansion here when AI Assist was on, with
			// ctrl+enter as the only literal-add escape hatch — but most
			// terminals don't send a distinct sequence for ctrl+enter, so
			// that path silently never fired for most users.)
			m.jobTitleTags = mergeJobTitleTags(m.jobTitleTags, []string{val})
			m.inputs[fJobTitles].SetValue("")
			m.jobTitleCursor = len(m.jobTitleTags) - 1
			return m, m.saveCmd(), true
		}
		// Empty enter with saved intent + AI → regenerate
		if m.aiAssist && strings.TrimSpace(m.jobIntent) != "" {
			m, cmd := m.startJobTitlesSuggest("")
			return m, cmd, true
		}
		// Empty → fall through to next field
	case "backspace", "x", "delete":
		if m.inputs[fJobTitles].Value() == "" && len(m.jobTitleTags) > 0 {
			m.clampJobTitleCursor()
			idx := m.jobTitleCursor
			m.jobTitleTags = append(m.jobTitleTags[:idx], m.jobTitleTags[idx+1:]...)
			m.clampJobTitleCursor()
			return m, m.saveCmd(), true
		}
	}
	return m, nil, false
}
