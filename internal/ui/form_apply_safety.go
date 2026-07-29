package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var workAuthOptions = []struct{ value, label string }{
	{"authorized", "Authorized to work"},
	{"citizen", "Citizen / permanent resident"},
	{"need_sponsorship", "Need sponsorship"},
	{"unspecified", "Prefer not to say"},
}

var coverLetterOptions = []struct{ value, label string }{
	{"off", "Off"},
	{"template", "Use template below"},
	{"ai", "AI draft (when Assist is on)"},
}

func (m *FormModel) initApplySafetyFromCfg(cfgApplyConsent bool, consentAt string, maxRun, maxDay, delay int, blocklist, workAuth, coverMode, coverText string) {
	m.applyConsent = cfgApplyConsent
	m.applyConsentAt = consentAt
	if m.applyConsent {
		m.applyConsentCursor = 0
	} else {
		m.applyConsentCursor = 1
	}
	if maxRun <= 0 {
		maxRun = 10
	}
	if maxDay <= 0 {
		maxDay = 25
	}
	if delay <= 0 {
		delay = 3
	}
	m.inputs[fMaxPerRun].SetValue(strconv.Itoa(maxRun))
	m.inputs[fMaxPerDay].SetValue(strconv.Itoa(maxDay))
	m.inputs[fApplyDelaySec].SetValue(strconv.Itoa(delay))
	m.inputs[fCompanyBlocklist].SetValue(blocklist)
	m.inputs[fCoverLetterText].SetValue(coverText)

	m.workAuth = workAuth
	if m.workAuth == "" {
		m.workAuth = "unspecified"
	}
	m.workAuthCursor = indexWorkAuth(m.workAuth)

	m.coverLetterMode = coverMode
	if m.coverLetterMode == "" {
		m.coverLetterMode = "off"
	}
	m.coverLetterCursor = indexCoverMode(m.coverLetterMode)
}

func indexWorkAuth(v string) int {
	for i, o := range workAuthOptions {
		if o.value == v {
			return i
		}
	}
	return len(workAuthOptions) - 1
}

func indexCoverMode(v string) int {
	for i, o := range coverLetterOptions {
		if o.value == v {
			return i
		}
	}
	return 0
}

func (m FormModel) updateApplySafetyKeys(key string) (FormModel, tea.Cmd, bool) {
	switch m.focused {
	case fApplyConsent:
		switch key {
		case "left", "h", "right", "l", " ":
			if key == "left" || key == "h" {
				m.applyConsentCursor = 0
			} else if key == "right" || key == "l" {
				m.applyConsentCursor = 1
			} else {
				m.applyConsentCursor = 1 - m.applyConsentCursor
			}
			m.applyConsent = m.applyConsentCursor == 0
			if m.applyConsent && m.applyConsentAt == "" {
				m.applyConsentAt = time.Now().Format(time.RFC3339)
			}
			if !m.applyConsent {
				m.applyConsentAt = ""
			}
			return m, m.saveCmd(), true
		}
	case fWorkAuth:
		switch key {
		case "left", "h":
			if m.workAuthCursor > 0 {
				m.workAuthCursor--
			}
			m.workAuth = workAuthOptions[m.workAuthCursor].value
			return m, m.saveCmd(), true
		case "right", "l", " ":
			if m.workAuthCursor < len(workAuthOptions)-1 {
				m.workAuthCursor++
			}
			m.workAuth = workAuthOptions[m.workAuthCursor].value
			return m, m.saveCmd(), true
		}
	case fCoverLetterMode:
		switch key {
		case "left", "h":
			if m.coverLetterCursor > 0 {
				m.coverLetterCursor--
			}
			m.coverLetterMode = coverLetterOptions[m.coverLetterCursor].value
			return m, m.saveCmd(), true
		case "right", "l", " ":
			if m.coverLetterCursor < len(coverLetterOptions)-1 {
				m.coverLetterCursor++
			}
			m.coverLetterMode = coverLetterOptions[m.coverLetterCursor].value
			return m, m.saveCmd(), true
		}
	}
	return m, nil, false
}

func (m FormModel) renderApplyConsent(active bool) string {
	yes := "Yes — I allow Nexus to submit applications"
	no := "No — queue only / manual"
	if m.applyConsentCursor == 0 {
		yes = "▶ " + yes
		no = "  " + no
	} else {
		yes = "  " + yes
		no = "▶ " + no
	}
	style := mutedStyle
	if active {
		style = primaryStyle
	}
	line := style.Render(yes+"   "+no)
	help := mutedStyle.Render("Required before Auto Apply. ←→ toggle")
	if m.applyConsent && m.applyConsentAt != "" {
		help = mutedStyle.Render("Consent recorded "+m.applyConsentAt+"  ·  ←→ toggle")
	}
	return line + "\n    " + help
}

func (m FormModel) renderWorkAuth(active bool) string {
	var parts []string
	for i, o := range workAuthOptions {
		label := o.label
		if i == m.workAuthCursor {
			label = "▶ " + label
			if active {
				label = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true).Render(label)
			} else {
				label = primaryStyle.Render(label)
			}
		} else {
			label = mutedStyle.Render("  " + label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}

func (m FormModel) renderCoverLetterMode(active bool) string {
	var parts []string
	for i, o := range coverLetterOptions {
		label := o.label
		if i == m.coverLetterCursor {
			label = "▶ " + label
			if active {
				label = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true).Render(label)
			} else {
				label = primaryStyle.Render(label)
			}
		} else {
			label = mutedStyle.Render("  " + label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}

func parsePositiveInt(s string, fallback int) int {
	s = strings.TrimSpace(s)
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func (m FormModel) applySafetySummary() string {
	consent := "no"
	if m.applyConsent {
		consent = "yes"
	}
	return fmt.Sprintf("consent=%s  %s/run  %s/day  delay %ss",
		consent,
		m.inputs[fMaxPerRun].Value(),
		m.inputs[fMaxPerDay].Value(),
		m.inputs[fApplyDelaySec].Value(),
	)
}
