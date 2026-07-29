package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthanmanthan/nexus/internal/resume"
)

// ResumeReanalyzeRequestMsg asks AppModel to re-run AI resume analysis.
type ResumeReanalyzeRequestMsg struct{}

// ResumeTabModel shows AI resume analysis progress and results.
type ResumeTabModel struct {
	width, height int
	viewport      viewport.Model
	ready         bool

	analyzing    bool
	path         string
	result       resume.Result
	hasResult    bool
	status       string
	spinnerFrame int
}

func NewResumeTabModel() ResumeTabModel {
	return ResumeTabModel{}
}

func (m ResumeTabModel) Init() tea.Cmd { return nil }

func (m ResumeTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// height is already the app body area. Reserve local title + border + hint.
		vh := m.height - 6
		if vh < 5 {
			vh = 5
		}
		vw := m.width - 4
		if vw < 40 {
			vw = 40
		}
		if !m.ready {
			m.viewport = viewport.New(vw, vh)
			m.ready = true
		} else {
			m.viewport.Width = vw
			m.viewport.Height = vh
		}
		m.viewport.SetContent(m.content())

	case resumeSpinnerTickMsg:
		if m.analyzing {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		}

	case ResumeAnalysisStartMsg:
		m.analyzing = true
		m.path = msg.Path
		m.status = "Extracting resume text and building your career profile…"
		m.hasResult = false
		if m.ready {
			m.viewport.SetContent(m.content())
		}

	case ResumeAnalysisDoneMsg:
		m.analyzing = false
		m.result = msg.Result
		m.hasResult = true
		m.status = ""
		if msg.Path != "" {
			m.path = msg.Path
		}
		if m.ready {
			m.viewport.SetContent(m.content())
			m.viewport.GotoTop()
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			if !m.analyzing && m.path != "" {
				return m, func() tea.Msg { return ResumeReanalyzeRequestMsg{} }
			}
			return m, nil
		}
		if !m.ready {
			return m, nil
		}
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, cmd
}

func (m ResumeTabModel) View() string {
	var b strings.Builder
	b.WriteString("\n  " + labelStyle.Render("STEP 1 — REVIEW") + "\n")
	b.WriteString("  " + mutedStyle.Render("Read this first. Then tab to add the work your resume underplays.") + "\n")

	if m.analyzing {
		frame := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render(spinnerFrames[m.spinnerFrame%len(spinnerFrames)])
		b.WriteString("\n  " + frame + " " + primaryStyle.Render("AI is reading your resume…") + "\n")
		if m.path != "" {
			b.WriteString("  " + mutedStyle.Render(filepath.Base(m.path)) + "\n")
		}
		if m.status != "" {
			b.WriteString("  " + mutedStyle.Render(m.status) + "\n")
		}
		return b.String()
	}

	if !m.hasResult {
		b.WriteString("\n  " + mutedStyle.Render("Nothing here yet.") + "\n")
		b.WriteString("  " + mutedStyle.Render("Go to Config → turn on AI Assist → set your Resume Path. Analysis runs automatically.") + "\n")
		return b.String()
	}

	if !m.ready {
		return b.String() + "\n  " + m.content()
	}

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorGrey)).
		Padding(0, 1).
		Width(m.viewport.Width).
		Render(m.viewport.View())
	b.WriteString("\n  " + panel + "\n")
	b.WriteString("\n  " + mutedStyle.Render(m.scrollHint()) + "\n")
	return b.String()
}

func (m ResumeTabModel) scrollHint() string {
	parts := []string{"↑↓ scroll", "r re-analyze"}
	if m.ready && m.viewport.TotalLineCount() > m.viewport.Height {
		pct := int(m.viewport.ScrollPercent() * 100)
		parts = append(parts, fmt.Sprintf("%d%%", pct))
		if !m.viewport.AtBottom() {
			parts = append(parts, "more ↓")
		}
	}
	return strings.Join(parts, "  ·  ")
}

func (m ResumeTabModel) content() string {
	if !m.hasResult {
		return "No analysis yet."
	}
	r := m.result
	w := m.viewport.Width
	if w < 48 {
		w = 48
	}
	inner := w - 2

	var b strings.Builder

	if !r.Valid {
		b.WriteString(errorStyle.Render("✗ "+r.Err) + "\n")
		return b.String()
	}

	// Quiet chrome: filename + small ready badge
	b.WriteString(renderInsightChrome(m.path, r) + "\n\n")

	p := r.Profile
	if p == nil {
		b.WriteString(mutedStyle.Render("Basic file check only — enable AI Assist for a full career profile.") + "\n")
		return b.String()
	}
	if p.Error != "" && p.Summary == "" {
		b.WriteString(errorStyle.Render(p.Error) + "\n")
		b.WriteString(mutedStyle.Render("Tip: export as .docx or a text-based PDF and press r to retry.") + "\n")
		return b.String()
	}

	// ── First viewport: level + short summary + primary chart ───────────
	b.WriteString(renderLevelChip(p.ExperienceLevel, p.YearsEstimate) + "\n\n")

	if p.Summary != "" {
		b.WriteString(insightHeading("Overview"))
		b.WriteString("  " + primaryStyle.Render(wrapText(truncateSummary(p.Summary, 280), inner-2)) + "\n\n")
	}

	// Balanced critique — equal visual weight for praise and criticism
	good := p.WhatsGood
	if len(good) == 0 {
		good = p.Strengths
	}
	wrong := p.WhatsWrong
	if len(wrong) == 0 {
		wrong = p.Improvements // older caches without whats_wrong
	}
	if len(good) > 0 || len(wrong) > 0 {
		b.WriteString(renderBalanceCritique(good, wrong, inner))
		b.WriteString("\n")
	}

	// Primary chart: role fit only
	roles := p.RoleFit
	if len(roles) == 0 {
		roles = scoresFromNames(p.SuitableRoles, 6)
	}
	if len(roles) > 0 {
		b.WriteString(insightHeading("Best role fit"))
		b.WriteString(renderBarChart(roles, inner, colorOrange))
		b.WriteString("\n")
	}

	// Actionable fixes after critique
	if len(p.Improvements) > 0 {
		b.WriteString(renderImproveCallout(p.Improvements, inner))
		b.WriteString("\n")
	}

	// Secondary: compact ranked lists (not more bar charts)
	strengths := p.StrengthScores
	if len(strengths) == 0 {
		strengths = scoresFromNames(p.Strengths, 6)
	}
	if len(strengths) > 0 {
		b.WriteString(insightHeading("Strengths"))
		b.WriteString(renderRankedList(strengths, colorGreen))
		b.WriteString("\n")
	}

	skills := p.SkillScores
	if len(skills) == 0 {
		skills = scoresFromNames(p.Skills, 8)
	}
	if len(skills) > 0 {
		b.WriteString(insightHeading("Top skills"))
		b.WriteString(renderRankedList(skills, colorGreen))
		b.WriteString("\n")
	}

	if len(p.Industries) > 0 {
		b.WriteString(insightHeading("Industries"))
		b.WriteString("  " + renderChips(p.Industries) + "\n")
	}

	return b.String()
}

func renderInsightChrome(path string, r resume.Result) string {
	name := "resume"
	if path != "" {
		name = filepath.Base(path)
	}
	badge := mutedStyle.Render("ready")
	if r.Profile != nil && r.Profile.Summary != "" {
		badge = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("AI ready")
	}
	return "  " + primaryStyle.Render(name) + "  " + mutedStyle.Render("·") + "  " + badge
}

func renderLevelChip(level string, years int) string {
	level = strings.TrimSpace(level)
	if level == "" {
		level = "mid"
	}
	// Title-case first letter
	disp := strings.ToUpper(level[:1]) + strings.ToLower(level[1:])
	chip := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorGreen)).
		Bold(true).
		Render(disp)
	extra := ""
	if years > 0 {
		extra = mutedStyle.Render(fmt.Sprintf("  ·  %d years experience", years))
	}
	return "  " + chip + extra
}

func insightHeading(title string) string {
	return mutedStyle.Render("  "+strings.ToUpper(title)) + "\n"
}

func renderBalanceCritique(good, wrong []string, width int) string {
	var b strings.Builder
	goodHead := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	wrongHead := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true)
	goodMark := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	wrongMark := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange))

	b.WriteString(insightHeading("Honest take"))
	b.WriteString(goodHead.Render("  + What's strong") + "\n")
	maxG := len(good)
	if maxG > 5 {
		maxG = 5
	}
	for i := 0; i < maxG; i++ {
		b.WriteString("    " + goodMark.Render("▸") + " " + primaryStyle.Render(wrapText(good[i], width-6)) + "\n")
	}
	if maxG == 0 {
		b.WriteString("    " + mutedStyle.Render("—") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(wrongHead.Render("  − What needs work") + "\n")
	maxW := len(wrong)
	if maxW > 5 {
		maxW = 5
	}
	for i := 0; i < maxW; i++ {
		b.WriteString("    " + wrongMark.Render("▸") + " " + primaryStyle.Render(wrapText(wrong[i], width-6)) + "\n")
	}
	if maxW == 0 {
		b.WriteString("    " + mutedStyle.Render("—") + "\n")
	}
	return b.String()
}

func renderImproveCallout(tips []string, width int) string {
	var b strings.Builder
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true)
	b.WriteString(accent.Render("  → How to fix it") + "\n")
	max := len(tips)
	if max > 3 {
		max = 3
	}
	for i := 0; i < max; i++ {
		b.WriteString(fmt.Sprintf("    %s %s\n",
			mutedStyle.Render(fmt.Sprintf("%d.", i+1)),
			primaryStyle.Render(wrapText(tips[i], width-6)),
		))
	}
	return b.String()
}

func renderRankedList(items []resume.ScoredItem, scoreColor string) string {
	var b strings.Builder
	scoreStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(scoreColor))
	for i, it := range items {
		if i >= 8 {
			break
		}
		score := it.Score
		if score < 1 {
			score = 1
		}
		if score > 10 {
			score = 10
		}
		b.WriteString(fmt.Sprintf("  %s  %-28s  %s\n",
			mutedStyle.Render(fmt.Sprintf("%d.", i+1)),
			truncateLabelUI(it.Name, 28),
			scoreStyle.Render(fmt.Sprintf("%2d/10", score)),
		))
	}
	return b.String()
}

func renderBarChart(items []resume.ScoredItem, width int, barColor string) string {
	if len(items) == 0 {
		return ""
	}
	// Cap to top 5 so the first viewport stays tight.
	if len(items) > 5 {
		items = items[:5]
	}
	labelW := 20
	for _, it := range items {
		if len(it.Name) > labelW {
			labelW = min(26, len(it.Name))
		}
	}
	barW := width - 4 - labelW - 5
	if barW < 12 {
		barW = 12
	}

	var b strings.Builder
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(barColor))
	for _, it := range items {
		name := truncateLabelUI(it.Name, labelW)
		score := it.Score
		if score < 0 {
			score = 0
		}
		if score > 10 {
			score = 10
		}
		filled := int(float64(barW) * float64(score) / 10.0)
		if score > 0 && filled == 0 {
			filled = 1
		}
		// Color intensity: strong scores greener/amber, weak muted
		fill := barStyle.Render(strings.Repeat("█", filled))
		empty := mutedStyle.Render(strings.Repeat("░", barW-filled))
		b.WriteString(fmt.Sprintf("  %-*s %s %s\n",
			labelW, name,
			fill+empty,
			mutedStyle.Render(fmt.Sprintf("%2d", score)),
		))
	}
	return b.String()
}

func renderChips(items []string) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, mutedStyle.Render("· "+it))
	}
	return strings.Join(parts, "  ")
}

func scoresFromNames(names []string, n int) []resume.ScoredItem {
	out := make([]resume.ScoredItem, 0, n)
	for i, name := range names {
		if i >= n {
			break
		}
		score := 9 - i
		if score < 5 {
			score = 5
		}
		out = append(out, resume.ScoredItem{Name: name, Score: score})
	}
	return out
}

func truncateSummary(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	// Prefer breaking at sentence end within max.
	cut := s[:max]
	if i := strings.LastIndexAny(cut, ".!?"); i >= max/3 {
		return strings.TrimSpace(cut[:i+1])
	}
	if i := strings.LastIndex(cut, " "); i > 0 {
		return strings.TrimSpace(cut[:i]) + "…"
	}
	return cut + "…"
}

func truncateLabelUI(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
