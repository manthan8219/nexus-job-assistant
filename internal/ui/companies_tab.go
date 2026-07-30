package ui

// Package ui — companies_tab.go
// The Companies tab model: CompaniesTabModel state, message types, the Update
// dispatch (list navigation, search, detail, add-company), and data helpers
// (reload, jobCount, scrapedName, companySlug). The key handlers are in
// companies_keys.go and the views in companies_view.go.

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/companies"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

type companiesLoadedMsg struct {
	Items   []companies.Company
	Total   int
	Counts  map[string]int // store.CompanyKey(name) → scraped jobs recorded
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

// companyJobsLoadedMsg carries the scraped jobs recorded for one company
// into the detail view.
type companyJobsLoadedMsg struct {
	Company string
	Jobs    []store.Application
	Err     error
}

type CompaniesTabModel struct {
	width, height int

	items      []companies.Company
	total      int
	counts     map[string]int // store.CompanyKey(name) → scraped jobs recorded
	cursor     int
	err        string
	status     string
	loading    bool
	refreshing bool

	// detail (scraped jobs for one company)
	detail        bool
	detailCompany companies.Company
	detailJobs    []store.Application
	detailCursor  int
	detailLoading bool

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
	return m.searching || m.adding || m.detail || m.search.Focused() || m.country.Focused()
}

func (m CompaniesTabModel) reload() tea.Cmd {
	q := strings.TrimSpace(m.search.Value())
	ctry := strings.TrimSpace(m.country.Value())
	routed := false
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
		var counts map[string]int
		if st, stErr := store.Open(); stErr == nil {
			counts, _ = st.CompanyJobCounts()
			st.Close()
		}
		return companiesLoadedMsg{Items: items, Total: total, Counts: counts, Err: err, Query: q, Country: ctry, Routed: routed}
	}
}

// loadCompanyJobs fetches every scraped job recorded for the given company.
func (m CompaniesTabModel) loadCompanyJobs(company string) tea.Cmd {
	return func() tea.Msg {
		st, err := store.Open()
		if err != nil {
			return companyJobsLoadedMsg{Company: company, Err: err}
		}
		defer st.Close()
		jobs, err := st.ListByCompany(company)
		return companyJobsLoadedMsg{Company: company, Jobs: jobs, Err: err}
	}
}

// jobCount returns how many scraped jobs are recorded for a company.
func (m CompaniesTabModel) jobCount(c companies.Company) int {
	if len(m.counts) == 0 {
		return 0
	}
	if n, ok := m.counts[store.CompanyKey(c.Name)]; ok {
		return n
	}
	slug := companySlug(c.Name)
	if slug == "" {
		return 0
	}
	n := 0
	for name, count := range m.counts {
		if companySlug(name) == slug {
			n += count
		}
	}
	return n
}

// scrapedName resolves the company name as it appears in the scraped-jobs DB.
func (m CompaniesTabModel) scrapedName(c companies.Company) string {
	if len(m.counts) == 0 {
		return c.Name
	}
	key := store.CompanyKey(c.Name)
	if _, ok := m.counts[key]; ok {
		return key
	}
	slug := companySlug(c.Name)
	for name := range m.counts {
		if companySlug(name) == slug {
			return name
		}
	}
	return c.Name
}

// companySlug reduces a name to lowercase letters/digits for fuzzy matching.
func companySlug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// refreshFromNetwork re-pulls OpenJobs + Y Combinator + embedded sources.
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
			m.counts = msg.Counts
			if m.cursor >= len(m.items) {
				m.cursor = max(0, len(m.items)-1)
			}
		}
		return m, nil

	case companyJobsLoadedMsg:
		if !m.detail || companySlug(msg.Company) != companySlug(m.detailCompany.Name) {
			return m, nil
		}
		m.detailLoading = false
		if msg.Err != nil {
			m.err = msg.Err.Error()
			m.detailJobs = nil
		} else {
			m.detailJobs = msg.Jobs
			m.detailCursor = 0
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
		if m.detail {
			return m.updateDetail(msg)
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
		case "enter":
			if m.loading || len(m.items) == 0 || m.cursor >= len(m.items) {
				return m, nil
			}
			m.detail = true
			m.detailCompany = m.items[m.cursor]
			m.detailJobs = nil
			m.detailCursor = 0
			m.detailLoading = true
			m.err = ""
			return m, m.loadCompanyJobs(m.scrapedName(m.detailCompany))
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
