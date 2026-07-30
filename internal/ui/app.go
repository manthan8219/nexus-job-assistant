package ui

// Package ui — app.go
// The AppModel: the root TUI state that owns every tab (Dashboard, Config,
// Resume, History, Companies, Outreach, Contacts, Logs), the chrome header/
// footer/body renderers, tab switching, and chrome navigation. The Update
// message dispatcher lives in app_update.go and async commands in
// app_commands.go.

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/engine"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// ── Tab constants ────────────────────────────────────────────────────────────

const (
	TabDashboard = iota
	TabConfig
	TabResume
	TabHistory
	TabCompanies
	TabOutreach
	TabContacts
	TabLogs
	tabCount
)

var tabLabels = [tabCount]string{"Dashboard", "Config", "Resume", "Jobs", "Companies", "Outreach", "Contacts", "Logs"}

// ── Engine messages ──────────────────────────────────────────────────────────

type AppendLogMsg struct{ Line string }
type EngineResultMsg struct{ Result engine.Result }
type EngineDoneMsg struct{ Err error }

// historyOutcomeSavedMsg confirms a pipeline outcome was persisted.
type historyOutcomeSavedMsg struct{ Line string }

// OutreachReplyCheckRequestMsg asks App to run an inbox reply check now.
type OutreachReplyCheckRequestMsg struct{}

// replyCheckDoneMsg carries the result of a reply-check pass.
type replyCheckDoneMsg struct {
	Text       string
	Err        error
	Replies    int
	Rejections int
	Background bool
}

// replyCheckTickMsg fires the background inbox watch.
type replyCheckTickMsg struct{}
type RefreshStatsMsg struct {
	Applied, Skipped, Failed int
	AppliedToday             int
	Recent                   []DashRecent
}
type ProviderProgressMsg struct{ P engine.ProviderProgress }

// ScraperScanMsg is sent from the Config form to trigger a career-page crawl.
type ScraperScanMsg struct{}

// scraperScanProgressMsg carries one company result back to the app.
type scraperScanProgressMsg struct {
	company   string
	careerURL string
	found     int
	saved     int
	err       error
}

// scraperScanDoneMsg fires when the full crawl finishes.
type scraperScanDoneMsg struct{ err error }

// ── App model ────────────────────────────────────────────────────────────────

// AppOptions holds optional flags passed from the CLI to the TUI.
type AppOptions struct {
	SkipResumeCheck bool
}

// AppModel is the root Bubble Tea model for the entire TUI application.
type AppModel struct {
	activeTab       int
	dashboard       DashboardModel
	config          FormModel
	resumeHub       ResumeHubModel
	history         HistoryModel
	companiesTab    CompaniesTabModel
	outreach        OutreachHubModel
	contacts        ContactsTabModel
	logs            LogsModel
	width           int
	height          int
	eng             *engine.Engine
	st              *store.Store
	cancel          context.CancelFunc
	profileComplete bool
	chromeNav       bool
}

func NewAppModel(cfg *config.Config, st *store.Store, eng *engine.Engine, opts AppOptions) AppModel {
	outreachHub := NewOutreachHubModel()
	outreachHub.st = st // wire store so Sent tab can load the audit log
	return AppModel{
		config:       NewFormModel(cfg, opts.SkipResumeCheck),
		dashboard:    NewDashboardModel(),
		resumeHub:    NewResumeHubModel(),
		history:      NewHistoryModel(),
		companiesTab: NewCompaniesTabModel(),
		outreach:     outreachHub,
		contacts:     NewContactsTabModel(),
		logs:         NewLogsModel(),
		eng:          eng,
		st:           st,
	}
}

func (m AppModel) syncDashboardMission() AppModel {
	m.dashboard.hasConsent = m.config.applyConsent
	m.dashboard.autoApply = m.config.applyConsent
	m.dashboard.providers = m.activeProviders()
	return m
}

func (m AppModel) activeProviders() []string {
	names := []string{}
	if m.eng != nil {
		names = m.eng.ProviderNames()
	}
	return names
}

func (m AppModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.config.InitCmd())
	cmds = append(cmds, m.dashboard.Init())
	cmds = append(cmds, m.resumeHub.Init())
	cmds = append(cmds, m.companiesTab.Init())
	cmds = append(cmds, m.outreach.Init())
	cmds = append(cmds, m.contacts.Init())
	cmds = append(cmds, m.logs.Init())
	cmds = append(cmds, scheduleReplyCheckTick())
	return tea.Batch(cmds...)
}

func (m AppModel) consumesEscapeInternally() bool {
	switch m.activeTab {
	case TabConfig:
		return m.config.ConsumesEscape()
	case TabHistory:
		return false
	case TabCompanies:
		return m.companiesTab.CapturesKeys()
	case TabOutreach:
		return m.outreach.CapturesKeys()
	case TabContacts:
		return m.contacts.CapturesKeys()
	default:
		return false
	}
}

func (m AppModel) enterChromeNav() AppModel {
	m.chromeNav = true
	if m.activeTab == TabConfig {
		m.config = m.config.BlurAll()
	}
	if m.activeTab == TabHistory && m.history.searching {
		m.history.searching = false
		m.history.search.Blur()
	}
	if m.activeTab == TabCompanies {
		m.companiesTab.search.Blur()
		m.companiesTab.country.Blur()
		m.companiesTab.searching = false
	}
	if m.activeTab == TabContacts {
		m.contacts.companyInput.Blur()
		m.contacts.domainInput.Blur()
	}
	return m
}

func (m AppModel) exitChromeNav() (AppModel, tea.Cmd) {
	m.chromeNav = false
	var cmds []tea.Cmd
	if m.activeTab == TabConfig {
		var cmd tea.Cmd
		m.config, cmd = m.config.FocusCurrent()
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m AppModel) switchTab(next int) (AppModel, tea.Cmd) {
	if !m.profileComplete && next != TabConfig && next != TabResume {
		return m, nil
	}
	if m.activeTab == TabConfig {
		m.config = m.config.BlurAll()
	}
	m.activeTab = next
	m = m.syncDashboardMission()
	var cmds []tea.Cmd
	if next == TabConfig {
		m.config.loadResumeLibrary()
		if m.chromeNav {
			// Already in chrome nav: blur form so user presses Enter to start editing.
			m.config = m.config.BlurAll()
		} else {
			// Programmatic switch (startup / profile guard): focus the form directly.
			var focusCmd tea.Cmd
			m.config, focusCmd = m.config.FocusCurrent()
			cmds = append(cmds, focusCmd)
		}
	}
	if next == TabDashboard {
		cmds = append(cmds, m.loadStats())
	}
	if next == TabHistory {
		cmds = append(cmds, m.loadHistory())
	}
	if next == TabResume {
		m.resumeHub.SetAIContext(m.config.aiOptions(), m.config.inputs[fResumePath].Value())
		cmds = append(cmds, m.resumeHub.Init())
	}
	if next == TabCompanies {
		m.companiesTab.detail = false
		m.companiesTab.detailLoading = false
		cmds = append(cmds, m.companiesTab.reload())
	}
	if next == TabOutreach {
		m.outreach.SetConfig(m.config.toConfig())
		m.outreach.SetJobs(m.history.apps)
		cmds = append(cmds, m.loadHistory(), m.outreach.Init())
	}
	if next == TabContacts {
		m.contacts.SetConfig(m.config.toConfig())
		m.contacts = m.contacts.Focus()
		cmds = append(cmds, m.contacts.Init())
	}
	if next == TabLogs {
		cmds = append(cmds, m.loadUsage(), usageTickCmd())
	}
	return m, tea.Batch(cmds...)
}

func (m AppModel) delegateUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.activeTab {
	case TabDashboard:
		var sub tea.Model
		sub, cmd = m.dashboard.Update(msg)
		m.dashboard = sub.(DashboardModel)
	case TabConfig:
		var sub tea.Model
		sub, cmd = m.config.Update(msg)
		m.config = sub.(FormModel)
	case TabResume:
		m.resumeHub.SetAIContext(m.config.aiOptions(), m.config.inputs[fResumePath].Value())
		var sub tea.Model
		sub, cmd = m.resumeHub.Update(msg)
		m.resumeHub = sub.(ResumeHubModel)
	case TabHistory:
		var sub tea.Model
		sub, cmd = m.history.Update(msg)
		m.history = sub.(HistoryModel)
	case TabCompanies:
		var sub tea.Model
		sub, cmd = m.companiesTab.Update(msg)
		m.companiesTab = sub.(CompaniesTabModel)
	case TabOutreach:
		m.outreach.SetJobs(m.history.apps)
		var sub tea.Model
		sub, cmd = m.outreach.Update(msg)
		m.outreach = sub.(OutreachHubModel)
	case TabContacts:
		var sub tea.Model
		sub, cmd = m.contacts.Update(msg)
		m.contacts = sub.(ContactsTabModel)
	case TabLogs:
		var sub tea.Model
		sub, cmd = m.logs.Update(msg)
		m.logs = sub.(LogsModel)
	}
	return m, cmd
}

func (m AppModel) View() string {
	header := m.renderChromeHeader()
	footer := m.renderChromeFooter()
	body := m.renderChromeBody()

	switch m.activeTab {
	case TabDashboard, TabConfig, TabResume, TabHistory, TabCompanies, TabOutreach, TabContacts, TabLogs:
		if m.height > 0 {
			avail := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
			if avail < 1 {
				avail = 1
			}
			body = lipgloss.NewStyle().MaxHeight(avail).Height(avail).Render(body)
		}
	}
	return header + body + footer
}

func (m AppModel) renderChromeHeader() string {
	var b strings.Builder
	b.WriteString(appTitleStyle.Render("⚡ Nexus"))
	if m.chromeNav {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true).Render("TAB MODE"))
	}
	b.WriteString("\n")

	tabs := make([]string, tabCount)
	for i, label := range tabLabels {
		if i == m.activeTab {
			tabs[i] = activeTabStyle.Render("▌ " + label)
		} else if !m.profileComplete && i != TabConfig && i != TabResume {
			tabs[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreyMid)).Faint(true).Padding(0, 2).Render("  " + label + " 🔒")
		} else {
			tabs[i] = inactiveTabStyle.Render("  " + label)
		}
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
	b.WriteString("\n")
	b.WriteString(divider(m.width))
	b.WriteString("\n")
	return b.String()
}

func (m AppModel) renderChromeFooter() string {
	if m.chromeNav {
		return "\n" + footerStyle.Render("TAB MODE  ·  ← → switch tabs  ·  enter focus this tab  ·  1-8 jump  ·  ctrl+c quit")
	}
	footer := "esc tab mode  ·  ← → / tab switch tabs  •  ctrl+z background  •  ctrl+c quit"
	if m.activeTab == TabDashboard {
		footer = "enter start/stop  •  d dry-run  •  a auto-apply  •  esc tab mode  •  ctrl+c quit"
	}
	if m.activeTab == TabConfig {
		footer = "↑↓/←→ edit fields  •  esc tab mode  •  ctrl+z background  •  ctrl+c quit"
	}
	if m.activeTab == TabResume {
		footer = m.resumeHub.FooterHint()
	}
	if m.activeTab == TabLogs {
		footer = "↑↓ scroll  •  r refresh usage  •  esc tab mode  •  ctrl+c quit"
	}
	if m.activeTab == TabHistory {
		footer = "/ search  •  ↑↓ navigate  •  enter details  •  esc tab mode  •  ctrl+c quit"
	}
	if m.activeTab == TabCompanies {
		footer = m.companiesTab.FooterHint()
	}
	if m.activeTab == TabOutreach {
		footer = m.outreach.FooterHint()
	}
	if m.activeTab == TabContacts {
		footer = m.contacts.FooterHint()
	}
	return "\n" + footerStyle.Render(footer)
}

func (m AppModel) renderChromeBody() string {
	switch m.activeTab {
	case TabDashboard:
		return m.dashboard.View()
	case TabConfig:
		return m.config.View()
	case TabResume:
		return m.resumeHub.View()
	case TabHistory:
		return m.history.View()
	case TabCompanies:
		return m.companiesTab.View()
	case TabOutreach:
		return m.outreach.View()
	case TabContacts:
		return m.contacts.View()
	case TabLogs:
		return m.logs.View()
	}
	return ""
}

func (m AppModel) contentAreaHeight() int {
	if m.height <= 0 {
		return 0
	}
	header := m.renderChromeHeader()
	footer := m.renderChromeFooter()
	avail := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if avail < 1 {
		avail = 1
	}
	return avail
}
