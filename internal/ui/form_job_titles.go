package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthanmanthan/nexus/internal/resume"
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
		help = "↑↓ select title  ·  backspace/x remove selected  ·  type description + enter = AI expand  ·  ctrl+enter add one"
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
