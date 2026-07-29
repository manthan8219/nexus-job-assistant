package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthanmanthan/nexus/internal/companies"
	"github.com/manthanmanthan/nexus/internal/config"
	"github.com/manthanmanthan/nexus/internal/engine"
	"github.com/manthanmanthan/nexus/internal/enrich"
	"github.com/manthanmanthan/nexus/internal/notifier"
	"github.com/manthanmanthan/nexus/internal/provider"
	"github.com/manthanmanthan/nexus/internal/resume"
	"github.com/manthanmanthan/nexus/internal/scraper"
	"github.com/manthanmanthan/nexus/internal/store"
	"github.com/manthanmanthan/nexus/internal/usage"
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
	SkipResumeCheck bool // --skip-resume-check: bypass resume validation
}

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
	profileComplete bool // gates access to non-Config tabs
	chromeNav       bool // Escape: ←→ switch main tabs; Enter: focus tab content
}

func NewAppModel(cfg *config.Config, st *store.Store, eng *engine.Engine, opts AppOptions) AppModel {
	dash := NewDashboardModel()
	form := NewFormModel(cfg, opts.SkipResumeCheck)
	complete := form.IsComplete()
	startTab := TabConfig
	if complete {
		startTab = TabDashboard
	}
	m := AppModel{
		activeTab:       startTab,
		dashboard:       dash,
		config:          form,
		resumeHub:       NewResumeHubModel(),
		history:         NewHistoryModel(),
		companiesTab:    NewCompaniesTabModel(),
		outreach:        NewOutreachHubModel(),
		contacts:        NewContactsTabModel(),
		logs:            NewLogsModel(),
		eng:             eng,
		st:              st,
		profileComplete: complete,
	}
	m.dashboard.providers = m.activeProviders()
	m = m.syncDashboardMission()
	m.outreach.SetConfig(m.config.toConfig())
	m.contacts.SetConfig(m.config.toConfig())
	m.contacts.SetStore(st)
	return m
}

// activeProviders returns the list of active provider names for display on the dashboard.
// Always-active providers come from the engine; key-based providers are added when a key is set.
// syncDashboardMission refreshes Mission Control fields from Config + readiness.
func (m AppModel) syncDashboardMission() AppModel {
	cfg := m.config.toConfig()
	path := strings.TrimSpace(m.config.inputs[fResumePath].Value())
	m.dashboard.resumePath = path
	m.dashboard.resumeReady = path != "" && (m.config.skipResumeCheck || (m.config.resumeAnalysisDone && m.config.resumeAnalysisResult.Valid))
	m.dashboard.hasTitles = len(m.config.jobTitleTags) > 0
	m.dashboard.aiOn = m.config.aiAssist
	m.dashboard.hasConsent = m.config.applyConsent
	m.dashboard.maxPerDay = 25
	if cfg != nil && cfg.MaxAppsPerDay > 0 {
		m.dashboard.maxPerDay = cfg.MaxAppsPerDay
	}
	m.dashboard.providers = m.activeProviders()
	return m
}

func (m AppModel) activeProviders() []string {
	var names []string
	if m.eng != nil {
		names = append(names, m.eng.ProviderNames()...)
	}
	if m.config.inputs[fLinkedInKey].Value() != "" {
		names = append(names, "LinkedIn")
	}
	if m.config.inputs[fIndeedKey].Value() != "" {
		names = append(names, "Indeed")
	}
	return names
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.loadStats(), m.loadHistory(), m.config.InitCmd(), m.resumeHub.Init(), m.companiesTab.Init(), m.outreach.Init())
}

// ── Update ───────────────────────────────────────────────────────────────────

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		bodyH := m.contentAreaHeight()
		m.dashboard.width, m.dashboard.height = msg.Width, bodyH
		m.history.width, m.history.height = msg.Width, bodyH
		m.companiesTab.width, m.companiesTab.height = msg.Width, bodyH
		m.logs.width, m.logs.height = msg.Width, bodyH
		// Children size to the body area so app chrome always fits.
		inner := tea.WindowSizeMsg{Width: msg.Width, Height: bodyH}
		var cmds []tea.Cmd
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.logs.Update(inner)
		m.logs = sub.(LogsModel)
		cmds = append(cmds, cmd)
		sub, cmd = m.resumeHub.Update(inner)
		m.resumeHub = sub.(ResumeHubModel)
		cmds = append(cmds, cmd)
		sub, cmd = m.history.Update(inner)
		m.history = sub.(HistoryModel)
		cmds = append(cmds, cmd)
		sub, cmd = m.companiesTab.Update(inner)
		m.companiesTab = sub.(CompaniesTabModel)
		cmds = append(cmds, cmd)
		sub, cmd = m.outreach.Update(inner)
		m.outreach = sub.(OutreachHubModel)
		cmds = append(cmds, cmd)
		sub, cmd = m.contacts.Update(inner)
		m.contacts = sub.(ContactsTabModel)
		cmds = append(cmds, cmd)
		sub, cmd = m.config.Update(inner)
		m.config = sub.(FormModel)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	// ── Test notification ───────────────────────────────────────────────
	case TestNotifyMsg:
		if msg.Cfg != nil {
			return m, sendTestNotifyCmd(msg.Cfg)
		}
		return m, nil

	// ── Notify test result ───────────────────────────────────────────────
	case testNotifyResultMsg:
		if msg.err != nil {
			m.config.err = msg.err
			m.config.notifyBanner = ""
			m.logs.lines = append(m.logs.lines, fmt.Sprintf("[notify test] error: %v", msg.err))
		} else {
			m.config.err = nil
			m.config.notifyBanner = "✓ Test notification sent — check Telegram/Discord"
			m.logs.lines = append(m.logs.lines, "[notify test] ✓ test message sent successfully")
		}
		return m, nil

	// ── Config saved (partial or complete) ───────────────────────────────
	case SavedMsg:
		if msg.Cfg != nil && m.eng != nil {
			*m.eng.Cfg() = *msg.Cfg
			m.eng.RebuildNotifier(msg.Cfg)
		}
		m.dashboard.providers = m.activeProviders()
		if msg.Cfg != nil {
			m.outreach.SetConfig(msg.Cfg)
		} else {
			m.outreach.SetConfig(m.config.toConfig())
		}
		return m, nil

	// ── Profile completion ────────────────────────────────────────────────
	case ProfileCompleteMsg:
		if msg.Cfg != nil && m.eng != nil {
			*m.eng.Cfg() = *msg.Cfg
			m.eng.RebuildNotifier(msg.Cfg)
		}
		m.dashboard.providers = m.activeProviders()
		if msg.Cfg != nil {
			m.outreach.SetConfig(msg.Cfg)
		}
		wasComplete := m.profileComplete
		m.profileComplete = true
		if !wasComplete {
			newM, cmd := m.switchTab(TabDashboard)
			return newM, cmd
		}
		return m, nil

	// ── Engine lifecycle ──────────────────────────────────────────────────
	case StartEngineMsg:
		if m.eng == nil {
			return m, nil
		}
		// Keep engine config in sync with latest form values (limits, consent, blocklist).
		if cfg := m.config.toConfig(); cfg != nil {
			*m.eng.Cfg() = *cfg
		}
		m.dashboard.hasConsent = m.config.applyConsent
		if msg.AutoApply && !m.config.applyConsent {
			msg.AutoApply = false
			m.dashboard.autoApply = false
			m.dashboard.errMsg = "Auto Apply blocked — enable Apply Consent in Config"
		}
		m.eng.DryRun = msg.DryRun
		m.eng.AutoApply = msg.AutoApply
		m.eng.Reset()
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		m.dashboard.status = "running"
		m.dashboard.lastJob = ""
		m.dashboard.progress = make(map[string]string)
		m.dashboard.liveFeed = nil
		m.dashboard.foundCount = 0
		return m, tea.Batch(
			runEngineCmd(m.eng, ctx),
			waitForLog(m.eng.LogCh),
			waitForResult(m.eng.ResultCh),
			waitForProgress(m.eng.ProgressCh),
		)

	case ProviderProgressMsg:
		if m.dashboard.progress == nil {
			m.dashboard.progress = make(map[string]string)
		}
		m.dashboard.progress[msg.P.Provider] = msg.P.Status
		if msg.P.Status == "done" && msg.P.Count > 0 {
			m.dashboard.progress[msg.P.Provider] = fmt.Sprintf("done:%d", msg.P.Count)
		}
		return m, waitForProgress(m.eng.ProgressCh)

	case StopEngineMsg:
		if m.cancel != nil {
			m.cancel()
		}
		return m, nil

	case EngineDoneMsg:
		m.dashboard.status = "done"
		if msg.Err != nil {
			m.dashboard.status = "error"
			m.dashboard.errMsg = msg.Err.Error()
		}
		return m, tea.Batch(m.loadStats(), m.loadHistory())

	// ── Career page scraper ──────────────────────────────────────────────
	case ScraperScanMsg:
		cfg := m.config.toConfig()
		keywords := m.config.jobTitleTags
		logCh := m.eng.LogCh
		logCh <- "Career scraper: starting company scan…"
		m.activeTab = TabLogs
		return m, func() tea.Msg {
			return runScraperScanCmd(cfg, keywords, logCh)
		}

	case scraperScanProgressMsg:
		if msg.err != nil {
			m.eng.LogCh <- fmt.Sprintf("✗ %s — %v", msg.company, msg.err)
		} else {
			m.eng.LogCh <- fmt.Sprintf("✓ %s — %d found · %d new  %s", msg.company, msg.found, msg.saved, msg.careerURL)
		}
		return m, nil

	case scraperScanDoneMsg:
		if msg.err != nil {
			m.eng.LogCh <- fmt.Sprintf("Scraper scan error: %v", msg.err)
		} else {
			m.eng.LogCh <- "Career scraper: scan complete."
		}
		return m, m.loadStats()

	// ── Log streaming ─────────────────────────────────────────────────────
	case AppendLogMsg:
		var cmd tea.Cmd
		var sub tea.Model
		sub, cmd = m.logs.Update(msg)
		m.logs = sub.(LogsModel)
		// Re-queue to wait for next log line
		return m, tea.Batch(cmd, waitForLog(m.eng.LogCh))

	case RefreshUsageMsg:
		var cmd tea.Cmd
		var sub tea.Model
		sub, cmd = m.logs.Update(msg)
		m.logs = sub.(LogsModel)
		return m, cmd

	case usageTickMsg:
		if m.activeTab != TabLogs {
			return m, nil
		}
		return m, tea.Batch(m.loadUsage(), usageTickCmd())

	// ── Result streaming ──────────────────────────────────────────────────
	case EngineResultMsg:
		r := msg.Result
		label := fmt.Sprintf("%s @ %s", r.Job.Title, r.Job.Company)
		if r.Job.Provider != "" {
			label += " · " + r.Job.Provider
		}
		if r.FitScore > 0 {
			label += fmt.Sprintf(" · fit %d", r.FitScore)
		}
		switch r.Status {
		case "found":
			m.dashboard.foundCount++
			m.dashboard.pushLive(DashRecent{Label: label, Status: "found"})
			m.dashboard.lastJob = label
		case "dry-run":
			m.dashboard.pushLive(DashRecent{Label: label, Status: "dry-run"})
			m.dashboard.lastJob = label
		case "queued":
			m.dashboard.skipped++
			m.dashboard.pushLive(DashRecent{Label: label, Status: "queued"})
			m.dashboard.lastJob = label
		case "applied":
			m.dashboard.applied++
			m.dashboard.appliedToday++
			m.dashboard.pushLive(DashRecent{Label: label, Status: "applied"})
			m.dashboard.lastJob = label
		case "skipped":
			m.dashboard.skipped++
			m.dashboard.pushLive(DashRecent{Label: label, Status: "skipped"})
		case "failed":
			m.dashboard.failed++
			m.dashboard.pushLive(DashRecent{Label: label, Status: "failed"})
		}
		return m, waitForResult(m.eng.ResultCh)

	// ── Stats / History refresh ───────────────────────────────────────────
	case RefreshStatsMsg:
		m.dashboard.applied = msg.Applied
		m.dashboard.skipped = msg.Skipped
		m.dashboard.failed = msg.Failed
		m.dashboard.appliedToday = msg.AppliedToday
		m.dashboard.recent = msg.Recent
		m = m.syncDashboardMission()
		return m, nil

	case RefreshHistoryMsg:
		var cmd tea.Cmd
		var sub tea.Model
		sub, cmd = m.history.Update(msg)
		m.history = sub.(HistoryModel)
		m.outreach.SetJobs(m.history.apps)
		return m, cmd

	case HistoryEnrichRequestMsg:
		return m, m.enrichHistoryCmd(msg)

	case historyEnrichProgressMsg:
		if msg.Line != "" {
			var sub tea.Model
			var cmd tea.Cmd
			sub, cmd = m.logs.Update(AppendLogMsg{Line: msg.Line})
			m.logs = sub.(LogsModel)
			// Keep a short live status on History without blocking navigation.
			status := msg.Line
			if len(status) > 90 {
				status = status[:89] + "…"
			}
			m.history.enrichStatus = status
			return m, tea.Batch(cmd, msg.Next)
		}
		return m, msg.Next

	case historyEnrichDoneMsg:
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.history.Update(msg)
		m.history = sub.(HistoryModel)
		line := msg.Status
		if msg.Err != nil {
			line = "enrich failed: " + msg.Err.Error()
		} else if line == "" {
			line = fmt.Sprintf("enrich done — updated %d · failed %d", msg.Updated, msg.Failed)
		}
		if !strings.HasPrefix(line, "[enrich]") {
			line = "[enrich] " + line
		}
		var logSub tea.Model
		var logCmd tea.Cmd
		logSub, logCmd = m.logs.Update(AppendLogMsg{Line: line})
		m.logs = logSub.(LogsModel)
		cmds := []tea.Cmd{cmd, logCmd, m.loadHistory()}
		return m, tea.Batch(cmds...)

	// ── Resume analysis — always forward to config regardless of active tab ──
	case resumeSpinnerTickMsg:
		var cmds []tea.Cmd
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.config.Update(msg)
		m.config = sub.(FormModel)
		cmds = append(cmds, cmd)
		sub, cmd = m.resumeHub.Update(msg)
		m.resumeHub = sub.(ResumeHubModel)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)

	case ResumeReanalyzeRequestMsg:
		path := strings.TrimSpace(m.config.inputs[fResumePath].Value())
		if path == "" {
			path = m.resumeHub.AnalyzePath()
		}
		if path == "" || m.config.skipResumeCheck {
			return m, nil
		}
		m.config.resumeAnalysisGen++
		m.config.resumeAnalyzing = true
		m.config.resumeAnalysisDone = false
		gen := m.config.resumeAnalysisGen
		ai := m.config.aiOptions()
		return m, tea.Batch(
			resumeAnalysisStartCmd(path, gen),
			analyzeResumeCmd(path, gen, ai),
			resumeSpinnerTickCmd(),
		)

	case ResumeAnalysisStartMsg:
		var cmd tea.Cmd
		var sub tea.Model
		sub, cmd = m.resumeHub.Update(msg)
		m.resumeHub = sub.(ResumeHubModel)
		return m, cmd

	case ResumeAnalysisDoneMsg:
		var cmds []tea.Cmd
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.config.Update(msg)
		m.config = sub.(FormModel)
		cmds = append(cmds, cmd)
		sub, cmd = m.resumeHub.Update(msg)
		m.resumeHub = sub.(ResumeHubModel)
		cmds = append(cmds, cmd)
		// Re-evaluate profile gate now that we have an analysis result.
		m.profileComplete = m.config.IsComplete()
		if !m.profileComplete && m.activeTab != TabConfig && m.activeTab != TabResume {
			newM, switchCmd := m.switchTab(TabConfig)
			m = newM
			cmds = append(cmds, switchCmd)
		}
		return m, tea.Batch(cmds...)

	case workLoadedMsg, workSavedMsg, workDeletedMsg, improveDoneMsg:
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.resumeHub.Update(msg)
		m.resumeHub = sub.(ResumeHubModel)
		return m, cmd

	case OutreachSetupSaveMsg:
		m.config.ApplyOutreachSetup(msg.Consent, msg.ConsentAt, msg.MaxEmail, msg.MaxLI, msg.LIMode, msg.LICookie)
		m.outreach.consent = msg.Consent
		m.outreach.consentAt = msg.ConsentAt
		m.outreach.maxEmail = msg.MaxEmail
		m.outreach.maxLI = msg.MaxLI
		m.outreach.runMode = msg.LIMode
		m.outreach.liCookie = msg.LICookie
		m.outreach.SetConfig(m.config.toConfig())
		m.outreach.status = "Setup saved"
		m.outreach.errText = ""
		return m, m.config.saveCmd()

	case companiesLoadedMsg, companySavedMsg:
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.companiesTab.Update(msg)
		m.companiesTab = sub.(CompaniesTabModel)
		return m, cmd

	case outreachLoadedMsg, outreachErrMsg, outreachStatusMsg, outreachQueueBuiltMsg, outreachActionDoneMsg, outreachAutoTickMsg:
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.outreach.Update(msg)
		m.outreach = sub.(OutreachHubModel)
		return m, cmd
	}

	// ── Global key handling ───────────────────────────────────────────────
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit

		case "ctrl+z":
			return m, func() tea.Msg { return tea.SuspendMsg{} }

		case "esc":
			// Nested UIs consume Escape first (detail panes, typing, AI title confirm…).
			if m.consumesEscapeInternally() {
				break
			}
			if !m.chromeNav {
				m = m.enterChromeNav()
				return m, nil
			}
			return m, nil

		case "enter":
			if m.chromeNav {
				return m.exitChromeNav()
			}

		case "left":
			if m.chromeNav {
				newM, cmd := m.switchTab((m.activeTab - 1 + tabCount) % tabCount)
				return newM, cmd
			}
			// Content-focused: keep ← for internal use on editing tabs / scroll tabs.
			if m.activeTab == TabConfig || m.activeTab == TabResume || m.activeTab == TabOutreach || m.activeTab == TabCompanies {
				break
			}
			if m.activeTab == TabHistory && m.history.CapturesKeys() {
				break
			}
			if m.activeTab == TabLogs {
				break
			}
			newM, cmd := m.switchTab((m.activeTab - 1 + tabCount) % tabCount)
			return newM, cmd
		case "right":
			if m.chromeNav {
				newM, cmd := m.switchTab((m.activeTab + 1) % tabCount)
				return newM, cmd
			}
			if m.activeTab == TabConfig || m.activeTab == TabResume || m.activeTab == TabOutreach || m.activeTab == TabCompanies {
				break
			}
			if m.activeTab == TabHistory && m.history.CapturesKeys() {
				break
			}
			if m.activeTab == TabLogs {
				break
			}
			newM, cmd := m.switchTab((m.activeTab + 1) % tabCount)
			return newM, cmd
		}

		// Inside Resume hub, tab cycles Analyze/Work/Improve (not main tabs).
		if m.activeTab == TabResume && !m.resumeHub.CapturesKeys() {
			switch key.String() {
			case "tab", "]":
				m.resumeHub = m.resumeHub.NextSub()
				m.resumeHub.SetAIContext(m.config.aiOptions(), m.config.inputs[fResumePath].Value())
				return m, nil
			case "shift+tab", "[":
				m.resumeHub = m.resumeHub.PrevSub()
				m.resumeHub.SetAIContext(m.config.aiOptions(), m.config.inputs[fResumePath].Value())
				return m, nil
			}
		}

		// Inside Outreach hub, tab cycles Setup/Email/LinkedIn.
		if m.activeTab == TabOutreach && !m.outreach.CapturesKeys() {
			switch key.String() {
			case "tab", "]":
				m.outreach = m.outreach.NextSub()
				return m, nil
			case "shift+tab", "[":
				m.outreach = m.outreach.PrevSub()
				return m, nil
			}
		}

		// Chrome mode: tab/shift-tab always move main tabs (Escape first from Config).
		if m.chromeNav {
			switch key.String() {
			case "tab":
				newM, cmd := m.switchTab((m.activeTab + 1) % tabCount)
				return newM, cmd
			case "shift+tab":
				newM, cmd := m.switchTab((m.activeTab - 1 + tabCount) % tabCount)
				return newM, cmd
			case "1", "2", "3", "4", "5", "6", "7", "8":
				// handled below
			}
		}

		// tab/shift+tab switch main tabs on other screens
		if m.activeTab != TabConfig && m.activeTab != TabResume && m.activeTab != TabOutreach && !(m.activeTab == TabCompanies && m.companiesTab.CapturesKeys()) && !(m.activeTab == TabContacts && m.contacts.CapturesKeys()) {
			switch key.String() {
			case "tab":
				newM, cmd := m.switchTab((m.activeTab + 1) % tabCount)
				return newM, cmd
			case "shift+tab":
				newM, cmd := m.switchTab((m.activeTab - 1 + tabCount) % tabCount)
				return newM, cmd
			case "r":
				if m.activeTab == TabLogs {
					return m, m.loadUsage()
				}
				if m.activeTab == TabHistory {
					return m, m.loadHistory()
				}
			}
		} else if m.activeTab == TabHistory {
			if key.String() == "r" {
				return m, m.loadHistory()
			}
		}

		// Number keys — always in chrome mode; otherwise skip when Config/editing.
		if m.chromeNav || (m.activeTab != TabConfig && !(m.activeTab == TabResume && m.resumeHub.CapturesKeys()) && !(m.activeTab == TabOutreach && m.outreach.CapturesKeys()) && !(m.activeTab == TabCompanies && m.companiesTab.CapturesKeys())) {
			switch key.String() {
			case "1":
				newM, cmd := m.switchTab(TabDashboard)
				return newM, cmd
			case "2":
				newM, cmd := m.switchTab(TabConfig)
				return newM, cmd
			case "3":
				newM, cmd := m.switchTab(TabResume)
				return newM, cmd
			case "4":
				newM, cmd := m.switchTab(TabHistory)
				return newM, cmd
			case "5":
				newM, cmd := m.switchTab(TabCompanies)
				return newM, cmd
			case "6":
				newM, cmd := m.switchTab(TabOutreach)
				return newM, cmd
			case "7":
				newM, cmd := m.switchTab(TabContacts)
				return newM, cmd
			case "8":
				newM, cmd := m.switchTab(TabLogs)
				return newM, cmd
			}

			// Tab mode: don't leak keys into Config/editors.
			if m.chromeNav {
				return m, nil
			}
		}
	}

	return m.delegateUpdate(msg)
}

func (m AppModel) consumesEscapeInternally() bool {
	switch m.activeTab {
	case TabConfig:
		return m.config.ConsumesEscape()
	case TabHistory:
		return m.history.CapturesKeys()
	case TabResume:
		return m.resumeHub.CapturesKeys()
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
	// Block navigation away from Config until profile is complete
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
		// Always land in TAB MODE — Enter focuses fields. Auto-focus felt like
		// being trapped inside Config whenever you switched to it.
		m.config.loadResumeLibrary()
		m.config = m.config.BlurAll()
		m.chromeNav = true
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
		// Don't SetConfig here — that would wipe unsaved Setup edits on every key.
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

// ── View ─────────────────────────────────────────────────────────────────────

func (m AppModel) View() string {
	header := m.renderChromeHeader()
	footer := m.renderChromeFooter()
	body := m.renderChromeBody()

	// Keep scrollable tabs ≤ terminal height. Extra lines push the Nexus tab
	// bar off-screen in the alt buffer (Resume/History symptom).
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
	default:
		return ""
	}
}

// contentAreaHeight is terminal rows left for the active tab body.
func (m AppModel) contentAreaHeight() int {
	if m.height <= 0 {
		return 20
	}
	// title + tabs + divider + footer ≈ 5 rows
	h := m.height - 5
	if h < 5 {
		return 5
	}
	return h
}

// ── Tea commands ──────────────────────────────────────────────────────────────

// runScraperScanCmd crawls all companies without a known ATS and saves found jobs.
// Returns scraperScanDoneMsg when finished; logs progress to logCh as it goes.
func runScraperScanCmd(cfg *config.Config, keywords []string, logCh chan string) tea.Msg {
	db, err := companies.OpenDefault()
	if err != nil {
		return scraperScanDoneMsg{err: fmt.Errorf("open companies db: %w", err)}
	}
	defer db.Close()

	st, err := store.Open()
	if err != nil {
		return scraperScanDoneMsg{err: fmt.Errorf("open store: %w", err)}
	}
	defer st.Close()

	ollamaURL := cfg.LocalLLMURL
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	err = scraper.CrawlCompanies(context.Background(), db, st, keywords, func(r scraper.CrawlResult) {
		if r.Err != nil {
			logCh <- fmt.Sprintf("  ✗ %-30s %v", r.Company, r.Err)
		} else {
			logCh <- fmt.Sprintf("  ✓ %-30s %d found · %d new  %s", r.Company, r.JobsFound, r.JobsSaved, r.CareerURL)
		}
	})
	return scraperScanDoneMsg{err: err}
}

// runEngineCmd runs the engine and returns EngineDoneMsg when finished.
func runEngineCmd(eng *engine.Engine, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		err := eng.RunOnce(ctx)
		return EngineDoneMsg{Err: err}
	}
}

// waitForLog blocks until a log line arrives, then returns AppendLogMsg.
// When the channel is closed (engine done) it returns nil to stop the loop.
func waitForLog(ch chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil // channel closed — engine done
		}
		return AppendLogMsg{Line: line}
	}
}

// waitForResult blocks until a result arrives, then returns EngineResultMsg.
func waitForResult(ch chan engine.Result) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return nil
		}
		return EngineResultMsg{Result: result}
	}
}

// waitForProgress blocks until a provider progress event arrives, then returns ProviderProgressMsg.
func waitForProgress(ch chan engine.ProviderProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return ProviderProgressMsg{P: p}
	}
}

// loadStats queries the store for aggregate stats.
func (m AppModel) loadStats() tea.Cmd {
	if m.st == nil {
		return nil
	}
	return func() tea.Msg {
		a, s, f, _ := m.st.Stats()
		now := time.Now()
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		today, _ := m.st.CountAppliedSince(dayStart)
		apps, _ := m.st.List()
		recent := make([]DashRecent, 0, 3)
		for i, app := range apps {
			if i >= 3 {
				break
			}
			recent = append(recent, DashRecent{
				Label:  app.Role + " @ " + app.Company,
				Status: string(app.Status),
			})
		}
		return RefreshStatsMsg{
			Applied: a, Skipped: s, Failed: f,
			AppliedToday: today,
			Recent:       recent,
		}
	}
}

// enrichHistoryCmd re-fetches job descriptions for History backfill (u / U).
// Streams progress to the Logs tab; UI stays interactive while it runs.
func (m AppModel) enrichHistoryCmd(req HistoryEnrichRequestMsg) tea.Cmd {
	st := m.st
	cfg := m.config.toConfig()
	events := make(chan historyEnrichDoneOrProgress, 64)
	go runHistoryEnrich(events, st, cfg, req)
	return listenHistoryEnrich(events)
}

// historyEnrichDoneOrProgress is an internal wire type for the enrich goroutine.
type historyEnrichDoneOrProgress struct {
	Line   string
	Done   bool
	Result historyEnrichDoneMsg
}

func listenHistoryEnrich(events <-chan historyEnrichDoneOrProgress) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return historyEnrichDoneMsg{Err: fmt.Errorf("enrich stopped")}
		}
		if ev.Done {
			return ev.Result
		}
		return historyEnrichProgressMsg{
			Line: ev.Line,
			Next: listenHistoryEnrich(events),
		}
	}
}

func runHistoryEnrich(events chan<- historyEnrichDoneOrProgress, st *store.Store, cfg *config.Config, req HistoryEnrichRequestMsg) {
	defer close(events)
	emit := func(line string) {
		events <- historyEnrichDoneOrProgress{Line: line}
	}
	finish := func(r historyEnrichDoneMsg) {
		events <- historyEnrichDoneOrProgress{Done: true, Result: r}
	}

	if st == nil {
		finish(historyEnrichDoneMsg{Err: fmt.Errorf("store unavailable")})
		return
	}

	var apps []store.Application
	if req.All {
		var err error
		apps, err = st.ListMissingDescription()
		if err != nil {
			finish(historyEnrichDoneMsg{Err: err})
			return
		}
		if len(apps) == 0 {
			emit("[enrich] no jobs with empty descriptions")
			finish(historyEnrichDoneMsg{Status: "no jobs with empty descriptions"})
			return
		}
		emit(fmt.Sprintf("[enrich] backfill starting — %d job(s) with empty descriptions", len(apps)))
	} else {
		apps = []store.Application{req.App}
		emit(fmt.Sprintf("[enrich] refresh starting — %s @ %s", req.App.Role, req.App.Company))
	}

	ai := resume.AIOptions{}
	resumeText := ""
	if cfg != nil {
		ai = resume.AIOptions{
			Enabled:      cfg.AIAssist,
			Provider:     cfg.AIProvider,
			LocalURL:     cfg.LocalLLMURL,
			LocalModel:   cfg.LocalLLMModel,
			OpenAIKey:    cfg.OpenAIKey,
			AnthropicKey: cfg.AnthropicKey,
		}
		if ai.Enabled && strings.TrimSpace(cfg.ResumePath) != "" {
			if text, err := resume.ExtractText(cfg.ResumePath); err == nil {
				resumeText = text
				emit("[enrich] AI fit scoring enabled (resume loaded)")
			} else {
				emit(fmt.Sprintf("[enrich] AI on but resume unavailable: %v — descriptions only", err))
			}
		} else if ai.Enabled {
			emit("[enrich] AI on but no resume path — descriptions only")
		} else {
			emit("[enrich] AI Assist off — fetching descriptions only")
		}
	}

	updated, failed := 0, 0
	var lastErr error
	for i, app := range apps {
		label := fmt.Sprintf("%s @ %s", app.Role, app.Company)
		emit(fmt.Sprintf("[enrich] (%d/%d) fetch description: %s (%s) …", i+1, len(apps), label, app.Provider))
		timeout := 25 * time.Second
		if app.Provider == "linkedin" || app.Provider == "careerscraper" {
			timeout = 60 * time.Second // browser launch + page load + Show more click
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		desc, err := enrich.FetchDescription(ctx, app.Provider, app.URL)
		cancel()
		if err != nil {
			failed++
			lastErr = err
			emit(fmt.Sprintf("[enrich] ✗ fetch failed: %s — %v", label, err))
			continue
		}
		emit(fmt.Sprintf("[enrich] ✓ description (%d chars): %s", len(desc), label))

		fitScore, fitSummary := app.FitScore, app.FitSummary
		if ai.Enabled && strings.TrimSpace(resumeText) != "" {
			emit(fmt.Sprintf("[enrich] scoring fit vs resume: %s …", label))
			job := provider.Job{
				Title:       app.Role,
				Company:     app.Company,
				Location:    app.Location,
				Remote:      app.Remote,
				URL:         app.URL,
				Provider:    app.Provider,
				Description: desc,
			}
			fitCtx, fitCancel := context.WithTimeout(context.Background(), 55*time.Second)
			res, scoreErr := resume.ScoreJobFit(fitCtx, ai, resumeText, job)
			fitCancel()
			if scoreErr != nil {
				emit(fmt.Sprintf("[enrich] ~ fit skip: %s — %v", label, scoreErr))
			} else {
				fitScore, fitSummary = res.Score, res.Summary
				sum := strings.TrimSpace(fitSummary)
				if len(sum) > 80 {
					sum = sum[:79] + "…"
				}
				emit(fmt.Sprintf("[enrich] ✓ fit %d/100 — %s", fitScore, sum))
			}
		}
		if err := st.UpdateDescriptionFit(app.URL, desc, fitScore, fitSummary); err != nil {
			failed++
			lastErr = err
			emit(fmt.Sprintf("[enrich] ✗ save failed: %s — %v", label, err))
			continue
		}
		updated++
	}

	status := ""
	if !req.All && updated == 0 && failed == 1 && lastErr != nil {
		status = lastErr.Error()
	}
	finish(historyEnrichDoneMsg{Updated: updated, Failed: failed, Status: status})
}

func usageTickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return usageTickMsg{} })
}

// loadUsage collects ~/.nexus disk + process memory for the Logs USAGE panel.
func (m AppModel) loadUsage() tea.Cmd {
	st := m.st
	cfg := m.config.toConfig()
	return func() tea.Msg {
		dir, err := config.Dir()
		if err != nil {
			dir = ""
		}
		jobs := 0
		if st != nil {
			if apps, e := st.List(); e == nil {
				jobs = len(apps)
			}
		}
		mode := "off"
		if cfg != nil && cfg.AIAssist {
			mode = strings.TrimSpace(cfg.AIProvider)
			if mode == "" {
				mode = "api"
			}
		}
		return RefreshUsageMsg{Snap: usage.Collect(dir, jobs, mode)}
	}
}

// loadHistory queries the store for all applications.
func (m AppModel) loadHistory() tea.Cmd {
	if m.st == nil {
		return nil
	}
	return func() tea.Msg {
		apps, _ := m.st.List()
		return RefreshHistoryMsg{Apps: apps}
	}
}

type testNotifyResultMsg struct{ err error }

func sendTestNotifyCmd(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		discordURL, tgToken, tgChatID, channels := cfg.NotifyFields()
		ncfg := &notifier.NotifyConfig{
			DiscordWebhookURL: discordURL,
			TelegramBotToken:  tgToken,
			TelegramChatID:    tgChatID,
			EnabledChannels:   channels,
		}
		mn := notifier.FromConfig(ncfg)
		if len(mn) == 0 {
			return testNotifyResultMsg{err: fmt.Errorf(
				"no channels ready — fill credentials above and enable them under Notify on Apply",
			)}
		}
		ev := notifier.Event{
			Kind:    notifier.EventCustom,
			Title:   "⚡ Nexus — Test Notification",
			Message: "Your notification integration is working correctly.",
		}
		errs := mn.Send(context.Background(), ev)
		if len(errs) > 0 {
			parts := make([]string, len(errs))
			for i, e := range errs {
				parts[i] = e.Error()
			}
			return testNotifyResultMsg{err: fmt.Errorf("%s", strings.Join(parts, "; "))}
		}
		return testNotifyResultMsg{}
	}
}
