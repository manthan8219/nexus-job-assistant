package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/osint"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// ── Messages ──────────────────────────────────────────────────────────────────

type contactsSearchDoneMsg struct{ Result osint.SearchResult }
type contactsSavedMsg struct{ err error }
type contactsLoadedMsg struct{ contacts []osint.Contact }

// ── Sub-tabs ──────────────────────────────────────────────────────────────────

const (
	contactsSubSearch = iota
	contactsSubSaved
	contactsSubCount
)

var contactsSubLabels = [contactsSubCount]string{"Search", "Saved"}

// ── Model ─────────────────────────────────────────────────────────────────────

type contactsMode int

const (
	contactsModeInput contactsMode = iota
	contactsModeResults
	contactsModeSearching
	contactsModeIdle // unfocused — esc propagates to app TAB MODE
)

// ContactsTabModel is the OSINT HR-contact-finder tab.
type ContactsTabModel struct {
	width, height int
	sub           int
	mode          contactsMode
	focusField    int // 0=company, 1=domain

	cfg *config.Config
	st  *store.Store

	companyInput textinput.Model
	domainInput  textinput.Model

	results     []osint.Contact
	savedList   []osint.Contact
	cursor      int
	searching   bool
	statusLine  string
	errLine     string
	sourcesUsed []string
}

func NewContactsTabModel() ContactsTabModel {
	ci := textinput.New()
	ci.Placeholder = "e.g. Linear, Vercel, Stripe"
	ci.CharLimit = 120
	ci.Width = 40
	ci.Focus()

	di := textinput.New()
	di.Placeholder = "e.g. linear.app  (optional)"
	di.CharLimit = 120
	di.Width = 40

	return ContactsTabModel{
		companyInput: ci,
		domainInput:  di,
		mode:         contactsModeInput,
		focusField:   0,
	}
}

func (m *ContactsTabModel) SetConfig(cfg *config.Config) { m.cfg = cfg }
func (m *ContactsTabModel) SetStore(st *store.Store)     { m.st = st }

// CapturesKeys returns true when the tab is actively using keys internally.
// When false, esc propagates to the app and enters chromeNav (TAB MODE).
func (m ContactsTabModel) CapturesKeys() bool {
	return m.mode == contactsModeInput || m.mode == contactsModeResults
}

// Focus re-activates the tab when the user presses enter to leave chromeNav.
func (m ContactsTabModel) Focus() ContactsTabModel {
	m.mode = contactsModeInput
	m.focusField = 0
	m.companyInput.Focus()
	m.domainInput.Blur()
	return m
}

func (m ContactsTabModel) Init() tea.Cmd {
	return m.cmdLoadSaved()
}

func (m ContactsTabModel) cmdLoadSaved() tea.Cmd {
	if m.st == nil {
		return nil
	}
	st := m.st
	return func() tea.Msg {
		contacts, _ := st.ListContacts()
		return contactsLoadedMsg{contacts}
	}
}

func (m ContactsTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		w := maxI(20, m.width/3)
		m.companyInput.Width = w
		m.domainInput.Width = w

	case contactsSearchDoneMsg:
		m.searching = false
		m.results = msg.Result.Contacts
		m.sourcesUsed = msg.Result.Sources
		m.cursor = 0
		m.mode = contactsModeResults
		if len(msg.Result.Errors) > 0 {
			m.errLine = strings.Join(msg.Result.Errors, "  ·  ")
		} else {
			m.errLine = ""
		}
		if len(m.results) == 0 {
			m.statusLine = "No contacts found — add Hunter.io or Apollo.io keys in Config › API Keys"
		} else {
			m.statusLine = fmt.Sprintf("Found %d contacts via: %s",
				len(m.results), strings.Join(m.sourcesUsed, ", "))
		}

	case contactsSavedMsg:
		if msg.err != nil {
			m.statusLine = "Save error: " + msg.err.Error()
		} else {
			m.statusLine = "Saved ✓"
		}
		cmds = append(cmds, m.cmdLoadSaved())

	case contactsLoadedMsg:
		m.savedList = msg.contacts

	case tea.KeyMsg:
		switch m.sub {
		case contactsSubSearch:
			nm, cmd := m.handleSearchKey(msg)
			return nm, cmd
		case contactsSubSaved:
			nm, cmd := m.handleSavedKey(msg)
			return nm, cmd
		}
	}

	// forward to active input
	if m.mode == contactsModeInput {
		var cmd tea.Cmd
		if m.focusField == 0 {
			m.companyInput, cmd = m.companyInput.Update(msg)
		} else {
			m.domainInput, cmd = m.domainInput.Update(msg)
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m ContactsTabModel) handleSearchKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch key.String() {
	case "1":
		m.sub = contactsSubSearch
		return m, nil
	case "2":
		m.sub = contactsSubSaved
		m.cursor = 0
		return m, m.cmdLoadSaved()

	case "tab", "down", "j":
		if m.mode == contactsModeInput {
			m.focusField = 1
			m.domainInput.Focus()
			m.companyInput.Blur()
			return m, nil
		}
		if m.mode == contactsModeResults && key.String() != "tab" && m.cursor < len(m.results)-1 {
			m.cursor++
			return m, nil
		}

	case "shift+tab", "up", "k":
		if m.mode == contactsModeInput {
			m.focusField = 0
			m.companyInput.Focus()
			m.domainInput.Blur()
			return m, nil
		}
		if m.mode == contactsModeResults && key.String() != "shift+tab" && m.cursor > 0 {
			m.cursor--
			return m, nil
		}

	case "enter":
		if m.mode == contactsModeInput {
			company := strings.TrimSpace(m.companyInput.Value())
			if company == "" {
				m.statusLine = "Enter a company name first"
				break
			}
			m.searching = true
			m.mode = contactsModeSearching
			m.results = nil
			m.sourcesUsed = nil
			m.statusLine = "Searching…"
			m.errLine = ""
			m.cursor = 0
			cfg := m.cfg
			domain := strings.TrimSpace(m.domainInput.Value())
			cmds = append(cmds, func() tea.Msg {
				var hunterKey, apolloKey string
				if cfg != nil {
					hunterKey = cfg.HunterKey
					apolloKey = cfg.ApolloKey
				}
				finder := osint.NewFinder(hunterKey, apolloKey)
				finder.Verify = true // deep search: SMTP-probe pattern addresses
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				result := finder.Search(ctx, company, domain)
				return contactsSearchDoneMsg{result}
			})
		} else if m.mode == contactsModeResults && len(m.results) > 0 {
			c := m.results[m.cursor]
			if c.Email != "" {
				_ = clipboard.WriteAll(c.Email)
				m.statusLine = "Copied: " + c.Email
			} else if c.LinkedIn != "" {
				_ = clipboard.WriteAll(c.LinkedIn)
				m.statusLine = "Copied LinkedIn URL"
			}
		}

	case "i":
		if m.mode == contactsModeResults {
			m.mode = contactsModeInput
			m.focusField = 0
			m.companyInput.Focus()
			m.domainInput.Blur()
		}

	case "esc":
		if m.mode == contactsModeResults {
			// First esc: go back to input fields
			m.mode = contactsModeInput
			m.focusField = 0
			m.companyInput.Focus()
			m.domainInput.Blur()
			return m, nil
		}
		if m.mode == contactsModeInput {
			// Esc from input: blur everything so CapturesKeys() returns false,
			// letting the app enter TAB MODE on the very same esc.
			m.mode = contactsModeIdle
			m.companyInput.Blur()
			m.domainInput.Blur()
			return m, nil
		}

	case "s":
		if m.mode == contactsModeResults && len(m.results) > 0 && m.st != nil {
			c := m.results[m.cursor]
			c.FoundAt = time.Now()
			st := m.st
			cmds = append(cmds, func() tea.Msg {
				return contactsSavedMsg{st.SaveContact(c)}
			})
		}

	case "S":
		if m.mode == contactsModeResults && len(m.results) > 0 && m.st != nil {
			all := make([]osint.Contact, len(m.results))
			copy(all, m.results)
			st := m.st
			cmds = append(cmds, func() tea.Msg {
				var lastErr error
				now := time.Now()
				for i := range all {
					all[i].FoundAt = now
					if err := st.SaveContact(all[i]); err != nil {
						lastErr = err
					}
				}
				return contactsSavedMsg{lastErr}
			})
		}
	}

	if m.mode == contactsModeInput {
		var cmd tea.Cmd
		if m.focusField == 0 {
			m.companyInput, cmd = m.companyInput.Update(key)
		} else {
			m.domainInput, cmd = m.domainInput.Update(key)
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m ContactsTabModel) handleSavedKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch key.String() {
	case "1":
		m.sub = contactsSubSearch
	case "2":
		m.sub = contactsSubSaved
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.savedList)-1 {
			m.cursor++
		}
	case "enter", "c":
		if len(m.savedList) > 0 {
			c := m.savedList[m.cursor]
			if c.Email != "" {
				_ = clipboard.WriteAll(c.Email)
				m.statusLine = "Copied: " + c.Email
			}
		}
	case "d":
		if len(m.savedList) > 0 && m.st != nil {
			id := m.savedList[m.cursor].ID
			st := m.st
			prevCursor := m.cursor
			cmds = append(cmds, func() tea.Msg {
				_ = st.DeleteContact(id)
				contacts, _ := st.ListContacts()
				return contactsLoadedMsg{contacts}
			})
			if prevCursor > 0 {
				m.cursor--
			}
		}
	}
	return m, tea.Batch(cmds...)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m ContactsTabModel) FooterHint() string {
	if m.sub == contactsSubSaved {
		return "↑↓ navigate  •  enter/c copy email  •  d delete  •  1 search  •  esc tab mode"
	}
	if m.mode == contactsModeResults {
		return "↑↓ navigate  •  enter copy email  •  s save one  •  S save all  •  i back to search  •  esc tab mode"
	}
	return "tab switch field  •  enter search  •  2 saved contacts  •  esc tab mode"
}

func (m ContactsTabModel) View() string {
	var b strings.Builder

	// Sub-tab bar
	subTabs := make([]string, contactsSubCount)
	for i, label := range contactsSubLabels {
		if i == m.sub {
			subTabs[i] = lipgloss.NewStyle().
				Bold(true).Foreground(lipgloss.Color(colorPurple)).
				Padding(0, 1).Render("▸ " + label)
		} else {
			subTabs[i] = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorGrey)).
				Padding(0, 1).Render("  " + label)
		}
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, subTabs...))
	b.WriteString("\n\n")

	switch m.sub {
	case contactsSubSearch:
		b.WriteString(m.viewSearch())
	case contactsSubSaved:
		b.WriteString(m.viewSaved())
	}
	return b.String()
}

func (m ContactsTabModel) viewSearch() string {
	var b strings.Builder

	// Source status row
	srcs := []string{}
	if m.cfg != nil && m.cfg.HunterKey != "" {
		srcs = append(srcs, lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("● Hunter.io"))
	} else {
		srcs = append(srcs, lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).Render("○ Hunter.io (add key in Config)"))
	}
	if m.cfg != nil && m.cfg.ApolloKey != "" {
		srcs = append(srcs, lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("● Apollo.io"))
	} else {
		srcs = append(srcs, lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).Render("○ Apollo.io (add key in Config)"))
	}
	srcs = append(srcs, lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("● GitHub"))
	srcs = append(srcs, lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("● Patterns"))
	b.WriteString("  Sources: " + strings.Join(srcs, "   ") + "\n\n")

	// Input fields
	purpleBold := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true)
	greyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey))

	compLabel := greyStyle.Render("  Company")
	domLabel := greyStyle.Render("  Domain ")
	if m.mode == contactsModeInput && m.focusField == 0 {
		compLabel = purpleBold.Render("▸ Company")
	}
	if m.mode == contactsModeInput && m.focusField == 1 {
		domLabel = purpleBold.Render("▸ Domain ")
	}
	b.WriteString(compLabel + "  " + m.companyInput.View() + "\n")
	b.WriteString(domLabel + "  " + m.domainInput.View() + "\n\n")

	// Status / error
	if m.errLine != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render("⚠ "+m.errLine) + "\n")
	}
	if m.statusLine != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).Render(m.statusLine) + "\n")
	}

	if m.searching {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Render("Searching…") + "\n")
		return b.String()
	}

	if len(m.results) == 0 {
		return b.String()
	}

	// Results list
	b.WriteString("\n")
	colW := [5]int{22, 34, 8, 6, 24}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorGrey)).Render(
		fmt.Sprintf("  %-*s  %-*s  %-3s  %-*s  %-*s  %s",
			colW[0], "Name",
			colW[1], "Email",
			"T",
			colW[2], "Source",
			colW[3], "Conf",
			"Notes",
		),
	)
	b.WriteString(header + "\n")
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).Render("  " + strings.Repeat("─", maxI(10, m.width-6)))
	b.WriteString(sep + "\n")

	visH := maxI(3, m.height-16)
	start := 0
	if m.cursor >= visH {
		start = m.cursor - visH + 1
	}

	for i := start; i < len(m.results) && i < start+visH; i++ {
		c := m.results[i]
		conf := ""
		if c.Confidence > 0 {
			conf = fmt.Sprintf("%d%%", c.Confidence)
		}
		typeBadge := ctEmailTypeBadge(c.EmailType)
		row := fmt.Sprintf("%-*s  %-*s  %-3s  %-*s  %-*s  %s",
			colW[0], ctTruncate(c.Name, colW[0]),
			colW[1], ctTruncate(c.Email, colW[1]),
			typeBadge,
			colW[2], c.Source,
			colW[3], conf,
			ctTruncate(c.Notes, colW[4]),
		)
		if i == m.cursor {
			prefix := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render("▶ ")
			b.WriteString(prefix + lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Render(row) + "\n")
		} else {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(textPrimary).Render(row) + "\n")
		}
	}

	return b.String()
}

func (m ContactsTabModel) viewSaved() string {
	var b strings.Builder

	if len(m.savedList) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).
			Render("  No saved contacts yet. Search first, then press s to save a contact.") + "\n")
		return b.String()
	}

	b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).
		Render(fmt.Sprintf("%d saved contacts\n\n", len(m.savedList))))

	if m.statusLine != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).Render(m.statusLine) + "\n\n")
	}

	colW := [4]int{22, 28, 34, 8}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorGrey)).Render(
		fmt.Sprintf("  %-*s  %-*s  %-*s  %s",
			colW[0], "Company",
			colW[1], "Name · Title",
			colW[2], "Email",
			"Source",
		),
	)
	b.WriteString(header + "\n")
	sep := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).Render("  " + strings.Repeat("─", maxI(10, m.width-6)))
	b.WriteString(sep + "\n")

	visH := maxI(3, m.height-10)
	start := 0
	if m.cursor >= visH {
		start = m.cursor - visH + 1
	}

	for i := start; i < len(m.savedList) && i < start+visH; i++ {
		c := m.savedList[i]
		nt := c.Name
		if c.Title != "" {
			nt = c.Name + " · " + c.Title
		}
		row := fmt.Sprintf("%-*s  %-*s  %-*s  %s",
			colW[0], ctTruncate(c.Company, colW[0]),
			colW[1], ctTruncate(nt, colW[1]),
			colW[2], ctTruncate(c.Email, colW[2]),
			c.Source,
		)
		if i == m.cursor {
			prefix := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render("▶ ")
			b.WriteString(prefix + lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Render(row) + "\n")
		} else {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(textPrimary).Render(row) + "\n")
		}
	}
	return b.String()
}

// ctTruncate shortens s to at most n runes.
func ctTruncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}

// ctEmailTypeBadge returns a short 1-char badge for the email type column.
// W=work  P=personal  ~=pattern
func ctEmailTypeBadge(t string) string {
	switch t {
	case "personal":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true).Render("P")
	case "work":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("W")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorGrey)).Render("~")
	}
}

// maxI returns the larger of two ints.
func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}
