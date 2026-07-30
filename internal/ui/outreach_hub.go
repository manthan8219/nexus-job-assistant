package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	outBrowse outreachUIMode = iota
	outConfirmAction // y/n before sending/opening
	outEditContact
	outRunning // auto-run in progress between ticks
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

// OutreachSetupSaveMsg asks App to merge Setup into config and persist.
type OutreachSetupSaveMsg struct {
	Consent   bool
	ConsentAt string
	MaxEmail  int
	MaxLI     int
	LIMode    string // stores OutreachMode (confirm|queue|auto)
	LICookie  string
	// Pipeline options
	AutoQueue  bool
	AICompose  bool
	AIReview   bool
	GenModel   string
	CheckModel string
	MinScore   int
	MaxRetries int
	SMTPVerify bool
}

// outreachWorkerMsg carries a pipeline progress line from the background worker.
type outreachWorkerMsg struct{ Line string }
type outreachLogLoadedMsg struct {
	Entries []store.OutreachLogEntry
	Err     error
}

// OutreachHubModel — automated email + LinkedIn follow-up after apply.
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
	runMode    string // confirm|queue|auto
	liCookie   string
	setupFocus int
	setupInput textinput.Model

	// Pipeline options (Setup sub-tab, auto-saved)
	autoQueue  bool
	aiCompose  bool
	aiReview   bool
	genModel   string
	checkModel string
	minScore   int
	maxRetries int
	smtpVerify bool

	// Sent log (Sent sub-tab)
	logEntries []store.OutreachLogEntry
	logCursor  int
	logLoading bool

	contactInput textinput.Model
	pending      outreach.Item // item awaiting confirm
	autoGen      int
	building     bool
}

func NewOutreachHubModel() OutreachHubModel {
	ti := textinput.New()
	ti.CharLimit = 200
	ci := textinput.New()
	ci.CharLimit = 120
	ci.Placeholder = "name@company.com"
	return OutreachHubModel{
		maxEmail:     10,
		maxLI:        10,
		minScore:     70,
		maxRetries:   3,
		runMode:      outreach.ModeConfirm,
		setupInput:   ti,
		contactInput: ci,
		ui:           outBrowse,
	}
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

func (m *OutreachHubModel) SetConfig(cfg *config.Config) {
	m.cfg = cfg
	if cfg == nil {
		return
	}
	m.consent = cfg.OutreachConsent
	m.consentAt = cfg.OutreachConsentAt
	m.maxEmail = cfg.MaxEmailsPerDay
	if m.maxEmail <= 0 {
		m.maxEmail = 10
	}
	m.maxLI = cfg.MaxLinkedInPerDay
	if m.maxLI <= 0 {
		m.maxLI = 10
	}
	m.runMode = outreach.EffectiveMode(cfg)
	m.liCookie = cfg.LinkedInSessionCookie
}

func (m *OutreachHubModel) SetJobs(apps []store.Application) { m.jobs = apps }

func (m OutreachHubModel) ApplyToConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	cfg.OutreachConsent = m.consent
	cfg.OutreachConsentAt = m.consentAt
	cfg.MaxEmailsPerDay = m.maxEmail
	cfg.MaxLinkedInPerDay = m.maxLI
	cfg.OutreachMode = outreach.NormalizeMode(m.runMode)
	cfg.LinkedInMode = cfg.OutreachMode
	cfg.LinkedInSessionCookie = m.liCookie
}

func (m OutreachHubModel) effectiveCfg() *config.Config {
	if m.cfg == nil {
		cfg := &config.Config{}
		m.ApplyToConfig(cfg)
		return cfg
	}
	cp := *m.cfg
	m.ApplyToConfig(&cp)
	return &cp
}

func (m OutreachHubModel) CapturesKeys() bool {
	return m.ui == outEditContact || (m.sub == outreachSubSetup && m.setupFocusIsText() && m.setupInput.Focused())
}

func (m OutreachHubModel) setupFocusIsText() bool {
	return m.setupFocus == setupMaxEmail || m.setupFocus == setupMaxLI
}

func (m OutreachHubModel) NextSub() OutreachHubModel {
	m.sub = (m.sub + 1) % outreachSubCount
	m.ui = outBrowse
	m.errText = ""
	m.clampCursor()
	return m
}

func (m OutreachHubModel) PrevSub() OutreachHubModel {
	m.sub = (m.sub - 1 + outreachSubCount) % outreachSubCount
	m.ui = outBrowse
	m.errText = ""
	m.clampCursor()
	return m
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
		return "typing  •  enter save  •  esc cancel  •  ctrl+c quit"
	}
	if m.ui == outConfirmAction {
		return "y run this one  •  n / esc skip back  •  ctrl+c quit"
	}
	if m.ui == outRunning {
		return "auto-running…  •  esc stop  •  ctrl+c quit"
	}
	switch m.sub {
	case outreachSubSetup:
		return "↑↓ fields  •  ←→ / space change  •  auto-saves  •  tab → Email"
	case outreachSubEmail:
		return "g build queue  •  enter process next  •  a auto-run remaining  •  e fix To:  •  tab cycles"
	case outreachSubLinkedIn:
		return "g build queue  •  enter open browser  •  a auto-run remaining  •  tab cycles"
	default:
		return "tab cycles Setup/Email/LinkedIn"
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
		return m.NextSub(), nil
	case "shift+tab", "[":
		return m.PrevSub(), nil
	}

	if m.sub == outreachSubSetup {
		return m.handleSetupKey(key)
	}
	return m.handleChannelKey(key)
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
			Consent:   m.consent,
			ConsentAt: m.consentAt,
			MaxEmail:  m.maxEmail,
			MaxLI:     m.maxLI,
			LIMode:    outreach.NormalizeMode(m.runMode),
			LICookie:  m.liCookie,
		}
	}
}

func (m OutreachHubModel) handleChannelKey(key string) (tea.Model, tea.Cmd) {
	list := m.filtered()
	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(list)-1 {
			m.cursor++
		}
	case "g":
		return m.buildQueue()
	case "a":
		if !m.consent {
			m.errText = "Turn on consent in Setup first."
			return m, nil
		}
		m.autoGen++
		m.ui = outRunning
		m.status = "Auto-running queue…"
		return m.fireNext(true)
	case "enter", "s":
		return m.fireNext(false)
	case "e":
		if m.sub != outreachSubEmail {
			return m, nil
		}
		it, ok := m.selected()
		if !ok {
			m.errText = "No item selected."
			return m, nil
		}
		m.contactInput.SetValue(it.ContactEmail)
		m.contactInput.Focus()
		m.contactInput.CursorEnd()
		m.ui = outEditContact
		return m, textinput.Blink
	case "r":
		return m, loadOutreachCmd()
	}
	return m, nil
}

func (m OutreachHubModel) buildQueue() (tea.Model, tea.Cmd) {
	if len(m.jobs) == 0 {
		m.errText = "No Jobs yet — apply from Dashboard first."
		return m, nil
	}
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

// fireNext processes the next pending item according to run mode.
// forceAuto skips confirm prompts (used by auto-run / 'a').
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
	// Put message on clipboard as a helper when LinkedIn browser opens.
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

func (m OutreachHubModel) View() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	var b strings.Builder
	b.WriteString(m.renderStepStrip())
	b.WriteString("\n")
	b.WriteString(m.renderNextAction())
	b.WriteString("\n\n")

	switch m.sub {
	case outreachSubSetup:
		b.WriteString(m.viewSetup(w))
	case outreachSubEmail:
		b.WriteString(m.viewEmail(w))
	case outreachSubLinkedIn:
		b.WriteString(m.viewLinkedIn(w))
	}

	if m.errText != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render("⚠ " + m.errText))
	} else if m.status != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("✓ " + m.status))
	}
	return b.String()
}

func (m OutreachHubModel) renderStepStrip() string {
	emailOK := outreach.AllOK(outreach.EmailReady(m.effectiveCfg()))
	liOK := outreach.AllOK(outreach.LinkedInReady(m.effectiveCfg()))
	done := [outreachSubCount]bool{
		m.consent,
		emailOK && len(outreach.Pending(m.items, outreach.ChannelEmail)) > 0,
		liOK && len(outreach.Pending(m.items, outreach.ChannelLinkedIn)) > 0,
	}
	parts := make([]string, 0, 6)
	for i, label := range outreachSubLabels {
		num := fmt.Sprintf("%d.", i+1)
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
		parts = append(parts, style.Render(fmt.Sprintf("%s %s %s", mark, num, label)))
		if i < outreachSubCount-1 {
			parts = append(parts, mutedStyle.Render("  →  "))
		}
	}
	return strings.Join(parts, "")
}

func (m OutreachHubModel) renderNextAction() string {
	mode := outreach.NormalizeMode(m.runMode)
	line := ""
	switch {
	case m.ui == outConfirmAction:
		line = "Approve this action? [y] run  [n] cancel"
	case m.ui == outRunning:
		line = "Auto mode running — esc to stop"
	case m.sub == outreachSubSetup:
		if !m.consent {
			line = "Opt in, pick automation mode (Confirm / Queue / Auto), set caps — saves as you go."
		} else {
			line = "Mode: " + mode + " · tab → Email or LinkedIn to build & run queues."
		}
	case m.sub == outreachSubEmail:
		if m.building {
			line = "Finding contacts + drafting emails…"
		} else if len(outreach.Pending(m.items, outreach.ChannelEmail)) == 0 {
			line = "Press g — Nexus builds an email queue from Jobs (Hunter finds To: addresses)."
		} else if mode == outreach.ModeAuto {
			line = "Queue ready · press a to send all remaining (or enter for one)."
		} else if mode == outreach.ModeQueue {
			line = "Queue ready · mash enter to send, send, send…"
		} else {
			line = "Queue ready · enter asks before each send."
		}
	case m.sub == outreachSubLinkedIn:
		if m.building {
			line = "Building LinkedIn queue…"
		} else if len(outreach.Pending(m.items, outreach.ChannelLinkedIn)) == 0 {
			line = "Press g — Nexus queues companies, then opens LinkedIn in your browser."
		} else if mode == outreach.ModeAuto {
			line = "Queue ready · press a to open browser for each company automatically."
		} else if mode == outreach.ModeQueue {
			line = "Queue ready · enter opens the next LinkedIn search (message copied for paste if needed)."
		} else {
			line = "Queue ready · enter asks, then opens LinkedIn in the browser."
		}
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render("→ " + line)
}

func (m OutreachHubModel) viewSetup(w int) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Automated outreach"))
	b.WriteString("\n")
	b.WriteString(primaryStyle.Render("Nexus prepares follow-ups from jobs you already applied to, then runs them."))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Email: finds recruiter addresses + sends via Gmail.  LinkedIn: opens browser searches for you to message."))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("Controls"))
	b.WriteString("\n")
	b.WriteString(m.renderSetupRow(setupConsent, "Consent", m.consentLabel()))
	b.WriteString(m.renderSetupRow(setupRunMode, "Automation mode", outreach.ModeLabel(m.runMode)))
	b.WriteString(m.renderSetupRow(setupMaxEmail, "Max emails / day", strconv.Itoa(m.maxEmail)))
	b.WriteString(m.renderSetupRow(setupMaxLI, "Max LinkedIn opens / day", strconv.Itoa(m.maxLI)))

	if m.setupInput.Focused() {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Edit: ") + m.setupInput.View() + "\n")
	}

	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Confirm = ask each time · Queue = tap Enter repeatedly · Auto = run the whole queue"))
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("Email checklist"))
	b.WriteString("\n" + m.renderChecks(outreach.EmailReady(m.effectiveCfg())))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("LinkedIn checklist"))
	b.WriteString("\n" + m.renderChecks(outreach.LinkedInReady(m.effectiveCfg())))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Secrets → Config → Outreach (Gmail app password, Hunter.io key)."))
	_ = w
	return b.String()
}

func (m OutreachHubModel) consentLabel() string {
	if m.consent {
		return "Yes — run automated follow-ups I start from this tab"
	}
	return "No — outreach blocked"
}

func (m OutreachHubModel) renderSetupRow(focus int, label, value string) string {
	cursor := "  "
	style := mutedStyle
	if m.setupFocus == focus {
		cursor = "▸ "
		style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
	}
	return cursor + style.Render(label) + "  " + primaryStyle.Render(value) + "\n"
}

func (m OutreachHubModel) renderChecks(checks []outreach.Check) string {
	var b strings.Builder
	for _, c := range checks {
		mark := lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render("✗")
		if c.OK {
			mark = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("✓")
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", mark, c.Label))
		if !c.OK && c.FixHint != "" {
			b.WriteString(mutedStyle.Render("      → " + c.FixHint) + "\n")
		}
	}
	return b.String()
}

func (m OutreachHubModel) viewEmail(w int) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Email automation"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("g builds a queue from Jobs · Hunter fills To: · Enter/Auto sends via Gmail (no copy-paste)."))
	b.WriteString("\n\n")
	pend := outreach.Pending(m.items, outreach.ChannelEmail)
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Pending %d · total %d", len(pend), len(m.filtered()))))
	b.WriteString("\n\n")
	list := m.filtered()
	if len(list) == 0 {
		b.WriteString(mutedStyle.Render("Empty. Press g to generate the queue."))
		return b.String()
	}
	b.WriteString(m.renderDraftList(list, w))
	if it, ok := m.selected(); ok {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Selected") + "\n")
		b.WriteString(primaryStyle.Render(fmt.Sprintf("%s — %s", it.Company, it.Role)) + "\n")
		to := it.ContactEmail
		if to == "" {
			to = "(missing — press e, or rebuild with Hunter key)"
		}
		if m.ui == outEditContact {
			b.WriteString(labelStyle.Render("To: ") + m.contactInput.View() + "\n")
		} else {
			b.WriteString(mutedStyle.Render("To: ") + primaryStyle.Render(to) + "\n")
		}
		b.WriteString(mutedStyle.Render("Subject: ") + primaryStyle.Render(it.Subject) + "\n")
		b.WriteString(mutedStyle.Render("Status: ") + primaryStyle.Render(string(it.Status)) + "\n")
		b.WriteString("\n" + labelStyle.Render("Body") + "\n")
		b.WriteString(wrapBody(it.Body, w))
	}
	if m.ui == outConfirmAction && m.pending.Channel == outreach.ChannelEmail {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true).
			Render(fmt.Sprintf("Send email to %s now?  [y]  [n]", m.pending.ContactEmail)))
	}
	return b.String()
}

func (m OutreachHubModel) viewLinkedIn(w int) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("LinkedIn automation"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("g builds a queue · Enter/Auto opens LinkedIn people-search in your browser (message placed on clipboard)."))
	b.WriteString("\n\n")
	pend := outreach.Pending(m.items, outreach.ChannelLinkedIn)
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Pending %d · total %d", len(pend), len(m.filtered()))))
	b.WriteString("\n\n")
	list := m.filtered()
	if len(list) == 0 {
		b.WriteString(mutedStyle.Render("Empty. Press g to generate the queue."))
		return b.String()
	}
	b.WriteString(m.renderDraftList(list, w))
	if it, ok := m.selected(); ok {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Selected") + "\n")
		b.WriteString(primaryStyle.Render(fmt.Sprintf("%s — %s", it.Company, it.Role)) + "\n")
		b.WriteString(mutedStyle.Render("Status: ") + primaryStyle.Render(string(it.Status)) + "\n")
		if it.LinkedInURL != "" {
			b.WriteString(mutedStyle.Render("Opens: ") + primaryStyle.Render(trimURL(it.LinkedInURL, w-8)) + "\n")
		}
		b.WriteString("\n" + labelStyle.Render("Message (auto-copied when browser opens)") + "\n")
		b.WriteString(wrapBody(it.Body, w))
	}
	if m.ui == outConfirmAction && m.pending.Channel == outreach.ChannelLinkedIn {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true).
			Render(fmt.Sprintf("Open LinkedIn for %s?  [y]  [n]", m.pending.Company)))
	}
	return b.String()
}

func (m OutreachHubModel) renderDraftList(list []outreach.Item, w int) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Queue") + "\n")
	limit := 8
	start := 0
	if m.cursor >= limit {
		start = m.cursor - limit + 1
	}
	for i := start; i < len(list) && i < start+limit; i++ {
		it := list[i]
		cur := "  "
		style := primaryStyle
		if i == m.cursor {
			cur = "▸ "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
		}
		line := fmt.Sprintf("[%s] %s · %s", it.Status, it.Company, it.Role)
		if it.ContactEmail != "" {
			line += "  <" + it.ContactEmail + ">"
		}
		if len(line) > w-4 {
			line = line[:max(0, w-4)]
		}
		b.WriteString(cur + style.Render(line) + "\n")
	}
	return b.String()
}

func wrapBody(s string, w int) string {
	if w < 40 {
		w = 40
	}
	maxW := w - 2
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		for len(line) > maxW {
			b.WriteString(primaryStyle.Render(line[:maxW]) + "\n")
			line = line[maxW:]
		}
		b.WriteString(primaryStyle.Render(line) + "\n")
	}
	return b.String()
}

func trimURL(u string, n int) string {
	if n < 16 {
		n = 16
	}
	if len(u) <= n {
		return u
	}
	return u[:n-1] + "…"
}
