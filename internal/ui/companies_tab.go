package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		// Scraped-job counts per company (applications DB). Non-fatal: if the
		// jobs DB can't be opened the JOBS column just renders dashes.
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

// jobCount returns how many scraped jobs are recorded for a company: exact
// normalized-name match first, then an alphanumeric-slug fallback so
// punctuation differences ("Acme, Inc." vs "Acme Inc") still line up.
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

// scrapedName resolves the company name as it appears in the scraped-jobs DB,
// so the detail query matches even when punctuation/case differs slightly.
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
			m.counts = msg.Counts
			if m.cursor >= len(m.items) {
				m.cursor = max(0, len(m.items)-1)
			}
		}
		return m, nil

	case companyJobsLoadedMsg:
		if !m.detail || companySlug(msg.Company) != companySlug(m.detailCompany.Name) {
			return m, nil // stale load for a company no longer open
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

// updateDetail handles keys while the scraped-jobs detail view is open.
func (m CompaniesTabModel) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "backspace":
		m.detail = false
		m.detailJobs = nil
		m.detailLoading = false
		m.err = ""
		return m, nil
	case "j", "down":
		if m.detailCursor < len(m.detailJobs)-1 {
			m.detailCursor++
		}
	case "k", "up":
		if m.detailCursor > 0 {
			m.detailCursor--
		}
	case "g":
		m.detailCursor = 0
	case "G":
		m.detailCursor = max(0, len(m.detailJobs)-1)
	case "o":
		if m.detailCursor < len(m.detailJobs) {
			url := m.detailJobs[m.detailCursor].URL
			if strings.TrimSpace(url) != "" {
				m.status = "opening " + truncate(url, 60) + " …"
				return m, func() tea.Msg {
					_ = openURL(url)
					return nil
				}
			}
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
	b.WriteString("  " + mutedStyle.Render("OpenJobs + ATS boards + India priority + Y Combinator + manual. JOBS = scraped roles recorded · enter opens them.") + "\n\n")

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

	if m.detail {
		return m.detailView(w, h)
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
		list.WriteString(mutedStyle.Render(fmt.Sprintf("  %-26s  %-12s  %-14s  %-8s  %5s  %s", "NAME", "ATS", "BOARD", "COUNTRY", "JOBS", "SOURCE")) +
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
			jobsCell := mutedStyle.Render(fmt.Sprintf("%5s", "—"))
			if n := m.jobCount(c); n > 0 {
				jobsCell = lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorOrange)).Bold(true).
					Render(fmt.Sprintf("%5d", n))
			}
			line := fmt.Sprintf("  %-26s  %-12s  %-14s  %-8s  ", truncate(c.Name, 26), ats, truncate(c.Board, 14), truncate(countries, 8)) +
				jobsCell + "  " + primaryStyle.Render(truncate(c.Source, 14))
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

// detailView renders the scraped jobs recorded for the selected company.
func (m CompaniesTabModel) detailView(w, h int) string {
	c := m.detailCompany
	var b strings.Builder

	b.WriteString("\n  " + labelStyle.Render("◂ "+strings.ToUpper(truncate(c.Name, 40))))

	applied, skipped, failed := 0, 0, 0
	for _, j := range m.detailJobs {
		switch j.Status {
		case store.StatusApplied:
			applied++
		case store.StatusSkipped:
			skipped++
		case store.StatusFailed:
			failed++
		}
	}
	countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Bold(true)
	if m.detailLoading {
		b.WriteString("  " + mutedStyle.Render("loading scraped jobs…") + "\n")
	} else {
		word := "jobs"
		if len(m.detailJobs) == 1 {
			word = "job"
		}
		b.WriteString("  " + countStyle.Render(fmt.Sprintf("%d scraped %s", len(m.detailJobs), word)) +
			mutedStyle.Render(fmt.Sprintf("  ·  %d applied  ·  %d skipped  ·  %d failed", applied, skipped, failed)) + "\n")
	}

	var meta []string
	if c.ATS != "" {
		s := c.ATS
		if c.Board != "" {
			s += "/" + c.Board
		}
		meta = append(meta, s)
	}
	if c.Website != "" {
		meta = append(meta, c.Website)
	}
	if len(c.HireCountryCodes) > 0 {
		meta = append(meta, "hires: "+strings.Join(c.HireCountryCodes, ", "))
	}
	if len(meta) > 0 {
		b.WriteString("  " + mutedStyle.Render(truncate(strings.Join(meta, "  ·  "), max(20, w-4))) + "\n")
	}
	b.WriteString("\n")

	headerH := lipgloss.Height(b.String())
	listH := h - headerH - 2 // leave room for the selected-job URL line
	if listH < 5 {
		listH = 5
	}

	var list strings.Builder
	switch {
	case m.detailLoading:
		list.WriteString(mutedStyle.Render("  Loading…"))
	case len(m.detailJobs) == 0:
		list.WriteString(mutedStyle.Render("  No scraped jobs recorded for this company yet. Run a search — matching roles will show up here."))
	default:
		cw := companyJobColWidths(w)
		list.WriteString(mutedStyle.Render(fmt.Sprintf("  %-*s  %-10s  %-4s  %-*s  %-9s  %s",
			cw.role, "ROLE", "STATUS", "FIT", cw.location, "LOCATION", "POSTED", "PROVIDER")) +
			mutedStyle.Render(fmt.Sprintf("   %d/%d", m.detailCursor+1, len(m.detailJobs))) + "\n")
		rows := listH - 1
		if rows < 3 {
			rows = 3
		}
		start := m.detailCursor - rows/2
		if start < 0 {
			start = 0
		}
		end := start + rows
		if end > len(m.detailJobs) {
			end = len(m.detailJobs)
		}
		if end-start < rows && start > 0 {
			start = max(0, end-rows)
		}
		for i := start; i < end; i++ {
			list.WriteString(renderCompanyJobRow(m.detailJobs[i], cw, i == m.detailCursor, w) + "\n")
		}
	}

	listStr := lipgloss.NewStyle().Height(listH).MaxHeight(listH).Width(w).Render(list.String())
	out := b.String() + listStr

	if !m.detailLoading && m.detailCursor < len(m.detailJobs) {
		if u := strings.TrimSpace(m.detailJobs[m.detailCursor].URL); u != "" {
			out += "\n  " + mutedStyle.Render(truncate(u, max(20, w-4)))
		}
	}
	if m.err != "" {
		out += "\n  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(m.err)
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(out)
}

type companyJobColW struct{ role, location int }

func companyJobColWidths(w int) companyJobColW {
	// fixed: indent 2 + status 10 + fit 4 + posted 9 + provider 12 + 5 gaps of 2
	avail := w - 47
	if avail < 40 {
		avail = 40
	}
	role := avail * 55 / 100
	if role < 20 {
		role = 20
	}
	location := avail - role
	if location < 12 {
		location = 12
	}
	return companyJobColW{role: role, location: location}
}

func renderCompanyJobRow(j store.Application, cw companyJobColW, selected bool, width int) string {
	fit := "  —"
	if j.FitScore > 0 {
		fit = fmt.Sprintf("%3d", j.FitScore)
	}
	posted := j.PostedAt
	if posted.IsZero() {
		posted = j.AppliedAt
	}
	date := "—"
	if !posted.IsZero() {
		date = posted.Format("Jan 02")
	}
	prov := j.Provider
	if prov == "" {
		prov = "—"
	}
	base := fmt.Sprintf("  %-*s  %s  %-4s  %-*s  %-9s  %s",
		cw.role, truncate(j.Role, cw.role),
		statusBadge(j.Status),
		fit,
		cw.location, truncate(jobLocationLabel(j), cw.location),
		date,
		truncate(prov, 12),
	)
	if selected {
		return lipgloss.NewStyle().
			Background(lipgloss.AdaptiveColor{Light: "#EDE9FE", Dark: "#1E1B4B"}).
			Width(max(20, width-1)).
			Render(base)
	}
	return primaryStyle.Render(base)
}

// jobLocationLabel shows the job location, tagging remote roles with a marker.
func jobLocationLabel(j store.Application) string {
	loc := strings.TrimSpace(j.Location)
	if loc == "" {
		if j.Remote {
			return "Remote"
		}
		return "—"
	}
	if j.Remote && !strings.Contains(strings.ToLower(loc), "remote") {
		return loc + " · R"
	}
	return loc
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
	if m.detail {
		return "scraped jobs  ·  j/k move  ·  o open in browser  ·  esc/q back  ·  ctrl+c quit"
	}
	if m.CapturesKeys() {
		return "filtering  ·  enter apply  ·  esc done  ·  ctrl+c quit"
	}
	if m.refreshing {
		return "fetching from network…  ·  ctrl+c quit"
	}
	return "/ name search  ·  c country (India)  ·  enter view jobs  ·  a add  ·  r reload  ·  R refetch from network  ·  j/k move  ·  esc tab mode  ·  ctrl+c quit"
}
