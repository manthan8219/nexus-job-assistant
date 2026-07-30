package ui

// Package ui — outreach_keys.go
// Key handlers for the Outreach tab: the main handleKey dispatcher, the Setup
// sub-tab (toggles, text edits), the Email/LinkedIn channel sub-tabs (cursor,
// build, fire, execute), and the contact-edit / confirm-action modals.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

func (m OutreachHubModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.ui == outRunning {
		if key == "esc" {
			m.autoGen++
			m.ui = outBrowse
			m.status = "Auto-run stopped"
			return m, nil
		}
		return m, nil
	}

	if m.ui == outEditContact {
		switch key {
		case "esc":
			m.ui = outBrowse
			m.contactInput.Blur()
			return m, nil
		case "enter":
			return m.commitContactEdit()
		default:
			var cmd tea.Cmd
			m.contactInput, cmd = m.contactInput.Update(msg)
			return m, cmd
		}
	}

	if m.sub == outreachSubSetup && m.setupFocusIsText() && m.setupInput.Focused() {
		switch key {
		case "esc":
			m.setupInput.Blur()
			return m, nil
		case "enter":
			m.applySetupInput()
			m.setupInput.Blur()
			return m, m.saveSetupCmd()
		default:
			var cmd tea.Cmd
			m.setupInput, cmd = m.setupInput.Update(msg)
			return m, cmd
		}
	}

	if m.ui == outConfirmAction {
		switch key {
		case "y", "Y", "enter":
			return m.executePending()
		case "n", "N", "esc":
			m.ui = outBrowse
			m.status = "Skipped"
			return m, nil
		}
		return m, nil
	}

	switch key {
	case "tab", "]":
		m = m.NextSub()
		if m.sub == outreachSubSent {
			m.logLoading = true
			return m, m.loadLogCmd()
		}
		return m, nil
	case "shift+tab", "[":
		m = m.PrevSub()
		if m.sub == outreachSubSent {
			m.logLoading = true
			return m, m.loadLogCmd()
		}
		return m, nil
	}

	if m.sub == outreachSubSetup {
		return m.handleSetupKey(key)
	}
	if m.sub == outreachSubSent {
		return m.handleSentKey(key)
	}
	return m.handleChannelKey(key)
}

func (m OutreachHubModel) handleSentKey(key string) (OutreachHubModel, tea.Cmd) {
	switch key {
	case "j", "down":
		if m.logCursor < len(m.logEntries)-1 {
			m.logCursor++
		}
	case "k", "up":
		if m.logCursor > 0 {
			m.logCursor--
		}
	case "g":
		m.logCursor = 0
	case "G":
		if len(m.logEntries) > 0 {
			m.logCursor = len(m.logEntries) - 1
		}
	case "r":
		m.logLoading = true
		return m, m.loadLogCmd()
	}
	return m, nil
}

func (m OutreachHubModel) handleSetupKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.setupFocus > 0 {
			m.setupFocus--
		}
	case "down", "j":
		if m.setupFocus < setupCount-1 {
			m.setupFocus++
		}
	case "left", "h", "right", "l", " ":
		return m.toggleSetupField()
	case "enter":
		if m.setupFocusIsText() {
			m.beginSetupEdit()
			return m, textinput.Blink
		}
		return m.toggleSetupField()
	case "ctrl+s":
		return m, m.saveSetupCmd()
	}
	return m, nil
}

func (m OutreachHubModel) toggleSetupField() (tea.Model, tea.Cmd) {
	switch m.setupFocus {
	case setupConsent:
		m.consent = !m.consent
		if m.consent && m.consentAt == "" {
			m.consentAt = time.Now().Format(time.RFC3339)
		}
		if !m.consent {
			m.consentAt = ""
		}
		return m, m.saveSetupCmd()
	case setupRunMode:
		m.runMode = outreach.CycleMode(m.runMode)
		return m, m.saveSetupCmd()
	case setupMaxEmail, setupMaxLI:
		m.beginSetupEdit()
		return m, textinput.Blink
	}
	return m, nil
}

func (m *OutreachHubModel) beginSetupEdit() {
	m.setupInput.Reset()
	m.setupInput.EchoMode = textinput.EchoNormal
	switch m.setupFocus {
	case setupMaxEmail:
		m.setupInput.SetValue(strconv.Itoa(m.maxEmail))
		m.setupInput.Placeholder = "emails per day"
	case setupMaxLI:
		m.setupInput.SetValue(strconv.Itoa(m.maxLI))
		m.setupInput.Placeholder = "LinkedIn opens per day"
	}
	m.setupInput.Focus()
	m.setupInput.CursorEnd()
}

func (m *OutreachHubModel) applySetupInput() {
	v := strings.TrimSpace(m.setupInput.Value())
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return
	}
	switch m.setupFocus {
	case setupMaxEmail:
		m.maxEmail = n
	case setupMaxLI:
		m.maxLI = n
	}
}

func (m OutreachHubModel) saveSetupCmd() tea.Cmd {
	return func() tea.Msg {
		return OutreachSetupSaveMsg{
			Consent:    m.consent,
			ConsentAt:  m.consentAt,
			MaxEmail:   m.maxEmail,
			MaxLI:      m.maxLI,
			LIMode:     m.runMode,
			LICookie:   m.liCookie,
			AutoQueue:  m.autoQueue,
			AICompose:  m.aiCompose,
			AIReview:   m.aiReview,
			GenModel:   m.genModel,
			CheckModel: m.checkModel,
			MinScore:   m.minScore,
			MaxRetries: m.maxRetries,
			SMTPVerify: m.smtpVerify,
		}
	}
}

func (m OutreachHubModel) handleChannelKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		if m.cursor < len(m.filtered())-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g":
		return m.buildQueue()
	case "enter":
		return m.fireNext(false)
	case "a":
		m.autoGen++
		gen := m.autoGen
		var model tea.Model
		var cmd tea.Cmd
		model, cmd = m.fireNext(true)
		m = model.(OutreachHubModel)
		cmds := []tea.Cmd{cmd}
		cmds = append(cmds, tea.Tick(1200*time.Millisecond, func(t time.Time) tea.Msg {
			return outreachAutoTickMsg{Channel: m.channelForSub(), Gen: gen}
		}))
		return m, tea.Batch(cmds...)
	case "e":
		if item, ok := m.selected(); ok {
			m.ui = outEditContact
			m.contactInput.SetValue(item.ContactEmail)
			return m, m.contactInput.Focus()
		}
	}
	return m, nil
}

func (m OutreachHubModel) buildQueue() (tea.Model, tea.Cmd) {
	m.building = true
	m.status = "Building automated queue…"
	m.errText = ""
	cfg := m.effectiveCfg()
	jobs := append([]store.Application(nil), m.jobs...)
	items := append([]outreach.Item(nil), m.items...)
	ch := m.channelForSub()
	return m, func() tea.Msg {
		if ch == outreach.ChannelLinkedIn {
			created, skipped, errs := outreach.BuildLinkedInQueue(cfg, jobs, items)
			return outreachQueueBuiltMsg{Channel: ch, Created: len(created), Skipped: skipped, Errs: errs}
		}
		created, skipped, errs := outreach.BuildEmailQueue(cfg, jobs, items, true)
		return outreachQueueBuiltMsg{Channel: ch, Created: len(created), Skipped: skipped, Errs: errs}
	}
}

func (m OutreachHubModel) fireNext(forceAuto bool) (tea.Model, tea.Cmd) {
	cfg := m.effectiveCfg()
	ch := m.channelForSub()
	if ch == outreach.ChannelEmail && !outreach.AllOK(outreach.EmailReady(cfg)) {
		m.errText = "Email not ready — fix Setup checklist (Gmail + Hunter)."
		m.ui = outBrowse
		m.autoGen++
		return m, nil
	}
	if ch == outreach.ChannelLinkedIn && !outreach.AllOK(outreach.LinkedInReady(cfg)) {
		m.errText = "LinkedIn not ready — fix Setup checklist."
		m.ui = outBrowse
		m.autoGen++
		return m, nil
	}
	it, ok := outreach.NextPending(m.items, ch)
	if !ok {
		m.status = "Queue empty — press g to build from Jobs."
		m.ui = outBrowse
		m.autoGen++
		return m, nil
	}
	mode := outreach.NormalizeMode(m.runMode)
	if !forceAuto && mode == outreach.ModeConfirm {
		m.pending = it
		m.ui = outConfirmAction
		m.status = fmt.Sprintf("Confirm: %s · %s", it.Company, it.Role)
		return m, nil
	}
	m.pending = it
	return m.executePending()
}

func (m OutreachHubModel) executePending() (tea.Model, tea.Cmd) {
	it := m.pending
	cfg := m.effectiveCfg()
	ch := it.Channel
	if ch == "" {
		ch = m.channelForSub()
	}
	if ch == outreach.ChannelLinkedIn && strings.TrimSpace(it.Body) != "" {
		_ = clipboard.WriteAll(it.Body)
	}
	markSent := outreach.NormalizeMode(m.runMode) == outreach.ModeAuto
	return m, func() tea.Msg {
		if ch == outreach.ChannelLinkedIn {
			if err := outreach.ExecuteLinkedIn(cfg, it, markSent); err != nil {
				return outreachActionDoneMsg{Err: err}
			}
			if markSent {
				return outreachActionDoneMsg{Text: "Browser opened + marked sent: " + it.Company}
			}
			return outreachActionDoneMsg{Text: "Browser opened for " + it.Company + " (message on clipboard)"}
		}
		if err := outreach.ExecuteEmail(cfg, it); err != nil {
			return outreachActionDoneMsg{Err: err}
		}
		return outreachActionDoneMsg{Text: "Email sent → " + it.ContactEmail}
	}
}

func (m OutreachHubModel) commitContactEdit() (tea.Model, tea.Cmd) {
	it, ok := m.selected()
	if !ok {
		m.ui = outBrowse
		m.contactInput.Blur()
		return m, nil
	}
	email := strings.TrimSpace(m.contactInput.Value())
	it.ContactEmail = email
	if email != "" {
		it.Status = outreach.StatusReady
	} else {
		it.Status = outreach.StatusDraft
	}
	for i := range m.items {
		if m.items[i].ID == it.ID {
			m.items[i] = it
			break
		}
	}
	m.ui = outBrowse
	m.contactInput.Blur()
	return m, func() tea.Msg {
		if err := outreach.Upsert(it); err != nil {
			return outreachErrMsg{err}
		}
		return outreachStatusMsg{Text: "To: " + email}
	}
}
