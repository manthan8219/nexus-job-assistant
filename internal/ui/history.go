package ui

// Package ui — history.go
// The Jobs/History tab model: message types, HistoryModel state, the Update
// loop (list navigation, search, detail open, outcome cycling, enrich triggers)
// and selection/filtering. The list view is in history_view.go and the detail
// pane in history_detail.go.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// RefreshHistoryMsg replaces the job list (and outcome funnel counts).
type RefreshHistoryMsg struct {
	Apps     []store.Application
	Outcomes map[store.Outcome]int // funnel counts per outcome
}

// HistoryOutcomeRequestMsg asks App to persist an outcome change for one row.
type HistoryOutcomeRequestMsg struct {
	App     store.Application
	Outcome store.Outcome
}

// HistoryEnrichRequestMsg asks App to backfill description(+fit) for one or all missing.
type HistoryEnrichRequestMsg struct {
	All bool // false = selected only
	App store.Application
}

type historyEnrichDoneMsg struct {
	Updated int
	Failed  int
	Err     error
	Status  string
}

// historyEnrichProgressMsg streams one log line while backfill runs (UI stays interactive).
type historyEnrichProgressMsg struct {
	Line string
	Next tea.Cmd
}

// HistoryModel is the Bubble Tea model for the Jobs/History tab.
type HistoryModel struct {
	width        int
	height       int
	apps         []store.Application
	outcomes     map[store.Outcome]int
	cursor       int
	loading      bool
	detail       bool // show detail pane for selected row
	detailVP     viewport.Model
	detailReady  bool
	enriching    bool
	enrichStatus string
	search       textinput.Model
	searching    bool // search box focused
}

func NewHistoryModel() HistoryModel {
	ti := textinput.New()
	ti.Placeholder = "company, role, provider, status, location…"
	ti.CharLimit = 120
	ti.Width = 48
	ti.Prompt = "/ "
	return HistoryModel{loading: true, search: ti}
}

// CapturesKeys is true while search is focused or a job detail is open.
func (m HistoryModel) CapturesKeys() bool {
	return m.searching || m.detail
}

func (m HistoryModel) Init() tea.Cmd { return nil }

func (m HistoryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = max(24, msg.Width-18)
		m = m.syncDetailViewport()
		if m.detail {
			m = m.refreshDetailContent(false)
		}

	case RefreshHistoryMsg:
		m.apps = msg.Apps
		m.outcomes = msg.Outcomes
		m.loading = false
		m = m.clampCursor()
		if m.detail {
			m = m.refreshDetailContent(false)
		}

	case historyEnrichDoneMsg:
		m.enriching = false
		if msg.Err != nil {
			m.enrichStatus = "enrich failed: " + msg.Err.Error()
		} else if msg.Status != "" {
			m.enrichStatus = msg.Status
		} else {
			m.enrichStatus = fmt.Sprintf("updated %d · failed %d", msg.Updated, msg.Failed)
		}
		return m, nil

	case tea.KeyMsg:
		if m.detail {
			switch msg.String() {
			case "esc", "q", "backspace":
				m.detail = false
				return m, nil
			case "o":
				return m.cycleOutcome()
			case "u":
				if m.enriching {
					return m, nil
				}
				if app, ok := m.selectedApp(); ok {
					m.enriching = true
					m.enrichStatus = "fetching description…"
					return m, func() tea.Msg {
						return HistoryEnrichRequestMsg{All: false, App: app}
					}
				}
				return m, nil
			}
			m = m.syncDetailViewport()
			var cmd tea.Cmd
			m.detailVP, cmd = m.detailVP.Update(msg)
			return m, cmd
		}

		if m.searching {
			switch msg.String() {
			case "esc":
				m.searching = false
				m.search.Blur()
				return m, nil
			case "enter":
				m.searching = false
				m.search.Blur()
				m = m.clampCursor()
				if len(m.filtered()) > 0 {
					m.detail = true
					m = m.syncDetailViewport()
					m = m.refreshDetailContent(true)
				}
				return m, nil
			case "ctrl+u":
				m.search.SetValue("")
				m = m.clampCursor()
				return m, nil
			case "up", "down", "ctrl+j", "ctrl+k":
				// Leave search focus and navigate results.
				m.searching = false
				m.search.Blur()
				if msg.String() == "down" || msg.String() == "ctrl+j" {
					m.moveCursor(+1)
				} else {
					m.moveCursor(-1)
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m = m.clampCursor()
			return m, cmd
		}

		switch msg.String() {
		case "/":
			m.searching = true
			return m, m.search.Focus()
		case "esc":
			if strings.TrimSpace(m.search.Value()) != "" {
				m.search.SetValue("")
				m = m.clampCursor()
				return m, nil
			}
		case "j", "down":
			m.moveCursor(+1)
		case "k", "up":
			m.moveCursor(-1)
		case "g":
			m.cursor = 0
		case "G":
			f := m.filtered()
			m.cursor = max(0, len(f)-1)
		case "enter", " ":
			if len(m.filtered()) > 0 {
				m.detail = true
				m = m.syncDetailViewport()
				m = m.refreshDetailContent(true)
			}
		case "u":
			if m.enriching {
				return m, nil
			}
			if app, ok := m.selectedApp(); ok {
				m.enriching = true
				m.enrichStatus = "fetching description for selected…"
				return m, func() tea.Msg {
					return HistoryEnrichRequestMsg{All: false, App: app}
				}
			}
		case "U":
			if m.enriching {
				return m, nil
			}
			m.enriching = true
			m.enrichStatus = "backfilling all empty descriptions…"
			return m, func() tea.Msg {
				return HistoryEnrichRequestMsg{All: true}
			}
		case "o":
			return m.cycleOutcome()
		}
	}
	return m, nil
}

// cycleOutcome advances the selected row through the outcome pipeline
// (replied → interview → offer → rejected → ghosted → clear) and asks App
// to persist it. Only applied rows have outcomes to track.
func (m HistoryModel) cycleOutcome() (tea.Model, tea.Cmd) {
	app, ok := m.selectedApp()
	if !ok || app.Status != store.StatusApplied {
		return m, nil
	}
	next := store.NextOutcome(app.Outcome)
	return m, func() tea.Msg {
		return HistoryOutcomeRequestMsg{App: app, Outcome: next}
	}
}

func (m *HistoryModel) moveCursor(delta int) {
	f := m.filtered()
	if len(f) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(f) {
		m.cursor = len(f) - 1
	}
}

func (m HistoryModel) clampCursor() HistoryModel {
	f := m.filtered()
	if len(f) == 0 {
		m.cursor = 0
		return m
	}
	if m.cursor >= len(f) {
		m.cursor = len(f) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	return m
}

func (m HistoryModel) filtered() []store.Application {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if q == "" {
		return m.apps
	}
	tokens := strings.Fields(q)
	var out []store.Application
	for _, a := range m.apps {
		if jobMatchesQuery(a, tokens) {
			out = append(out, a)
		}
	}
	return out
}

func jobMatchesQuery(a store.Application, tokens []string) bool {
	hay := strings.ToLower(strings.Join([]string{
		a.Company,
		a.Role,
		a.Provider,
		string(a.Status),
		a.Location,
		a.URL,
		a.Reason,
		a.FitSummary,
		fmt.Sprintf("%d", a.FitScore),
	}, " "))
	for _, tok := range tokens {
		if !strings.Contains(hay, tok) {
			return false
		}
	}
	return true
}

func (m HistoryModel) selectedApp() (store.Application, bool) {
	f := m.filtered()
	if len(f) == 0 || m.cursor < 0 || m.cursor >= len(f) {
		return store.Application{}, false
	}
	return f[m.cursor], true
}
