package ui

// Package ui — outreach_hub.go
// The Outreach tab model: OutreachHubModel state, message types, the Update
// dispatch (setup, build, fire, auto-run), and helpers (config sync, sub-tab
// navigation, filtering). Key handlers are in outreach_keys.go and the views
// in outreach_view.go.

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

const (
	outreachSubSetup = iota
	outreachSubEmail
	outreachSubLinkedIn
	outreachSubSent
	outreachSubCount
)

var outreachSubLabels = [outreachSubCount]string{"Setup", "Email", "LinkedIn", "Sent"}

const (
	setupConsent = iota
	setupRunMode
	setupMaxEmail
	setupMaxLI
	setupAutoQueue
	setupAICompose
	setupAIReview
	setupGenModel
	setupCheckModel
	setupMinScore
	setupMaxRetries
	setupSMTPVerify
	setupCount
)

type outreachUIMode int

const (
	outBrowse        outreachUIMode = iota
	outConfirmAction                // y/n before sending/opening
	outEditContact
	outRunning
)

type outreachLoadedMsg struct{ Items []outreach.Item }
type outreachErrMsg struct{ Err error }
type outreachStatusMsg struct{ Text string }
type outreachQueueBuiltMsg struct {
	Channel outreach.Channel
	Created int
	Skipped int
	Errs    []string
}
type outreachActionDoneMsg struct {
	Text string
	Err  error
}
type outreachAutoTickMsg struct {
	Channel outreach.Channel
	Gen     int
}

type OutreachSetupSaveMsg struct {
	Consent    bool
	ConsentAt  string
	MaxEmail   int
	MaxLI      int
	LIMode     string
	LICookie   string
	AutoQueue  bool
	AICompose  bool
	AIReview   bool
	GenModel   string
	CheckModel string
	MinScore   int
	MaxRetries int
	SMTPVerify bool
}

type outreachWorkerMsg struct{ Line string }
type outreachLogLoadedMsg struct {
	Entries []store.OutreachLogEntry
	Err     error
}

type OutreachHubModel struct {
	width, height int
	sub           int
	ui            outreachUIMode

	cfg    *config.Config
	items  []outreach.Item
	jobs   []store.Application
	worker *outreach.Worker
	st     *store.Store

	cursor  int
	status  string
	errText string

	consent    bool
	consentAt  string
	maxEmail   int
	maxLI      int
	runMode    string
	liCookie   string
	setupFocus int
	setupInput textinput.Model

	autoQueue  bool
	aiCompose  bool
	aiReview   bool
	genModel   string
	checkModel string
	minScore   int
	maxRetries int
	smtpVerify bool

	logEntries []store.OutreachLogEntry
	logCursor  int
	logLoading bool

	contactInput textinput.Model
	pending      outreach.Item
	autoGen      int
	building     bool
}

func NewOutreachHubModel() OutreachHubModel {
	ti := textinput.New()
	ti.CharLimit = 200
	ci := textinput.New()
	ci.CharLimit = 120
	ci.Prompt = "> "
	return OutreachHubModel{setupInput: ti, contactInput: ci}
}

func (m OutreachHubModel) Init() tea.Cmd { return loadOutreachCmd() }

func loadOutreachCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := outreach.Load()
		if err != nil {
			return outreachErrMsg{err}
		}
		return outreachLoadedMsg{Items: items}
	}
}

func (m OutreachHubModel) loadLogCmd() tea.Cmd {
	if m.st == nil {
		return nil
	}
	st := m.st
	return func() tea.Msg {
		entries, err := st.ListOutreachLog(200)
		return outreachLogLoadedMsg{Entries: entries, Err: err}
	}
}

func (m *OutreachHubModel) SetConfig(cfg *config.Config) {
	m.cfg = cfg
	m.consent = cfg.OutreachConsent
	m.consentAt = cfg.OutreachConsentAt
	m.maxEmail = cfg.MaxEmailsPerDay
	m.maxLI = cfg.MaxLinkedInPerDay
	m.runMode = cfg.OutreachMode
	m.liCookie = cfg.LinkedInSessionCookie
	m.autoQueue = cfg.OutreachAutoQueue
	m.aiCompose = cfg.OutreachAICompose
	m.aiReview = cfg.OutreachAIReview
	m.genModel = cfg.OutreachGenModel
	m.checkModel = cfg.OutreachCheckModel
	m.minScore = cfg.OutreachMinScore
	m.maxRetries = cfg.OutreachMaxRetries
	m.smtpVerify = cfg.OutreachSMTPVerify
}

func (m *OutreachHubModel) SetJobs(apps []store.Application) { m.jobs = apps }

func (m OutreachHubModel) ApplyToConfig(cfg *config.Config) {
	cfg.OutreachConsent = m.consent
	cfg.OutreachConsentAt = m.consentAt
	cfg.MaxEmailsPerDay = m.maxEmail
	cfg.MaxLinkedInPerDay = m.maxLI
	cfg.OutreachMode = outreach.NormalizeMode(m.runMode)
	cfg.LinkedInSessionCookie = m.liCookie
	cfg.OutreachAutoQueue = m.autoQueue
	cfg.OutreachAICompose = m.aiCompose
	cfg.OutreachAIReview = m.aiReview
	cfg.OutreachGenModel = m.genModel
	cfg.OutreachCheckModel = m.checkModel
	cfg.OutreachMinScore = m.minScore
	cfg.OutreachMaxRetries = m.maxRetries
	cfg.OutreachSMTPVerify = m.smtpVerify
}

func (m OutreachHubModel) effectiveCfg() *config.Config {
	if m.cfg != nil {
		return m.cfg
	}
	cfg, _ := config.Load()
	return cfg
}

func (m OutreachHubModel) CapturesKeys() bool {
	return m.ui == outEditContact || m.ui == outConfirmAction || (m.sub == outreachSubSetup && m.setupFocusIsText() && m.setupInput.Focused()) || m.ui == outRunning
}

func (m OutreachHubModel) setupFocusIsText() bool {
	return m.setupFocus == setupMaxEmail || m.setupFocus == setupMaxLI
}

// gotoSub jumps to sub-section i, clearing modal state; landing on Sent loads
// the audit log. Out-of-range or current-sub jumps are a no-op.
func (m OutreachHubModel) gotoSub(i int) (OutreachHubModel, tea.Cmd) {
	if i < 0 || i >= outreachSubCount || i == m.sub {
		return m, nil
	}
	m.sub = i
	m.ui = outBrowse
	m.errText = ""
	m.clampCursor()
	if m.sub == outreachSubSent {
		m.logLoading = true
		return m, m.loadLogCmd()
	}
	return m, nil
}

func (m *OutreachHubModel) clampCursor() {
	n := len(m.filtered())
	if m.cursor >= n {
		m.cursor = max(0, n-1)
	}
}

func (m OutreachHubModel) channelForSub() outreach.Channel {
	if m.sub == outreachSubLinkedIn {
		return outreach.ChannelLinkedIn
	}
	return outreach.ChannelEmail
}

func (m OutreachHubModel) filtered() []outreach.Item {
	return outreach.FilterChannel(m.items, m.channelForSub())
}

func (m OutreachHubModel) selected() (outreach.Item, bool) {
	list := m.filtered()
	if len(list) == 0 || m.cursor < 0 || m.cursor >= len(list) {
		return outreach.Item{}, false
	}
	return list[m.cursor], true
}

func (m OutreachHubModel) FooterHint() string {
	if m.CapturesKeys() {
		return "typing  ·  enter save  ·  esc cancel  ·  ctrl+c quit"
	}
	if m.ui == outConfirmAction {
		return "y run this one  ·  n / esc skip back  ·  ctrl+c quit"
	}
	if m.ui == outRunning {
		return "auto-running…  ·  esc stop  ·  ctrl+c quit"
	}
	switch m.sub {
	case outreachSubSetup:
		return "↑↓ fields  ·  ←→ / space change  ·  auto-saves  ·  tab/1-4 sections"
	case outreachSubEmail:
		return "g build queue  ·  enter process next  ·  a auto-run  ·  c check replies  ·  e fix To:  ·  tab/1-4 sections"
	case outreachSubLinkedIn:
		return "g build queue  ·  enter open browser  ·  a auto-run remaining  ·  tab/1-4 sections"
	case outreachSubSent:
		return "j/k move  ·  r refresh  ·  tab/1-4 sections"
	default:
		return "tab/1-4 sections"
	}
}

func (m OutreachHubModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case outreachLoadedMsg:
		m.items = msg.Items
		m.building = false
		m.clampCursor()
		return m, nil
	case outreachLogLoadedMsg:
		m.logLoading = false
		if msg.Err == nil {
			m.logEntries = msg.Entries
		} else {
			m.errText = msg.Err.Error()
		}
		return m, nil
	case outreachErrMsg:
		m.errText = msg.Err.Error()
		m.building = false
		m.ui = outBrowse
		return m, nil
	case outreachStatusMsg:
		m.status = msg.Text
		m.errText = ""
		return m, nil
	case outreachQueueBuiltMsg:
		m.building = false
		m.status = fmt.Sprintf("Queue ready: +%d new · %d already queued", msg.Created, msg.Skipped)
		if len(msg.Errs) > 0 {
			m.errText = fmt.Sprintf("%d lookup warnings (first: %s)", len(msg.Errs), msg.Errs[0])
		} else {
			m.errText = ""
		}
		return m, loadOutreachCmd()
	case outreachActionDoneMsg:
		if msg.Err != nil {
			m.errText = msg.Err.Error()
		} else {
			m.status = msg.Text
			m.errText = ""
		}
		m.ui = outBrowse
		cmds := []tea.Cmd{loadOutreachCmd()}
		if m.autoGen > 0 {
			ch := m.channelForSub()
			gen := m.autoGen
			cmds = append(cmds, tea.Tick(1200*time.Millisecond, func(t time.Time) tea.Msg {
				return outreachAutoTickMsg{Channel: ch, Gen: gen}
			}))
			m.ui = outRunning
		}
		return m, tea.Batch(cmds...)
	case outreachAutoTickMsg:
		if msg.Gen != m.autoGen {
			return m, nil
		}
		return m.fireNext(true)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}
