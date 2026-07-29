package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthanmanthan/nexus/internal/companies"
)

type companiesLoadedMsg struct {
	Items   []companies.Company
	Total   int
	Err     error
	Query   string // resolved name query (may clear country-as-name)
	Country string
	Routed  bool // true when name search was reinterpreted as country
}

type companySavedMsg struct {
	Err  error
	Name string
}

type companiesRefreshedMsg struct {
	N   int
	Err error
}

type CompaniesTabModel struct {
	width, height int

	items   []companies.Company
	total   int
	cursor  int
	err        string
	status     string
	loading    bool
	refreshing bool

	search    textinput.Model
	country   textinput.Model
	searching bool // which field: use focus on search vs country via tab in search mode

	adding   bool
	add      [5]textinput.Model // name, website, board_url, countries, ats
	addFocus int
}

const (
	caName = iota
	caWebsite
	caBoardURL
	caCountries
	caATS
	caCount
)

func NewCompaniesTabModel() CompaniesTabModel {
	s := textinput.New()
	s.Placeholder = "company name / board / ats (not country)"
	s.CharLimit = 80
	s.Width = 40
	s.Prompt = "/ "

	c := textinput.New()
	c.Placeholder = "hire country — e.g. India or IN  (press c)"
	c.CharLimit = 40
	c.Width = 36
	c.Prompt = "c "

	var add [caCount]textinput.Model
	placeholders := []string{"Company name *", "Website", "ATS / careers URL *", "Countries (comma: India, US)", "ATS hint (greenhouse/lever) optional"}
	for i := 0; i < caCount; i++ {
		t := textinput.New()
		t.Placeholder = placeholders[i]
		t.CharLimit = 200
		t.Width = 50
		t.Prompt = "  "
		add[i] = t
	}
	return CompaniesTabModel{search: s, country: c, add: add, loading: true}
}

func (m CompaniesTabModel) Init() tea.Cmd {
	return m.reload()
}

func (m CompaniesTabModel) CapturesKeys() bool {
	return m.searching || m.adding || m.search.Focused() || m.country.Focused()
}

func (m CompaniesTabModel) reload() tea.Cmd {
	q := strings.TrimSpace(m.search.Value())
	ctry := strings.TrimSpace(m.country.Value())
	routed := false
	// Typing "India" in / search is a common mistake — treat known country
	// names/ISO codes as the country filter, not a name substring.
	if ctry == "" && q != "" {
		if _, iso, ok := companies.NormalizeCountry(q); ok && iso != "" {
			ctry = q
			q = ""
			routed = true
		}
	}
	return func() tea.Msg {
		db, err := companies.OpenDefault()
		if err != nil {
			return companiesLoadedMsg{Err: err}
		}
		defer db.Close()
		total, _ := db.Count()
		items, err := db.Search(q, ctry, 5000)
		return companiesLoadedMsg{Items: items, Total: total, Err: err, Query: q, Country: ctry, Routed: routed}
	}
}

// refreshFromNetwork re-pulls OpenJobs + Y Combinator + embedded sources
// from the network, regardless of whether they were already seeded.
// This runs as a bubbletea Cmd (its own goroutine) since it can take
// 1-2 minutes end to end.
func (m CompaniesTabModel) refreshFromNetwork() tea.Cmd {
	return func() tea.Msg {
		n, err := companies.RefreshCompanies()
		return companiesRefreshedMsg{N: n, Err: err}
	}
}

func (m CompaniesTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = max(20, msg.Width-28)
		m.country.Width = max(16, msg.Width/3)

	case companiesLoadedMsg:
		m.loading = false
		if msg.Routed || msg.Country != "" || msg.Query != m.search.Value() {
			m.search.SetValue(msg.Query)
			m.country.SetValue(msg.Country)
		}
		if msg.Routed {
			m.status = fmt.Sprintf("using country filter %q (not name search)", msg.Country)
		}
		if msg.Err != nil {
			m.err = msg.Err.Error()
			m.items = nil
		} else {
			m.err = ""
			m.items = msg.Items
			m.total = msg.Total
			if m.cursor >= len(m.items) {
				m.cursor = max(0, len(m.items)-1)
			}
		}
		return m, nil

	case companiesRefreshedMsg:
		m.refreshing = false
		if msg.Err != nil {
			m.err = "refresh: " + msg.Err.Error()
			return m, nil
		}
		m.err = ""
		m.status = fmt.Sprintf("refreshed — %d companies upserted from network", msg.N)
		m.loading = true
		return m, m.reload()

	case companySavedMsg:
		if msg.Err != nil {
			m.err = msg.Err.Error()
			return m, nil
		}
		m.adding = false
		m.status = "saved " + msg.Name
		m.err = ""
		for i := range m.add {
			m.add[i].SetValue("")
			m.add[i].Blur()
		}
		m.loading = true
		return m, m.reload()

	case tea.KeyMsg:
		if m.adding {
			return m.updateAdd(msg)
		}
		if m.search.Focused() || m.country.Focused() {
			return m.updateSearch(msg)
		}
		switch msg.String() {
		case "/":
			m.searching = true
			return m, m.search.Focus()
		case "c":
			m.searching = true
			return m, m.country.Focus()
		case "a":
			m.adding = true
			m.addFocus = 0
			m.status = "add company — fill fields, enter save, esc cancel"
			return m, m.add[0].Focus()
		case "r":
			m.loading = true
			m.status = "refreshing…"
			return m, m.reload()
		case "R":
			if m.refreshing {
				return m, nil
			}
			m.refreshing = true
			m.status = "fetching companies from network (OpenJobs + Y Combinator)… this can take a minute or two"
			m.err = ""
			return m, m.refreshFromNetwork()
		case "j", "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "g":
			m.cursor = 0
		case "G":
			m.cursor = max(0, len(m.items)-1)
		}
	}
	return m, nil
}

func (m CompaniesTabModel) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.search.Blur()
		m.country.Blur()
		m.searching = false
		m.loading = true
		return m, m.reload()
	case "enter":
		q := strings.TrimSpace(m.search.Value())
		if strings.TrimSpace(m.country.Value()) == "" && q != "" {
			if _, iso, ok := companies.NormalizeCountry(q); ok && iso != "" {
				m.country.SetValue(q)
				m.search.SetValue("")
				m.status = fmt.Sprintf("country filter → %s (not name search)", q)
			}
		}
		m.search.Blur()
		m.country.Blur()
		m.searching = false
		m.loading = true
		return m, m.reload()
	case "tab":
		if m.search.Focused() {
			m.search.Blur()
			return m, m.country.Focus()
		}
		m.country.Blur()
		return m, m.search.Focus()
	}
	var cmd tea.Cmd
	if m.search.Focused() {
		m.search, cmd = m.search.Update(msg)
	} else {
		m.country, cmd = m.country.Update(msg)
	}
	return m, cmd
}

func (m CompaniesTabModel) updateAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.adding = false
		for i := range m.add {
			m.add[i].Blur()
		}
		m.status = "add cancelled"
		return m, nil
	case "tab", "down":
		m.add[m.addFocus].Blur()
		m.addFocus = (m.addFocus + 1) % caCount
		return m, m.add[m.addFocus].Focus()
	case "shift+tab", "up":
		m.add[m.addFocus].Blur()
		m.addFocus = (m.addFocus - 1 + caCount) % caCount
		return m, m.add[m.addFocus].Focus()
	case "enter":
		return m, m.saveNew()
	}
	var cmd tea.Cmd
	m.add[m.addFocus], cmd = m.add[m.addFocus].Update(msg)
	return m, cmd
}

func (m CompaniesTabModel) saveNew() tea.Cmd {
	name := strings.TrimSpace(m.add[caName].Value())
	website := strings.TrimSpace(m.add[caWebsite].Value())
	boardURL := strings.TrimSpace(m.add[caBoardURL].Value())
	countriesRaw := strings.TrimSpace(m.add[caCountries].Value())
	atsHint := strings.ToLower(strings.TrimSpace(m.add[caATS].Value()))

	return func() tea.Msg {
		if name == "" {
			return companySavedMsg{Err: fmt.Errorf("company name is required")}
		}
		ats, board := companies.ParseATSURL(boardURL)
		if ats == "" && atsHint != "" {
			ats = atsHint
		}
		if board == "" && boardURL != "" && !strings.Contains(boardURL, "://") {
			board = boardURL
			if ats != "" {
				switch ats {
				case "greenhouse":
					boardURL = "https://boards.greenhouse.io/" + board
				case "lever":
					boardURL = "https://jobs.lever.co/" + board
				case "ashby":
					boardURL = "https://jobs.ashbyhq.com/" + board
				}
			}
		}
		var names, codes []string
		for _, part := range strings.Split(countriesRaw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			n, iso, ok := companies.NormalizeCountry(part)
			if !ok {
				continue
			}
			names = append(names, n)
			if iso != "" {
				codes = append(codes, iso)
			}
		}
		db, err := companies.OpenDefault()
		if err != nil {
			return companySavedMsg{Err: err}
		}
		defer db.Close()
		err = db.Upsert(companies.Company{
			Name:             name,
			Website:          website,
			ATS:              ats,
			Board:            board,
			BoardURL:         boardURL,
			HireCountries:    names,
			HireCountryCodes: codes,
			Kind:             "startup",
			Industry:         "startup",
			Source:           "manual",
		})
		return companySavedMsg{Err: err, Name: name}
	}
}

func (m CompaniesTabModel) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	var b strings.Builder
	b.WriteString("\n  " + labelStyle.Render("COMPANIES") + "  " + m.resultsSummary() + "\n")
	b.WriteString("  " + mutedStyle.Render("OpenJobs + ATS boards + India priority + Y Combinator + manual. Use c for country, / for name, R to refetch.") + "\n\n")

	if m.adding {
		b.WriteString("  " + labelStyle.Render("ADD COMPANY") + "\n")
		labels := []string{"Name", "Website", "Board URL", "Countries", "ATS hint"}
		for i := 0; i < caCount; i++ {
			mark := " "
			if i == m.addFocus {
				mark = "▸"
			}
			b.WriteString(fmt.Sprintf("  %s %-10s %s\n", mark, labels[i], m.add[i].View()))
		}
		b.WriteString("\n  " + mutedStyle.Render("tab fields · enter save · esc cancel") + "\n")
		if m.err != "" {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(m.err) + "\n")
		}
		return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(b.String())
	}

	b.WriteString("  " + m.search.View() + "\n")
	b.WriteString("  " + m.country.View() + "\n")
	if m.status != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render(m.status) + "\n")
	} else {
		b.WriteString("\n")
	}
	if m.err != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(m.err) + "\n")
	}

	headerH := lipgloss.Height(b.String())
	listH := h - headerH
	if listH < 5 {
		listH = 5
	}

	var list strings.Builder
	if m.loading {
		list.WriteString(mutedStyle.Render("  Loading…"))
	} else if len(m.items) == 0 {
		list.WriteString(mutedStyle.Render("  No match. Press c → India for hire-country, or a to add a company.") + "\n")
	} else {
		list.WriteString(mutedStyle.Render(fmt.Sprintf("  %-26s  %-12s  %-14s  %-8s  %s", "NAME", "ATS", "BOARD", "COUNTRY", "SOURCE")) +
			mutedStyle.Render(fmt.Sprintf("   %d/%d", m.cursor+1, len(m.items))) + "\n")
		rows := listH - 1
		if rows < 3 {
			rows = 3
		}
		start := m.cursor - rows/2
		if start < 0 {
			start = 0
		}
		end := start + rows
		if end > len(m.items) {
			end = len(m.items)
		}
		if end-start < rows && start > 0 {
			start = max(0, end-rows)
		}
		for i := start; i < end; i++ {
			c := m.items[i]
			ats := c.ATS
			if ats == "" {
				ats = "—"
			}
			countries := strings.Join(c.HireCountryCodes, ",")
			if countries == "" {
				countries = "—"
			}
			line := fmt.Sprintf("  %-26s  %-12s  %-14s  %-8s  %s",
				truncate(c.Name, 26), ats, truncate(c.Board, 14), truncate(countries, 8), truncate(c.Source, 14))
			if i == m.cursor {
				line = lipgloss.NewStyle().
					Background(lipgloss.AdaptiveColor{Light: "#EDE9FE", Dark: "#1E1B4B"}).
					Width(max(20, w-1)).
					Render(line)
			} else {
				line = primaryStyle.Render(line)
			}
			list.WriteString(line + "\n")
		}
	}

	listStr := lipgloss.NewStyle().Height(listH).MaxHeight(listH).Width(w).Render(list.String())
	out := b.String() + listStr
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(out)
}

func (m CompaniesTabModel) resultsSummary() string {
	q := strings.TrimSpace(m.search.Value())
	ctry := strings.TrimSpace(m.country.Value())
	n := len(m.items)
	countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true)
	var filter string
	switch {
	case q != "" && ctry != "":
		filter = fmt.Sprintf(" matching %q in %s", q, ctry)
	case ctry != "":
		filter = fmt.Sprintf(" hiring in %s", ctry)
	case q != "":
		filter = fmt.Sprintf(" matching %q", q)
	}
	if m.loading {
		return mutedStyle.Render("loading…")
	}
	if n == 0 {
		return countStyle.Render("0 results") + mutedStyle.Render(filter+fmt.Sprintf(" · %d in database", m.total))
	}
	word := "results"
	if n == 1 {
		word = "result"
	}
	return countStyle.Render(fmt.Sprintf("%d %s", n, word)) + mutedStyle.Render(filter+fmt.Sprintf(" · %d in database", m.total))
}

func (m CompaniesTabModel) FooterHint() string {
	if m.adding {
		return "adding company  ·  tab fields  ·  enter save  ·  esc cancel  ·  ctrl+c quit"
	}
	if m.CapturesKeys() {
		return "filtering  ·  enter apply  ·  esc done  ·  ctrl+c quit"
	}
	if m.refreshing {
		return "fetching from network…  ·  ctrl+c quit"
	}
	return "/ name search  ·  c country (India)  ·  a add  ·  r reload  ·  R refetch from network  ·  j/k move  ·  esc tab mode  ·  ctrl+c quit"
}
