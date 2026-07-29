package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthanmanthan/nexus/internal/resume"
	"github.com/manthanmanthan/nexus/internal/workcontext"
)

const (
	resumeSubAnalyze = iota
	resumeSubWork
	resumeSubImprove
	resumeSubCount
)

// Short labels in the step strip (numbered in render).
var resumeSubLabels = [resumeSubCount]string{"Review", "Your work", "New resume"}

// ResumeHubModel groups Analyze, Work, and Improve under one main tab.
type ResumeHubModel struct {
	width, height int
	sub           int
	analyze       ResumeTabModel
	work          WorkTabModel
	improve       ImproveTabModel

	ai         resume.AIOptions
	resumePath string
}

func NewResumeHubModel() ResumeHubModel {
	return ResumeHubModel{
		analyze: NewResumeTabModel(),
		work:    NewWorkTabModel(),
		improve: NewImproveTabModel(),
	}
}

func (m ResumeHubModel) Init() tea.Cmd {
	return tea.Batch(m.work.Init(), m.improve.Init())
}

func (m ResumeHubModel) CapturesKeys() bool {
	if m.sub == resumeSubWork && m.work.CapturesKeys() {
		return true
	}
	if m.sub == resumeSubImprove && m.improve.CapturesKeys() {
		return true
	}
	return false
}

func (m ResumeHubModel) NextSub() ResumeHubModel {
	m.sub = (m.sub + 1) % resumeSubCount
	return m
}

func (m ResumeHubModel) PrevSub() ResumeHubModel {
	m.sub = (m.sub - 1 + resumeSubCount) % resumeSubCount
	return m
}

func (m *ResumeHubModel) SetAIContext(ai resume.AIOptions, resumePath string) {
	m.ai = ai
	m.resumePath = resumePath
}

func (m ResumeHubModel) analyzeDone() bool {
	return m.analyze.hasResult && m.analyze.result.Valid && !m.analyze.analyzing
}

func (m ResumeHubModel) workCount() int {
	return len(m.work.projects)
}

func (m ResumeHubModel) workDone() bool {
	return m.workCount() >= 1
}

func (m ResumeHubModel) improveDone() bool {
	return m.improve.lastOut != nil
}

// recommendedSub is the step the user should do next.
func (m ResumeHubModel) recommendedSub() int {
	if !m.analyzeDone() {
		return resumeSubAnalyze
	}
	if m.workCount() < 4 {
		return resumeSubWork
	}
	if !m.improveDone() {
		return resumeSubImprove
	}
	return resumeSubImprove
}

func (m ResumeHubModel) nextAction() string {
	switch {
	case m.analyze.analyzing:
		return "Wait for AI to finish reading your resume…"
	case !m.analyzeDone():
		return "Set your resume path + AI Assist in Config, then come back here."
	case m.sub == resumeSubAnalyze:
		if m.workCount() == 0 {
			return "Next: add projects you shipped  →  press tab"
		}
		if m.workCount() < 4 {
			return fmt.Sprintf("Next: add more projects (%d/4–6)  →  press tab", m.workCount())
		}
		return "Next: generate a stronger resume  →  press tab twice (or go to New resume)"
	case m.sub == resumeSubWork:
		if m.workCount() == 0 {
			return "Press n · paste a Claude summary for one repo — it auto-saves. Aim for 4–6."
		}
		if m.workCount() < 4 {
			return fmt.Sprintf("%d project(s) saved · press n for another (goal 4–6), then tab → New resume", m.workCount())
		}
		return "Enough projects · press tab → New resume · then g to generate"
	case m.sub == resumeSubImprove:
		if !m.ai.Enabled {
			return "Turn on AI Assist in Config first."
		}
		if !m.workDone() {
			return "Add at least one project under Your work first (tab back)."
		}
		if m.improve.generating {
			return "Generating… this can take a minute."
		}
		if m.improveDone() {
			return "Done · files are in ~/.nexus/resumes/ · press g to regenerate"
		}
		return "Pick formats · optional target role (t) · press g to generate"
	default:
		return "tab moves to the next step"
	}
}

func (m ResumeHubModel) syncImproveContext() ResumeHubModel {
	var profile *resume.Profile
	if m.analyze.hasResult {
		profile = m.analyze.result.Profile
	}
	path := m.resumePath
	if path == "" {
		path = m.analyze.path
	}
	projects := m.work.projects
	if projects == nil {
		projects = []workcontext.Project{}
	}
	m.improve.SetContext(m.ai, path, profile, projects)
	return m
}

func (m ResumeHubModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Reserve rows for step strip + next-action banner.
		innerH := msg.Height - 5
		if innerH < 5 {
			innerH = 5
		}
		inner := tea.WindowSizeMsg{Width: msg.Width, Height: innerH}
		var cmds []tea.Cmd
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.analyze.Update(inner)
		m.analyze = sub.(ResumeTabModel)
		cmds = append(cmds, cmd)
		sub, cmd = m.work.Update(inner)
		m.work = sub.(WorkTabModel)
		cmds = append(cmds, cmd)
		sub, cmd = m.improve.Update(inner)
		m.improve = sub.(ImproveTabModel)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		if !m.CapturesKeys() {
			switch msg.String() {
			case "[", "h":
				m = m.PrevSub()
				m = m.syncImproveContext()
				return m, nil
			case "]", "l":
				m = m.NextSub()
				m = m.syncImproveContext()
				return m, nil
			case "n":
				// Jump to the recommended next step (unless Work list uses n for new project).
				if m.sub == resumeSubWork && m.work.mode == workList {
					break // let Work handle "n" = new project
				}
				m.sub = m.recommendedSub()
				m = m.syncImproveContext()
				return m, nil
			}
		}
	}

	switch msg.(type) {
	case ResumeAnalysisStartMsg, ResumeAnalysisDoneMsg, resumeSpinnerTickMsg:
		sub, cmd := m.analyze.Update(msg)
		m.analyze = sub.(ResumeTabModel)
		return m, cmd
	case workLoadedMsg, workSavedMsg, workDeletedMsg:
		sub, cmd := m.work.Update(msg)
		m.work = sub.(WorkTabModel)
		m = m.syncImproveContext()
		return m, cmd
	case improveDoneMsg:
		sub, cmd := m.improve.Update(msg)
		m.improve = sub.(ImproveTabModel)
		return m, cmd
	}

	var cmd tea.Cmd
	var sub tea.Model
	switch m.sub {
	case resumeSubAnalyze:
		sub, cmd = m.analyze.Update(msg)
		m.analyze = sub.(ResumeTabModel)
	case resumeSubWork:
		sub, cmd = m.work.Update(msg)
		m.work = sub.(WorkTabModel)
	case resumeSubImprove:
		m = m.syncImproveContext()
		sub, cmd = m.improve.Update(msg)
		m.improve = sub.(ImproveTabModel)
	}
	return m, cmd
}

func (m ResumeHubModel) View() string {
	var b strings.Builder
	b.WriteString(m.renderGuide())
	b.WriteString("\n")
	switch m.sub {
	case resumeSubAnalyze:
		b.WriteString(m.analyze.View())
	case resumeSubWork:
		b.WriteString(m.work.View())
	case resumeSubImprove:
		m = m.syncImproveContext()
		b.WriteString(m.improve.View())
	}
	return b.String()
}

func (m ResumeHubModel) renderGuide() string {
	var b strings.Builder
	title := labelStyle.Render("BUILD A STRONGER RESUME")
	b.WriteString("  " + title + "\n")
	b.WriteString("  " + mutedStyle.Render("Three steps. Finish them in order.") + "\n")
	b.WriteString("  " + m.renderSteps() + "\n")

	nextStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange))
	b.WriteString("  " + nextStyle.Render("→ "+m.nextAction()) + "\n")
	return b.String()
}

func (m ResumeHubModel) renderSteps() string {
	done := [resumeSubCount]bool{
		m.analyzeDone(),
		m.workDone(),
		m.improveDone(),
	}
	parts := make([]string, 0, resumeSubCount*2)
	for i, label := range resumeSubLabels {
		num := fmt.Sprintf("%d", i+1)
		mark := "○"
		style := mutedStyle
		if done[i] {
			mark = "✓"
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
		}
		if i == m.sub {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
			if !done[i] {
				mark = "●"
			}
		}
		// Soft warning: work has some but < 4
		if i == resumeSubWork && m.workCount() > 0 && m.workCount() < 4 && i != m.sub {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange))
			mark = "…"
		}
		parts = append(parts, style.Render(fmt.Sprintf("%s %s %s", mark, num, label)))
		if i < resumeSubCount-1 {
			parts = append(parts, mutedStyle.Render("  →  "))
		}
	}
	return strings.Join(parts, "")
}

func (m ResumeHubModel) FooterHint() string {
	if m.CapturesKeys() {
		return "editing  •  esc done  •  ctrl+c quit"
	}
	switch m.sub {
	case resumeSubAnalyze:
		return "tab → next step  •  r re-analyze  •  ↑↓ scroll  •  ←→ main tabs"
	case resumeSubWork:
		return "n add project  •  tab → New resume  •  ←→ main tabs"
	case resumeSubImprove:
		return "g generate  •  space formats  •  tab cycles steps  •  ←→ main tabs"
	default:
		return "tab next step  •  ←→ main tabs  •  ctrl+c quit"
	}
}

// Expose analyze fields app still reads.
func (m ResumeHubModel) AnalyzePath() string { return m.analyze.path }
