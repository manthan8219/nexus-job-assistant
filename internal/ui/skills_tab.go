package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// skillsPersistMsg is returned as a Cmd result to signal skills were saved.
type skillsPersistMsg struct{ err error }

// SkillsTabModel lets the user view, add, and remove their skills.
// Changes are persisted directly to config.json via saveCmd.
type SkillsTabModel struct {
	skills []string
	cursor int
	adding bool
	input  textinput.Model
	status string
	width  int
	height int
}

func NewSkillsTabModel(skills []string) SkillsTabModel {
	ti := textinput.New()
	ti.Placeholder = "e.g. Go, React, PostgreSQL"
	ti.CharLimit = 60
	ti.Width = 40
	ti.Prompt = ""
	s := make([]string, len(skills))
	copy(s, skills)
	return SkillsTabModel{skills: s, input: ti}
}

// Skills returns the current skill list (read by resume hub for prompt injection).
func (m SkillsTabModel) Skills() []string { return m.skills }

// CapturesKeys is true while the add-skill input is focused.
func (m SkillsTabModel) CapturesKeys() bool { return m.adding }

func (m SkillsTabModel) Init() tea.Cmd { return nil }

func (m SkillsTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case skillsPersistMsg:
		if msg.err != nil {
			m.status = "save error: " + msg.err.Error()
		} else {
			m.status = "saved"
		}

	case tea.KeyMsg:
		if m.adding {
			switch msg.String() {
			case "esc":
				m.input.Blur()
				m.adding = false
				m.input.SetValue("")
			case "enter":
				val := strings.TrimSpace(m.input.Value())
				if val != "" {
					// split on comma so user can add multiple at once
					parts := strings.Split(val, ",")
					for _, p := range parts {
						p = strings.TrimSpace(p)
						if p != "" && !m.hasSkill(p) {
							m.skills = append(m.skills, p)
						}
					}
					m.cursor = len(m.skills) - 1
				}
				m.input.Blur()
				m.adding = false
				m.input.SetValue("")
				m.status = ""
				return m, m.saveCmd()
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		switch msg.String() {
		case "n", "a":
			m.adding = true
			m.status = ""
			return m, m.input.Focus()
		case "d", "x", "delete", "backspace":
			if len(m.skills) == 0 {
				return m, nil
			}
			m.skills = append(m.skills[:m.cursor], m.skills[m.cursor+1:]...)
			if m.cursor >= len(m.skills) && m.cursor > 0 {
				m.cursor--
			}
			m.status = ""
			return m, m.saveCmd()
		case "j", "down":
			if m.cursor < len(m.skills)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		}
	}
	return m, nil
}

func (m SkillsTabModel) hasSkill(s string) bool {
	sl := strings.ToLower(s)
	for _, sk := range m.skills {
		if strings.ToLower(sk) == sl {
			return true
		}
	}
	return false
}

func (m SkillsTabModel) saveCmd() tea.Cmd {
	skills := make([]string, len(m.skills))
	copy(skills, m.skills)
	return func() tea.Msg {
		cur, err := config.Load()
		if err != nil || cur == nil {
			cur = &config.Config{}
		}
		cur.Skills = skills
		return skillsPersistMsg{err: config.Save(cur)}
	}
}

func (m SkillsTabModel) View() string {
	var b strings.Builder
	b.WriteString("\n  " + labelStyle.Render("SKILLS") + "\n")
	b.WriteString("  " + mutedStyle.Render("Skills injected into every resume generation run.") + "\n\n")

	if len(m.skills) == 0 && !m.adding {
		b.WriteString("  " + mutedStyle.Render("No skills yet — press n to add your first skill.") + "\n")
	} else {
		for i, sk := range m.skills {
			cursor := "  "
			style := primaryStyle
			if i == m.cursor {
				cursor = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true).Render("▶ ")
				style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
			}
			b.WriteString(cursor + style.Render(sk) + "\n")
		}
	}

	if m.adding {
		b.WriteString("\n  " + mutedStyle.Render("Add skill (comma-separate for multiple):") + "\n")
		b.WriteString("  " + m.input.View() + "\n")
		b.WriteString("  " + mutedStyle.Render("enter save  ·  esc cancel") + "\n")
	} else {
		b.WriteString("\n  " + mutedStyle.Render("n add  ·  d delete  ·  j/k move") + "\n")
	}

	if m.status != "" {
		b.WriteString("  " + mutedStyle.Render(m.status) + "\n")
	}
	return b.String()
}
