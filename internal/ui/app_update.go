package ui

// Package ui — app_update.go
// The AppModel Update dispatcher and its sub-handlers. Each message type is
// handled by a dedicated method, keeping the main Update loop slim.

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSizeMsg(msg)
	case AppendLogMsg:
		return m.handleAppendLogMsg(msg)
	case TestNotifyMsg:
		return m.handleTestNotifyMsg(msg)
	case testNotifyResultMsg:
		return m.handleTestNotifyResultMsg(msg)
	case SavedMsg:
		return m.handleSavedMsg(msg)
	case ProfileCompleteMsg:
		return m.handleProfileCompleteMsg(msg)
	case StartEngineMsg:
		return m.handleStartEngineMsg(msg)
	case StopEngineMsg:
		return m.handleStopEngineMsg(msg)
	case ScraperScanMsg:
		return m.handleScraperScanMsg(msg)
	case scraperScanProgressMsg:
		return m.handleScraperScanProgressMsg(msg)
	case scraperScanDoneMsg:
		return m.handleScraperScanDoneMsg(msg)
	case logsManualRefreshMsg:
		return m, m.loadUsage()
	case usageTickMsg:
		return m.handleUsageTickMsg(msg)
	case RefreshUsageMsg:
		return m.handleRefreshUsageMsg(msg)
	case EngineResultMsg:
		return m.handleEngineResultMsg(msg)
	case EngineDoneMsg:
		return m.handleEngineDoneMsg(msg)
	case ProviderProgressMsg:
		return m.handleProviderProgressMsg(msg)
	case RefreshStatsMsg:
		return m.handleRefreshStatsMsg(msg)
	case RefreshHistoryMsg:
		return m.handleRefreshHistoryMsg(msg)
	case HistoryEnrichRequestMsg:
		return m, m.enrichHistoryCmd(msg)
	case HistoryOutcomeRequestMsg:
		return m.handleHistoryOutcomeRequestMsg(msg)
	case historyOutcomeSavedMsg:
		return m.handleHistoryOutcomeSavedMsg(msg)
	case OutreachReplyCheckRequestMsg:
		return m, m.replyCheckCmd(false)
	case replyCheckDoneMsg:
		return m.handleReplyCheckDoneMsg(msg)
	case replyCheckTickMsg:
		return m, m.replyCheckCmd(true)
	case historyEnrichProgressMsg:
		return m.handleHistoryEnrichProgressMsg(msg)
	case historyEnrichDoneMsg:
		return m.handleHistoryEnrichDoneMsg(msg)
	case resumeSpinnerTickMsg:
		return m.handleResumeTickMsg(msg)
	case ResumeReanalyzeRequestMsg:
		return m.handleResumeReanalyzeRequestMsg(msg)
	case ResumeAnalysisStartMsg:
		return m.handleResumeAnalysisStartMsg(msg)
	case ResumeAnalysisDoneMsg:
		return m.handleResumeAnalysisDoneMsg(msg)
	case workLoadedMsg, workSavedMsg, workDeletedMsg, improveDoneMsg:
		return m.handleWorkMsgs(msg)
	case OutreachSetupSaveMsg:
		return m.handleOutreachSetupSaveMsg(msg)
	case companiesLoadedMsg, companySavedMsg, companyJobsLoadedMsg, companiesRefreshedMsg:
		return m.handleCompaniesMsgs(msg)
	case outreachLoadedMsg, outreachErrMsg, outreachStatusMsg, outreachQueueBuiltMsg, outreachActionDoneMsg, outreachAutoTickMsg:
		return m.handleOutreachMsgs(msg)
	case tea.KeyMsg:
		return m.handleAppKeyMsg(msg)
	}
	return m, nil
}

func (m AppModel) handleWindowSizeMsg(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width, m.height = msg.Width, msg.Height
	// Forward to every sub-model so viewports and layout helpers get the
	// correct dimensions immediately (not just on the next keypress).
	var cmds []tea.Cmd
	forward := func(sub tea.Model, cmd tea.Cmd) tea.Model { cmds = append(cmds, cmd); return sub }
	m.dashboard = forward(m.dashboard.Update(msg)).(DashboardModel)
	m.config = forward(m.config.Update(msg)).(FormModel)
	m.resumeHub = forward(m.resumeHub.Update(msg)).(ResumeHubModel)
	m.history = forward(m.history.Update(msg)).(HistoryModel)
	m.companiesTab = forward(m.companiesTab.Update(msg)).(CompaniesTabModel)
	m.outreach = forward(m.outreach.Update(msg)).(OutreachHubModel)
	m.contacts = forward(m.contacts.Update(msg)).(ContactsTabModel)
	m.logs = forward(m.logs.Update(msg)).(LogsModel)
	return m, tea.Batch(cmds...)
}

func (m AppModel) handleAppendLogMsg(msg AppendLogMsg) (tea.Model, tea.Cmd) {
	var sub tea.Model
	var cmd tea.Cmd
	sub, cmd = m.logs.Update(msg)
	m.logs = sub.(LogsModel)
	return m, cmd
}

func (m AppModel) handleEngineResultMsg(msg EngineResultMsg) (tea.Model, tea.Cmd) {
	r := msg.Result
	label := fmt.Sprintf("%s @ %s", r.Job.Title, r.Job.Company)
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
}

func (m AppModel) handleEngineDoneMsg(msg EngineDoneMsg) (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.dashboard.status = "stopped"
	if msg.Err != nil {
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.logs.Update(AppendLogMsg{Line: "Engine error: " + msg.Err.Error()})
		m.logs = sub.(LogsModel)
		return m, cmd
	}
	return m, nil
}

func (m AppModel) handleProviderProgressMsg(msg ProviderProgressMsg) (tea.Model, tea.Cmd) {
	if m.dashboard.progress == nil {
		m.dashboard.progress = make(map[string]string)
	}
	m.dashboard.progress[msg.P.Provider] = msg.P.Status
	if msg.P.Status == "done" && msg.P.Count > 0 {
		m.dashboard.progress[msg.P.Provider] = fmt.Sprintf("done:%d", msg.P.Count)
	}
	return m, waitForProgress(m.eng.ProgressCh)
}

func (m AppModel) handleTestNotifyResultMsg(msg testNotifyResultMsg) (tea.Model, tea.Cmd) {
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
}

func (m AppModel) handleTestNotifyMsg(msg TestNotifyMsg) (tea.Model, tea.Cmd) {
	if msg.Cfg != nil {
		return m, sendTestNotifyCmd(msg.Cfg)
	}
	return m, nil
}

func (m AppModel) handleProfileCompleteMsg(msg ProfileCompleteMsg) (tea.Model, tea.Cmd) {
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
}

func (m AppModel) handleStartEngineMsg(msg StartEngineMsg) (tea.Model, tea.Cmd) {
	if m.eng == nil {
		return m, nil
	}
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
}

func (m AppModel) handleStopEngineMsg(msg StopEngineMsg) (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	return m, nil
}

func (m AppModel) handleRefreshUsageMsg(msg RefreshUsageMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var sub tea.Model
	sub, cmd = m.logs.Update(msg)
	m.logs = sub.(LogsModel)
	return m, cmd
}

func (m AppModel) handleScraperScanProgressMsg(msg scraperScanProgressMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.eng.LogCh <- fmt.Sprintf("✗ %s — %v", msg.company, msg.err)
	} else {
		m.eng.LogCh <- fmt.Sprintf("✓ %s — %d found · %d new  %s", msg.company, msg.found, msg.saved, msg.careerURL)
	}
	return m, nil
}

func (m AppModel) handleScraperScanDoneMsg(msg scraperScanDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.eng.LogCh <- fmt.Sprintf("Scraper scan error: %v", msg.err)
	}
	return m, nil
}

func (m AppModel) handleSavedMsg(msg SavedMsg) (tea.Model, tea.Cmd) {
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
	m.outreach.SetJobs(m.history.apps)
	return m, nil
}

func (m AppModel) handleScraperScanMsg(msg ScraperScanMsg) (tea.Model, tea.Cmd) {
	cfg := m.config.toConfig()
	keywords := m.config.jobTitleTags
	logCh := m.eng.LogCh
	logCh <- "Career scraper: starting company scan…"
	m.activeTab = TabLogs
	return m, func() tea.Msg {
		return runScraperScanCmd(cfg, keywords, logCh)
	}
}

func (m AppModel) handleUsageTickMsg(msg usageTickMsg) (tea.Model, tea.Cmd) {
	if m.activeTab != TabLogs {
		return m, nil
	}
	return m, tea.Batch(m.loadUsage(), usageTickCmd())
}

func (m AppModel) handleRefreshStatsMsg(msg RefreshStatsMsg) (tea.Model, tea.Cmd) {
	m.dashboard.applied = msg.Applied
	m.dashboard.skipped = msg.Skipped
	m.dashboard.failed = msg.Failed
	m.dashboard.appliedToday = msg.AppliedToday
	m.dashboard.recent = msg.Recent
	m = m.syncDashboardMission()
	return m, nil
}

func (m AppModel) handleRefreshHistoryMsg(msg RefreshHistoryMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var sub tea.Model
	sub, cmd = m.history.Update(msg)
	m.history = sub.(HistoryModel)
	m.outreach.SetJobs(m.history.apps)
	return m, cmd
}

func (m AppModel) handleHistoryOutcomeRequestMsg(msg HistoryOutcomeRequestMsg) (tea.Model, tea.Cmd) {
	if m.st == nil {
		return m, nil
	}
	st := m.st
	return m, func() tea.Msg {
		if err := st.SetOutcome(msg.App.ID, msg.Outcome); err != nil {
			return AppendLogMsg{Line: "[pipeline] outcome failed: " + err.Error()}
		}
		label := string(msg.Outcome)
		if label == "" {
			label = "waiting"
		}
		return historyOutcomeSavedMsg{Line: fmt.Sprintf("[pipeline] %s @ %s → %s", msg.App.Role, msg.App.Company, label)}
	}
}

func (m AppModel) handleHistoryOutcomeSavedMsg(msg historyOutcomeSavedMsg) (tea.Model, tea.Cmd) {
	var logSub tea.Model
	var logCmd tea.Cmd
	logSub, logCmd = m.logs.Update(AppendLogMsg{Line: msg.Line})
	m.logs = logSub.(LogsModel)
	return m, tea.Batch(logCmd, m.loadHistory())
}

func (m AppModel) handleReplyCheckDoneMsg(msg replyCheckDoneMsg) (tea.Model, tea.Cmd) {
	if msg.Background && msg.Err == nil && msg.Replies == 0 && msg.Rejections == 0 {
		return m, nil
	}
	if msg.Err != nil {
		m.outreach.errText = "reply check: " + msg.Err.Error()
	} else {
		m.outreach.status = msg.Text
		m.outreach.errText = ""
	}
	var logSub tea.Model
	var logCmd tea.Cmd
	logSub, logCmd = m.logs.Update(AppendLogMsg{Line: "[replies] " + msg.Text})
	m.logs = logSub.(LogsModel)
	return m, tea.Batch(logCmd, m.loadHistory(), loadOutreachCmd())
}

func (m AppModel) handleHistoryEnrichProgressMsg(msg historyEnrichProgressMsg) (tea.Model, tea.Cmd) {
	if msg.Line != "" {
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.logs.Update(AppendLogMsg{Line: msg.Line})
		m.logs = sub.(LogsModel)
		status := msg.Line
		if len(status) > 90 {
			status = status[:89] + "…"
		}
		m.history.enrichStatus = status
		return m, tea.Batch(cmd, msg.Next)
	}
	return m, msg.Next
}

func (m AppModel) handleHistoryEnrichDoneMsg(msg historyEnrichDoneMsg) (tea.Model, tea.Cmd) {
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
}

func (m AppModel) handleResumeTickMsg(msg resumeSpinnerTickMsg) (tea.Model, tea.Cmd) {
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
}

func (m AppModel) handleResumeReanalyzeRequestMsg(msg ResumeReanalyzeRequestMsg) (tea.Model, tea.Cmd) {
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
}

func (m AppModel) handleResumeAnalysisStartMsg(msg ResumeAnalysisStartMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var sub tea.Model
	sub, cmd = m.resumeHub.Update(msg)
	m.resumeHub = sub.(ResumeHubModel)
	return m, cmd
}

func (m AppModel) handleResumeAnalysisDoneMsg(msg ResumeAnalysisDoneMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var sub tea.Model
	var cmd tea.Cmd
	sub, cmd = m.config.Update(msg)
	m.config = sub.(FormModel)
	cmds = append(cmds, cmd)
	sub, cmd = m.resumeHub.Update(msg)
	m.resumeHub = sub.(ResumeHubModel)
	cmds = append(cmds, cmd)
	m.profileComplete = m.config.IsComplete()
	if !m.profileComplete && m.activeTab != TabConfig && m.activeTab != TabResume {
		newM, switchCmd := m.switchTab(TabConfig)
		m = newM
		cmds = append(cmds, switchCmd)
	}
	return m, tea.Batch(cmds...)
}

func (m AppModel) handleWorkMsgs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var sub tea.Model
	var cmd tea.Cmd
	sub, cmd = m.resumeHub.Update(msg)
	m.resumeHub = sub.(ResumeHubModel)
	return m, cmd
}

func (m AppModel) handleOutreachSetupSaveMsg(msg OutreachSetupSaveMsg) (tea.Model, tea.Cmd) {
	m.config.ApplyOutreachSetup(msg)
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
}

func (m AppModel) handleCompaniesMsgs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var sub tea.Model
	var cmd tea.Cmd
	sub, cmd = m.companiesTab.Update(msg)
	m.companiesTab = sub.(CompaniesTabModel)
	return m, cmd
}

func (m AppModel) handleOutreachMsgs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var sub tea.Model
	var cmd tea.Cmd
	sub, cmd = m.outreach.Update(msg)
	m.outreach = sub.(OutreachHubModel)
	return m, cmd
}

func (m AppModel) handleAppKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	// Global quit / suspend — always handled regardless of mode or tab.
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if key == "ctrl+z" {
		return m, tea.Suspend
	}
	if m.chromeNav {
		switch key {
		case "enter":
			return m.exitChromeNav()
		case "esc":
			if m.activeTab == TabConfig || m.activeTab == TabHistory || m.activeTab == TabCompanies || m.activeTab == TabResume || m.activeTab == TabContacts {
				return m.exitChromeNav()
			}
			return m, nil
		case "left", "h", "shift+tab":
			next := (m.activeTab - 1 + tabCount) % tabCount
			if !m.profileComplete && next != TabConfig && next != TabResume {
				return m, nil
			}
			return m.switchTab(next)
		case "right", "l", "tab":
			next := (m.activeTab + 1) % tabCount
			if !m.profileComplete && next != TabConfig && next != TabResume {
				return m, nil
			}
			return m.switchTab(next)
		}
		for i := 1; i <= tabCount; i++ {
			if key == fmt.Sprintf("%d", i) {
				idx := i - 1
				if !m.profileComplete && idx != TabConfig && idx != TabResume {
					return m, nil
				}
				return m.switchTab(idx)
			}
		}
		// Unrecognized key: exit chrome nav and let the active tab handle it,
		// so content keys (d/a/enter on Dashboard, / on Companies, etc.) always work.
		m.chromeNav = false
		return m.delegateUpdate(msg)
	}
	if m.activeTab == TabConfig && m.config.ConsumesEscape() {
		if key == "esc" {
			var sub tea.Model
			var cmd tea.Cmd
			sub, cmd = m.config.Update(msg)
			m.config = sub.(FormModel)
			return m, cmd
		}
	}
	if m.activeTab == TabConfig && (key == "left" || key == "right") && m.config.CustomFieldActive() {
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.config.Update(msg)
		m.config = sub.(FormModel)
		return m, cmd
	}
	if m.activeTab == TabHistory && m.history.CapturesKeys() {
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.history.Update(msg)
		m.history = sub.(HistoryModel)
		return m, cmd
	}
	if m.activeTab == TabCompanies && m.companiesTab.CapturesKeys() {
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.companiesTab.Update(msg)
		m.companiesTab = sub.(CompaniesTabModel)
		return m, cmd
	}
	if m.activeTab == TabOutreach && m.outreach.CapturesKeys() {
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.outreach.Update(msg)
		m.outreach = sub.(OutreachHubModel)
		return m, cmd
	}
	if m.activeTab == TabResume && m.resumeHub.CapturesKeys() {
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.resumeHub.Update(msg)
		m.resumeHub = sub.(ResumeHubModel)
		return m, cmd
	}
	if m.activeTab == TabContacts && m.contacts.CapturesKeys() {
		var sub tea.Model
		var cmd tea.Cmd
		sub, cmd = m.contacts.Update(msg)
		m.contacts = sub.(ContactsTabModel)
		return m, cmd
	}
	if key == "esc" {
		return m.enterChromeNav(), nil
	}
	// Sub-menu tabs (Outreach, Resume) claim tab/shift+tab to cycle their
	// sub-sections — the CapturesKeys branches above already gave modal and
	// editing states first claim. From these tabs, main-tab switching stays
	// available via esc (tab mode), alt+N and the ctrl+letter shortcuts.
	if (m.activeTab == TabOutreach || m.activeTab == TabResume) && (key == "tab" || key == "shift+tab") {
		return m.delegateUpdate(msg)
	}
	if key == "tab" || key == "shift+tab" {
		dir := 1
		if key == "shift+tab" {
			dir = -1
		}
		next := (m.activeTab + dir + tabCount) % tabCount
		if !m.profileComplete && next != TabConfig && next != TabResume {
			return m, nil
		}
		return m.switchTab(next)
	}
	for i := 1; i <= tabCount; i++ {
		altKey := fmt.Sprintf("alt+%d", i)
		if key == altKey {
			idx := i - 1
			if !m.profileComplete && idx != TabConfig && idx != TabResume {
				return m, nil
			}
			return m.switchTab(idx)
		}
	}
	shortcuts := map[string]int{
		"ctrl+d": TabDashboard,
		"ctrl+r": TabResume,
		"ctrl+j": TabHistory,
		"ctrl+o": TabOutreach,
		"ctrl+l": TabLogs,
	}
	if tab, ok := shortcuts[key]; ok && tab >= 0 {
		return m.switchTab(tab)
	}
	return m.delegateUpdate(msg)
}
