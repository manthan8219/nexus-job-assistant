package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/workcontext"
)

type workMode int

const (
	workList workMode = iota
	workDetail
	workForm
)

const (
	wfName = iota
	wfRepo
	wfPeriod
	wfRole
	wfCount
)

type workLoadedMsg struct {
	projects []workcontext.Project
	err      error
}

type workSavedMsg struct {
	err        error
	stayInForm bool // quiet autosave while editing
}
type workDeletedMsg struct{ err error }

// WorkTabModel manages multi-repo work context for later resume use.
type WorkTabModel struct {
	width, height int
	mode          workMode
	projects      []workcontext.Project
	cursor        int
	selectedID    string
	err           string
	status        string
	loading       bool

	// detail
	detailVP viewport.Model
	detailOK bool

	// form
	inputs    [wfCount]textinput.Model
	summary   textarea.Model
	formFocus int // 0..wfCount-1 = inputs, wfCount = summary
	editID    string
	isNew     bool
}

func NewWorkTabModel() WorkTabModel {
	m := WorkTabModel{loading: true, mode: workList}
	ph := []string{"Payments API", "github.com/org/repo", "2024 – Present", "Backend Engineer"}
	for i := 0; i < wfCount; i++ {
		t := textinput.New()
		t.Prompt = ""
		t.Placeholder = ph[i]
		t.Width = 48
		t.CharLimit = 120
		m.inputs[i] = t
	}
	ta := textarea.New()
	ta.Placeholder = "Paste Claude's project summary here…\nUse - bullets for achievements — we'll pick them up."
	ta.SetWidth(56)
	ta.SetHeight(10)
	ta.ShowLineNumbers = false
	m.summary = ta
	return m
}

func loadWorkCmd() tea.Cmd {
	return func() tea.Msg {
		projects, err := workcontext.Load()
		return workLoadedMsg{projects: projects, err: err}
	}
}

func (m WorkTabModel) Init() tea.Cmd { return loadWorkCmd() }

// CapturesKeys reports whether the Work tab should keep global keys.
// True for both form (editing) and detail (viewing) so esc navigates back
// to the list instead of leaking to TAB MODE.
func (m WorkTabModel) CapturesKeys() bool { return m.mode == workForm || m.mode == workDetail }

func (m WorkTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		vw := msg.Width - 6
		if vw < 40 {
			vw = 40
		}
		vh := msg.Height - 8
		if vh < 5 {
			vh = 5
		}
		if !m.detailOK {
			m.detailVP = viewport.New(vw, vh)
			m.detailOK = true
		} else {
			m.detailVP.Width = vw
			m.detailVP.Height = vh
		}
		m.summary.SetWidth(min(60, vw))
		m.summary.SetHeight(min(12, max(6, vh-12)))
		if m.mode == workDetail {
			m.detailVP.SetContent(m.detailContent())
		}

	case workLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.err = ""
			m.projects = msg.projects
			if m.cursor >= len(m.projects) {
				m.cursor = max(0, len(m.projects)-1)
			}
		}

	case workSavedMsg:
		if msg.err != nil {
			// Quiet autosave skips incomplete drafts without flashing errors.
			if msg.stayInForm {
				return m, nil
			}
			m.err = msg.err.Error()
			return m, nil
		}
		m.status = "saved"
		m.err = ""
		if msg.stayInForm {
			// Keep editing; list refreshes when you leave the form.
			return m, nil
		}
		m.mode = workList
		m.loading = true
		return m, loadWorkCmd()

	case workDeletedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.status = "deleted"
		m.mode = workList
		m.selectedID = ""
		m.loading = true
		return m, loadWorkCmd()

	case tea.KeyMsg:
		m.status = ""
		switch m.mode {
		case workList:
			return m.updateList(msg)
		case workDetail:
			return m.updateDetail(msg)
		case workForm:
			return m.updateForm(msg)
		}
	}
	return m, nil
}

func (m WorkTabModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.projects)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = max(0, len(m.projects)-1)
	case "n":
		m.beginForm("", true)
		return m, textinput.Blink
	case "enter", " ":
		if len(m.projects) == 0 {
			return m, nil
		}
		m.selectedID = m.projects[m.cursor].ID
		m.mode = workDetail
		if m.detailOK {
			m.detailVP.SetContent(m.detailContent())
			m.detailVP.GotoTop()
		}
	case "e":
		if len(m.projects) == 0 {
			return m, nil
		}
		m.beginForm(m.projects[m.cursor].ID, false)
		return m, textinput.Blink
	case "d":
		if len(m.projects) == 0 {
			return m, nil
		}
		id := m.projects[m.cursor].ID
		return m, func() tea.Msg {
			return workDeletedMsg{err: workcontext.Delete(id)}
		}
	case "r":
		m.loading = true
		return m, loadWorkCmd()
	}
	return m, nil
}

func (m WorkTabModel) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "backspace":
		m.mode = workList
		return m, nil
	case "e":
		m.beginForm(m.selectedID, false)
		return m, textinput.Blink
	case "d":
		id := m.selectedID
		return m, func() tea.Msg {
			return workDeletedMsg{err: workcontext.Delete(id)}
		}
	default:
		var cmd tea.Cmd
		m.detailVP, cmd = m.detailVP.Update(msg)
		return m, cmd
	}
}

func (m WorkTabModel) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.blurForm()
		// Auto-save on leave when complete; incomplete drafts leave quietly.
		cmd := m.saveFormCmd(true)
		m.mode = workList
		m.loading = true
		return m, tea.Batch(cmd, loadWorkCmd())
	case "ctrl+s":
		// Kept for muscle memory; same as autosave commit.
		return m, m.saveFormCmd(false)
	case "tab":
		m.formFocus = (m.formFocus + 1) % (wfCount + 1)
		return m, m.focusForm()
	case "shift+tab":
		m.formFocus = (m.formFocus - 1 + wfCount + 1) % (wfCount + 1)
		return m, m.focusForm()
	case "up":
		if m.formFocus == wfCount {
			// stay in textarea unless at start — still allow leave
			m.formFocus = wfRole
			return m, m.focusForm()
		}
		if m.formFocus > 0 {
			m.formFocus--
			return m, m.focusForm()
		}
		return m, nil
	case "down":
		if m.formFocus < wfCount {
			m.formFocus++
			return m, m.focusForm()
		}
		return m, nil
	}

	var cmd tea.Cmd
	if m.formFocus == wfCount {
		m.summary, cmd = m.summary.Update(msg)
	} else {
		m.inputs[m.formFocus], cmd = m.inputs[m.formFocus].Update(msg)
	}
	// Quiet autosave as you type once name + summary are present.
	return m, tea.Batch(cmd, m.saveFormCmd(true))
}

func (m *WorkTabModel) beginForm(id string, isNew bool) {
	m.mode = workForm
	m.isNew = isNew
	m.editID = id
	m.err = ""
	m.formFocus = wfName

	for i := range m.inputs {
		m.inputs[i].SetValue("")
		m.inputs[i].Blur()
	}
	m.summary.SetValue("")
	m.summary.Blur()

	if !isNew && id != "" {
		if p, ok, _ := workcontext.Get(id); ok {
			m.inputs[wfName].SetValue(p.Name)
			m.inputs[wfRepo].SetValue(p.Repo)
			m.inputs[wfPeriod].SetValue(p.Period)
			m.inputs[wfRole].SetValue(p.Role)
			m.summary.SetValue(p.Summary)
		}
	}
	m.inputs[wfName].Focus()
}

func (m *WorkTabModel) blurForm() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	m.summary.Blur()
}

func (m *WorkTabModel) focusForm() tea.Cmd {
	m.blurForm()
	if m.formFocus == wfCount {
		return m.summary.Focus()
	}
	return m.inputs[m.formFocus].Focus()
}

func (m WorkTabModel) saveFormCmd(stayInForm bool) tea.Cmd {
	name := strings.TrimSpace(m.inputs[wfName].Value())
	summary := strings.TrimSpace(m.summary.Value())
	if name == "" {
		return func() tea.Msg {
			return workSavedMsg{err: fmt.Errorf("project name is required"), stayInForm: stayInForm}
		}
	}
	if summary == "" {
		return func() tea.Msg {
			return workSavedMsg{err: fmt.Errorf("paste a Claude summary or notes"), stayInForm: stayInForm}
		}
	}
	p := workcontext.Project{
		ID:      m.editID,
		Name:    name,
		Repo:    strings.TrimSpace(m.inputs[wfRepo].Value()),
		Period:  strings.TrimSpace(m.inputs[wfPeriod].Value()),
		Role:    strings.TrimSpace(m.inputs[wfRole].Value()),
		Summary: summary,
		Source:  "claude",
		Bullets: workcontext.ExtractBullets(summary),
	}
	return func() tea.Msg {
		return workSavedMsg{err: workcontext.Upsert(p), stayInForm: stayInForm}
	}
}

func (m WorkTabModel) selected() (workcontext.Project, bool) {
	for _, p := range m.projects {
		if p.ID == m.selectedID {
			return p, true
		}
	}
	if m.cursor >= 0 && m.cursor < len(m.projects) {
		return m.projects[m.cursor], true
	}
	return workcontext.Project{}, false
}

func (m WorkTabModel) View() string {
	var b strings.Builder
	b.WriteString("\n  " + labelStyle.Render("STEP 2 — YOUR WORK") + "\n")
	b.WriteString("  " + mutedStyle.Render("Add 4–6 repos/projects (paste Claude summaries). This is what Improve will rewrite from.") + "\n")

	if m.err != "" {
		b.WriteString("\n  " + errorStyle.Render("✗ "+m.err) + "\n")
	}
	if m.status != "" {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("✓ "+m.status) + "\n")
	}

	switch m.mode {
	case workList:
		b.WriteString(m.viewList())
	case workDetail:
		b.WriteString(m.viewDetail())
	case workForm:
		b.WriteString(m.viewForm())
	}
	return b.String()
}

func (m WorkTabModel) viewList() string {
	var b strings.Builder
	b.WriteString("\n")
	if m.loading {
		b.WriteString(mutedStyle.Render("  Loading…") + "\n")
		return b.String()
	}
	if len(m.projects) == 0 {
		b.WriteString(mutedStyle.Render("  Empty — this is the missing piece for a stronger resume.") + "\n")
		b.WriteString(mutedStyle.Render("  1. Press n") + "\n")
		b.WriteString(mutedStyle.Render("  2. Name the project / repo") + "\n")
		b.WriteString(mutedStyle.Render("  3. Paste what Claude wrote about that work") + "\n")
		b.WriteString(mutedStyle.Render("  4. edits auto-save — repeat until you have 4–6") + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("  → Press n to add your first project") + "\n")
		return b.String()
	}

	b.WriteString(mutedStyle.Render(fmt.Sprintf("  %d projects", len(m.projects))) + "\n\n")
	for i, p := range m.projects {
		active := i == m.cursor
		prefix := "  "
		nameStyle := primaryStyle
		if active {
			prefix = "▶ "
			nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
		}
		meta := []string{}
		if p.Role != "" {
			meta = append(meta, p.Role)
		}
		if p.Period != "" {
			meta = append(meta, p.Period)
		}
		if p.Repo != "" {
			meta = append(meta, p.Repo)
		}
		b.WriteString(prefix + nameStyle.Render(p.Name) + "\n")
		if len(meta) > 0 {
			b.WriteString("    " + mutedStyle.Render(strings.Join(meta, " · ")) + "\n")
		}
		preview := p.ShortSummary(72)
		if preview != "" {
			b.WriteString("    " + mutedStyle.Render(preview) + "\n")
		}
		if n := len(p.Bullets); n > 0 {
			b.WriteString("    " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render(fmt.Sprintf("%d bullets captured", n)) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(mutedStyle.Render("  ↑↓ move  ·  enter open  ·  n new  ·  e edit  ·  d delete  ·  r reload") + "\n")
	return b.String()
}

func (m WorkTabModel) viewDetail() string {
	var b strings.Builder
	b.WriteString("\n")
	if !m.detailOK {
		b.WriteString(m.detailContent())
	} else {
		panel := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorGrey)).
			Padding(0, 1).
			Width(m.detailVP.Width).
			Render(m.detailVP.View())
		b.WriteString("  " + panel + "\n")
	}
	b.WriteString("\n  " + mutedStyle.Render("esc back  ·  e edit  ·  d delete  ·  ↑↓ scroll") + "\n")
	return b.String()
}

func (m WorkTabModel) detailContent() string {
	p, ok := m.selected()
	if !ok {
		return mutedStyle.Render("Project not found.")
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true).Render(p.Name) + "\n")
	meta := []string{}
	if p.Role != "" {
		meta = append(meta, p.Role)
	}
	if p.Period != "" {
		meta = append(meta, p.Period)
	}
	if p.Repo != "" {
		meta = append(meta, p.Repo)
	}
	if len(meta) > 0 {
		b.WriteString(mutedStyle.Render(strings.Join(meta, " · ")) + "\n")
	}
	b.WriteString(mutedStyle.Render("source: "+p.Source) + "\n\n")

	b.WriteString(mutedStyle.Render("SUMMARY") + "\n")
	b.WriteString(primaryStyle.Render(wrapText(p.Summary, max(40, m.width-10))) + "\n\n")

	if len(p.Bullets) > 0 {
		b.WriteString(mutedStyle.Render("BULLETS") + "\n")
		for _, bullet := range p.Bullets {
			b.WriteString("  • " + primaryStyle.Render(bullet) + "\n")
		}
		b.WriteString("\n")
	}
	if len(p.Stack) > 0 {
		b.WriteString(mutedStyle.Render("STACK") + "\n  " + strings.Join(p.Stack, " · ") + "\n")
	}
	return b.String()
}

func (m WorkTabModel) viewForm() string {
	var b strings.Builder
	title := "New project"
	if !m.isNew {
		title = "Edit project"
	}
	b.WriteString("\n  " + mutedStyle.Render(strings.ToUpper(title)) + "\n")
	b.WriteString("  " + mutedStyle.Render("Paste what Claude wrote about this repo — keep 4–6 strong projects.") + "\n\n")

	labels := []string{"Name", "Repo", "Period", "Role"}
	for i := 0; i < wfCount; i++ {
		label := labels[i]
		if m.formFocus == i {
			label = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true).Render("▶ " + label)
		} else {
			label = mutedStyle.Render("  " + label)
		}
		b.WriteString(fmt.Sprintf("%s\n  %s\n\n", label, m.inputs[i].View()))
	}

	sumLabel := "Claude summary / notes"
	if m.formFocus == wfCount {
		sumLabel = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true).Render("▶ " + sumLabel)
	} else {
		sumLabel = mutedStyle.Render("  " + sumLabel)
	}
	b.WriteString(sumLabel + "\n" + m.summary.View() + "\n\n")
	b.WriteString(mutedStyle.Render("  tab fields  ·  auto-saves  ·  esc done") + "\n")
	return b.String()
}
