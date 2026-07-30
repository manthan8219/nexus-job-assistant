package ui

// Package ui — companies_keys.go
// Modal key handlers for the Companies tab: the scraped-jobs detail view, the
// search/country filter fields, and the add-company form + save command. The
// model + Update dispatch live in companies_tab.go.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/companies"
)

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

// updateSearch handles keys while the / name or c country filter field is focused.
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

// updateAdd handles keys while the add-company form is open.
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
