package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthanmanthan/nexus/internal/resume"
	"github.com/manthanmanthan/nexus/internal/workcontext"
)

type improveDoneMsg struct {
	out *resume.ImproveOutput
	err error
}

// ImproveTabModel generates rewritten resumes from analysis + work context.
type ImproveTabModel struct {
	width, height int

	formatIdx  int
	selected   map[resume.Format]bool
	target     textinput.Model
	focusTarget bool

	generating bool
	status     string
	err        string

	preview   viewport.Model
	previewOK bool
	lastOut   *resume.ImproveOutput

	// readiness inputs refreshed by hub before view/generate
	ai        resume.AIOptions
	resumePath string
	profile   *resume.Profile
	projects  []workcontext.Project
}

func NewImproveTabModel() ImproveTabModel {
	t := textinput.New()
	t.Placeholder = "e.g. Senior Backend Engineer"
	t.CharLimit = 80
	t.Width = 42
	t.Prompt = ""
	return ImproveTabModel{
		selected: map[resume.Format]bool{
			resume.FormatMarkdown: true,
			resume.FormatLaTeX:    true,
			resume.FormatPDF:      true,
		},
		target: t,
	}
}

func (m ImproveTabModel) Init() tea.Cmd { return nil }

// CapturesKeys when editing target role (keep global tab/numbers away).
func (m ImproveTabModel) CapturesKeys() bool {
	return m.focusTarget || m.generating
}

func (m *ImproveTabModel) SetContext(ai resume.AIOptions, resumePath string, profile *resume.Profile, projects []workcontext.Project) {
	m.ai = ai
	m.resumePath = resumePath
	m.profile = profile
	m.projects = projects
}

func (m ImproveTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		vw := msg.Width - 6
		if vw < 40 {
			vw = 40
		}
		vh := msg.Height - 18
		if vh < 5 {
			vh = 5
		}
		if !m.previewOK {
			m.preview = viewport.New(vw, vh)
			m.previewOK = true
		} else {
			m.preview.Width = vw
			m.preview.Height = vh
		}
		if m.lastOut != nil {
			m.preview.SetContent(m.lastOut.PreviewMD)
		}

	case improveDoneMsg:
		m.generating = false
		if msg.err != nil {
			m.err = msg.err.Error()
			m.status = ""
			return m, nil
		}
		m.err = ""
		m.lastOut = msg.out
		m.status = "saved to " + msg.out.Dir
		if msg.out.PDFNote != "" {
			m.status += " · " + msg.out.PDFNote
		}
		if m.previewOK {
			m.preview.SetContent(msg.out.PreviewMD)
			m.preview.GotoTop()
		}

	case tea.KeyMsg:
		if m.generating {
			return m, nil
		}
		if m.focusTarget {
			switch msg.String() {
			case "esc", "enter":
				m.target.Blur()
				m.focusTarget = false
				return m, nil
			}
			var cmd tea.Cmd
			m.target, cmd = m.target.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "up", "k":
			if m.formatIdx > 0 {
				m.formatIdx--
			}
		case "down", "j":
			if m.formatIdx < len(resume.SupportedFormats)-1 {
				m.formatIdx++
			}
		case " ", "x":
			f := resume.SupportedFormats[m.formatIdx]
			m.selected[f] = !m.selected[f]
		case "t":
			m.focusTarget = true
			return m, m.target.Focus()
		case "g", "enter":
			return m.StartGenerate()
		default:
			if m.previewOK && m.lastOut != nil {
				var cmd tea.Cmd
				m.preview, cmd = m.preview.Update(msg)
				return m, cmd
			}
		}
	}
	return m, nil
}

func (m ImproveTabModel) generateCmd() tea.Cmd {
	formats := m.selectedFormats()
	if len(formats) == 0 {
		return func() tea.Msg {
			return improveDoneMsg{err: fmt.Errorf("select at least one format")}
		}
	}
	soft := resume.ReadyChecklist(m.ai.Enabled, strings.TrimSpace(m.resumePath) != "", len(m.projects))
	for _, s := range soft {
		if strings.HasPrefix(s, "Enable") || strings.HasPrefix(s, "Set a resume") || strings.HasPrefix(s, "Add work") {
			return func() tea.Msg { return improveDoneMsg{err: fmt.Errorf("%s", s)} }
		}
	}
	ai := m.ai
	path := m.resumePath
	profile := m.profile
	projects := m.projects
	target := strings.TrimSpace(m.target.Value())
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		out, err := resume.GenerateImproved(ctx, ai, resume.ImproveInput{
			ResumePath: path,
			Profile:    profile,
			Projects:   projects,
			TargetRole: target,
			Formats:    formats,
		})
		return improveDoneMsg{out: out, err: err}
	}
}

func (m ImproveTabModel) StartGenerate() (ImproveTabModel, tea.Cmd) {
	m.generating = true
	m.err = ""
	m.status = "Rewriting resume from analysis + work context…"
	return m, m.generateCmd()
}

func (m ImproveTabModel) selectedFormats() []resume.Format {
	var out []resume.Format
	for _, f := range resume.SupportedFormats {
		if m.selected[f] {
			out = append(out, f)
		}
	}
	return out
}

func (m ImproveTabModel) View() string {
	var b strings.Builder
	b.WriteString("\n  " + labelStyle.Render("STEP 3 — NEW RESUME") + "\n")
	b.WriteString("  " + mutedStyle.Render("AI rewrites using Step 1 + Step 2. PDF is saved to ~/.nexus/resumes/ — pick it in Config.") + "\n\n")

	checks := resume.ReadyChecklist(m.ai.Enabled, strings.TrimSpace(m.resumePath) != "", len(m.projects))
	b.WriteString("  " + mutedStyle.Render("READY") + "\n")
	if len(checks) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("✓ AI on · resume set · "+fmt.Sprintf("%d projects", len(m.projects))) + "\n")
	} else {
		for _, c := range checks {
			style := mutedStyle
			if strings.HasPrefix(c, "Only") {
				style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange))
			} else {
				style = errorStyle
			}
			b.WriteString("  " + style.Render("• "+c) + "\n")
		}
	}

	b.WriteString("\n  " + mutedStyle.Render("FORMATS") + "  " + mutedStyle.Render("(space toggle)") + "\n")
	for i, f := range resume.SupportedFormats {
		mark := "[ ]"
		if m.selected[f] {
			mark = "[x]"
		}
		line := fmt.Sprintf("%s %s — %s", mark, f.Label(), resume.FormatHint(f))
		if i == m.formatIdx {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true).Render("▶ "+line) + "\n")
		} else {
			b.WriteString("  " + mutedStyle.Render("  "+line) + "\n")
		}
	}

	b.WriteString("\n  " + mutedStyle.Render("TARGET ROLE") + "  " + mutedStyle.Render("(t edit)") + "\n  ")
	if m.focusTarget {
		b.WriteString(m.target.View() + "\n")
	} else {
		v := strings.TrimSpace(m.target.Value())
		if v == "" {
			v = "(auto from profile)"
		}
		b.WriteString(primaryStyle.Render(v) + "\n")
	}

	if m.err != "" {
		b.WriteString("\n  " + errorStyle.Render("✗ "+m.err) + "\n")
	}
	if m.generating {
		b.WriteString("\n  " + mutedStyle.Render(m.status) + "\n")
	} else if m.status != "" {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("✓ "+m.status) + "\n")
	}

	if m.lastOut != nil {
		b.WriteString("\n  " + mutedStyle.Render("FILES") + "\n")
		for _, f := range resume.SupportedFormats {
			if p, ok := m.lastOut.Files[f]; ok {
				b.WriteString("  " + primaryStyle.Render(f.Label()+": "+filepath.Base(p)) + "\n")
			}
		}
		if m.previewOK {
			panel := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(colorGrey)).
				Padding(0, 1).
				Width(m.preview.Width).
				Render(m.preview.View())
			b.WriteString("\n  " + mutedStyle.Render("PREVIEW") + "\n  " + panel + "\n")
		}
	}

	b.WriteString("\n  " + mutedStyle.Render("↑↓ formats  ·  space toggle  ·  t target  ·  g generate") + "\n")
	return b.String()
}
