package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/geo"
	"github.com/manthan8219/nexus-job-assistant/internal/localllm"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

// ── Field indices ─────────────────────────────────────────────────────────────

const (
	fFirstName = iota
	fLastName
	fEmail
	fPhone
	fLinkedInID
	fResumePath
	fCity
	fYearsExp
	fJobTitles        // tag input
	fWorkType         // checkbox
	fLocations        // tag input
	fCurrency         // cycle select
	fMinSalary        // preset picker
	fLinkedInKey      // provider key — LinkedIn
	fIndeedKey        // provider key — Indeed
	fAIAssist         // AI — master toggle: use AI to improve quality
	fAIProvider       // AI — local LLM vs cloud API keys
	fAnthropicKey     // AI — Claude
	fOpenAIKey        // AI — OpenAI
	fLocalLLMURL      // AI — local LLM endpoint
	fLocalLLMModel    // AI — local LLM model name
	fGmailPassword    // Outreach — Gmail app password (SMTP)
	fHunterKey        // Outreach — Hunter.io (find recruiter emails)
	fApolloKey        // Outreach — Apollo.io (contact database)
	fDiscordWebhook   // Integrations — Discord webhook URL
	fTelegramBotToken // Integrations — Telegram bot token
	fTelegramChatID   // Integrations — Telegram chat ID
	fNotifyChannels   // Integrations — notify channel selector (custom widget)
	fApplyConsent     // Apply Safety — consent to submit applications
	fMaxPerRun        // Apply Safety — max apps per engine run
	fMaxPerDay        // Apply Safety — max successful applies per day
	fApplyDelaySec    // Apply Safety — seconds between real applies
	fCompanyBlocklist // Apply Safety — comma-separated companies to skip
	fWorkAuth         // Apply Safety — work authorization
	fNoticePeriod     // Apply Safety — notice period, answers common screening questions
	fOfficeDays       // Apply Safety — days/week willing to work onsite
	fCoverLetterMode  // Apply Safety — off | template | ai
	fCoverLetterText  // Apply Safety — template body
	fScraperTargets   // Career Scraper — comma-separated company:url pairs
	fieldCount
)

var fieldLabels = [fieldCount]string{
	"First Name",
	"Last Name",
	"Email",
	"Phone",
	"LinkedIn ID",
	"Resume Path",
	"City",
	"Years of Experience",
	"Job Titles (AI)",
	"Work Type",
	"Target Locations",
	"Currency",
	"Min Salary",
	"LinkedIn API Token",
	"Indeed API Key",
	"Use AI Assist",
	"AI Backend",
	"Anthropic API Key",
	"OpenAI API Key",
	"Local LLM URL",
	"Local Model",
	"Gmail App Password",
	"Hunter.io API Key",
	"Apollo.io API Key",
	"Discord Webhook URL",
	"Telegram Bot Token",
	"Telegram Chat ID",
	"Notify on Apply",
	"Apply Consent",
	"Max Apps Per Run",
	"Max Apps Per Day",
	"Delay Between Applies (sec)",
	"Company Blocklist",
	"Work Authorization",
	"Notice Period",
	"Days/Week in Office",
	"Cover Letter",
	"Cover Letter Template",
	"Career Page Targets",
}

var fieldPlaceholders = [fieldCount]string{
	"John", "Doe", "john@example.com", "+1 555 000 0000",
	"johndoe", "/Users/john/resume.pdf", "San Francisco", "3",
	"describe the job you want…", // AI expands · enter generates when AI on
	"",                           // WorkType — custom widget
	"type city — pick from list", // geo autocomplete
	"",                           // Currency — custom widget
	"",                           // MinSalary — custom widget
	"paste token here",           // LinkedIn API token
	"paste key here",             // Indeed API key
	"",                           // AI Assist — custom yes/no
	"",                           // AI Backend — custom local/api
	"sk-ant-...",                 // Anthropic API key
	"sk-...",                     // OpenAI API key
	"http://localhost:11434",     // Local LLM URL
	"llama3",                     // Local LLM model name
	"xxxx xxxx xxxx xxxx",        // Gmail app password (16-char)
	"paste key here",             // Hunter.io API key
	"paste key here",             // Apollo.io API key
	"https://discord.com/api/webhooks/...",
	"1234567890:ABCDEFGabcdefg...", // from @BotFather
	"-1001234567890",               // chat/group/channel ID
	"",                             // notify channels — custom widget
	"",                             // apply consent — custom
	"10",
	"25",
	"3",
	"Acme Staffing, Example Corp",
	"",
	"30, or Immediate",
	"3",
	"",
	"I am excited to apply for…",
	"Stripe:https://stripe.com/jobs, Linear:https://linear.app/careers",
}

var formSections = []struct {
	title string
	start int
}{
	{"Personal Information", fFirstName},
	{"Job Preferences", fJobTitles},
	{"Provider Keys", fLinkedInKey},
	{"AI Configuration", fAIAssist},
	{"Outreach", fGmailPassword},
	{"Integrations", fDiscordWebhook},
	{"Apply Safety", fApplyConsent},
	{"Career Scraper", fScraperTargets},
}

// providerStatus describes a job board provider shown in the Providers section.
type providerStatus struct {
	name     string
	keyField int    // -1 = no key required (always active)
	cfgKey   string // key in config.ProviderKeys
}

// allProviders lists every provider and whether it needs a user key.
// Providers with keyField == -1 are always active (no user key needed).
var allProviders = []providerStatus{
	{"Greenhouse", -1, ""},
	{"Ashby", -1, ""},
	{"SmartRecruiters", -1, ""},
	{"Lever", -1, ""},
	{"Workable", -1, ""},
	{"RemoteOK", -1, ""},
	{"Remotive", -1, ""},
	{"Arbeitnow", -1, ""},
	{"Jobicy", -1, ""},
	{"HackerNews", -1, ""},
	{"Workday", -1, ""},
	{"LinkedIn", fLinkedInKey, "linkedin"},
	{"Indeed", fIndeedKey, "indeed"},
}

// ── Work type ─────────────────────────────────────────────────────────────────

var wtOptions = [3]string{"Remote", "Onsite", "Hybrid"}

// ── Currency + salary presets ─────────────────────────────────────────────────

type currencyDef struct {
	Code    string
	Symbol  string
	Presets []int
}

var currencies = []currencyDef{
	{"USD", "$", []int{60000, 80000, 100000, 120000, 150000, 200000}},
	{"INR", "₹", []int{500000, 800000, 1200000, 1500000, 2000000, 3000000}},
	{"EUR", "€", []int{50000, 70000, 90000, 110000, 130000, 160000}},
	{"GBP", "£", []int{50000, 70000, 90000, 110000, 130000, 150000}},
	{"CAD", "CA$", []int{70000, 90000, 110000, 130000, 150000, 180000}},
	{"AUD", "A$", []int{70000, 90000, 110000, 130000, 150000, 180000}},
	{"SGD", "S$", []int{80000, 100000, 120000, 150000, 180000, 220000}},
}

func formatSalary(c currencyDef, amount int) string {
	if c.Code == "INR" {
		// Show in lakhs
		l := float64(amount) / 100000
		if l == float64(int(l)) {
			return fmt.Sprintf("%s%.0fL", c.Symbol, l)
		}
		return fmt.Sprintf("%s%.1fL", c.Symbol, l)
	}
	// Show in thousands
	k := amount / 1000
	return fmt.Sprintf("%s%dk", c.Symbol, k)
}

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPurpleMuted)).
			MarginTop(1)

	labelActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorPurple)).
				Bold(true)

	labelInactiveStyle = lipgloss.NewStyle().
				Foreground(textSecondary)

	savedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGreen)).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorRed)).
			Bold(true)

	// Inline tag — uses text characters not lipgloss border (stays single-line)
	tagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPurple))

	tagRemoveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGrey))
)

// ── Messages ──────────────────────────────────────────────────────────────────

type SavedMsg struct{ Cfg *config.Config }
type ErrMsg struct{ err error }
type ProfileCompleteMsg struct{ Cfg *config.Config } // fired when all required fields are filled

// jobTitlesSuggestMsg is the async result of LLM job-title expansion.
type jobTitlesSuggestMsg struct {
	Gen    int
	Intent string
	Titles []string
	Err    error
}
type TestNotifyMsg struct{ Cfg *config.Config } // fired when user requests a test notification

// ResumeAnalysisStartMsg is fired when resume analysis begins (for the Resume tab).
type ResumeAnalysisStartMsg struct {
	Gen  int
	Path string
}

// ResumeAnalysisDoneMsg carries the result of async resume analysis.
// Gen matches resumeAnalysisGen so stale results from old paths are ignored.
type ResumeAnalysisDoneMsg struct {
	Gen       int
	Result    resume.Result
	Path      string // set when restoring from cache or after analyze
	FromCache bool
}

// resumeSpinnerTickMsg drives the spinner animation while analysis runs.
type resumeSpinnerTickMsg time.Time

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ── Form model ────────────────────────────────────────────────────────────────

type FormModel struct {
	inputs       [fieldCount]textinput.Model
	focused      int
	width        int
	height       int // body area rows — form scrolls when content exceeds this
	saved        bool
	err          error
	notifyBanner string // set after ctrl+t test; cleared on next key

	// WorkType checkbox
	wtSelected [3]bool
	wtCursor   int

	// AI Assist yes/no
	aiAssist       bool
	aiAssistCursor int // 0=Yes, 1=No

	// AI Backend: "local" | "api"
	aiProvider       string
	aiProviderCursor int // 0=local, 1=api

	// Local LLM picker (Ollama)
	llmMachine     localllm.Machine
	llmOptions     []localllm.Recommendation
	llmCursor      int
	llmInstalling  bool
	llmStatus      string
	llmOffline     bool // runtime not reachable — show setup menu
	llmSetupCursor int

	// Notify Channels checkbox — sized from notifier.Available() at init
	ncSelected []bool
	ncCursor   int

	// Tag inputs
	jobTitleTags        []string
	locationTags        []string
	jobIntent           string // free-text description of desired role
	jobTitlesSuggesting bool
	jobTitlesSuggestGen int
	jobTitlesSuggestErr string
	jobTitleCursor      int      // which title tag is highlighted (0-based)
	jobTitlesPending    []string // AI results awaiting add vs replace choice

	// Currency + salary
	currencyIdx  int    // index into currencies
	salaryPreset int    // index into presets (-1 = custom/unset)
	salaryCustom string // raw number string when user types a custom value

	// Resume analysis (async)
	resumeAnalyzing      bool
	resumeAnalysisDone   bool
	resumeAnalysisResult resume.Result
	resumeAnalysisGen    int // generation counter — stale msgs are ignored
	spinnerFrame         int
	lastAnalyzedPath     string // path we last analyzed / loaded from cache
	pendingResumeAnalyze bool   // true → InitCmd should run analysis

	// Path autocomplete
	acSuggestions []string
	acIdx         int // -1 = nothing highlighted

	// Nexus-generated resume library (PDFs under ~/.nexus/resumes)
	resumeLib      []resume.Version
	resumeLibIdx   int  // highlighted generated resume
	resumeLibFocus bool // true when navigating the generated list

	// Career Scraper
	scraperOffline       bool
	scraperInstalling    bool
	scraperStatus        string
	scraperSetupCursor   int
	scraperBackendCursor int
	scraperInstalled     []string

	// Apply Safety
	applyConsent       bool
	applyConsentCursor int // 0=Yes, 1=No
	applyConsentAt     string
	workAuth           string
	workAuthCursor     int
	coverLetterMode    string
	coverLetterCursor  int

	// Outreach (edited in Outreach tab; preserved on Config save)
	outreachConsent       bool
	outreachConsentAt     string
	maxEmailsPerDay       int
	maxLinkedInPerDay     int
	emailSubjectTpl       string
	emailBodyTpl          string
	linkedinMsgTpl        string
	outreachMode          string
	linkedinMode          string
	linkedinSessionCookie string
	// Outreach pipeline (edited in Outreach → Setup)
	outreachAutoQueue  bool
	outreachAICompose  bool
	outreachAIReview   bool
	outreachGenModel   string
	outreachCheckModel string
	outreachMinScore   int
	outreachMaxRetries int
	outreachSMTPVerify bool
	// Gmail OAuth (written by cmd/gmailauth or Outreach → Setup)
	gmailOAuthClientID     string
	gmailOAuthClientSecret string
	gmailOAuthRefreshToken string

	// Feature flags
	skipResumeCheck bool // when true, analysis is skipped and all fields stay unlocked
}

func NewFormModel(cfg *config.Config, skipResumeCheck bool) FormModel {
	m := FormModel{salaryPreset: -1, acIdx: -1, resumeLibIdx: 0, skipResumeCheck: skipResumeCheck}

	// Textinput defaults for non-custom fields
	linkedInKey := ""
	indeedKey := ""
	if cfg.ProviderKeys != nil {
		linkedInKey = cfg.ProviderKeys["linkedin"]
		indeedKey = cfg.ProviderKeys["indeed"]
	}
	textDefaults := [fieldCount]string{
		cfg.FirstName, cfg.LastName, cfg.Email, cfg.Phone,
		cfg.LinkedInID, cfg.ResumePath, cfg.City, cfg.YearsOfExperience,
		"", "", "", "", "",
		linkedInKey, indeedKey,
		"", // fAIAssist — custom widget
		"", // fAIProvider — custom widget
		cfg.AnthropicKey, cfg.OpenAIKey, cfg.LocalLLMURL, cfg.LocalLLMModel,
		cfg.GmailAppPassword, cfg.HunterKey, cfg.ApolloKey, cfg.DiscordWebhookURL,
		cfg.TelegramBotToken, cfg.TelegramChatID,
	}
	// fNotifyChannels, fApplyConsent, fMaxPerRun, fMaxPerDay, fApplyDelaySec,
	// fCompanyBlocklist, fWorkAuth, fCoverLetterMode, fCoverLetterText are handled separately.
	for i := 0; i < fieldCount; i++ {
		t := textinput.New()
		t.Prompt = "" // remove the "> " prefix
		t.Placeholder = fieldPlaceholders[i]
		t.SetValue(textDefaults[i])
		t.Width = 40
		if i == fEmail {
			t.CharLimit = 100
		}
		if i == fLinkedInKey || i == fIndeedKey || i == fAnthropicKey || i == fOpenAIKey ||
			i == fGmailPassword || i == fHunterKey || i == fApolloKey || i == fDiscordWebhook ||
			i == fTelegramBotToken {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}
		m.inputs[i] = t
	}

	// AI Assist + backend
	m.aiAssist = cfg.AIAssist
	if m.aiAssist {
		m.aiAssistCursor = 0
	} else {
		m.aiAssistCursor = 1
	}
	m.aiProvider = strings.ToLower(strings.TrimSpace(cfg.AIProvider))
	if m.aiProvider != "local" && m.aiProvider != "api" {
		m.aiProvider = ""
	}
	if m.aiProvider == "api" {
		m.aiProviderCursor = 1
	} else {
		m.aiProviderCursor = 0 // default highlight Local
	}
	m.llmMachine = localllm.ProbeMachine()
	m.refreshLLMOptions(nil)
	if m.aiAssist && m.aiProvider == "local" {
		if strings.TrimSpace(m.inputs[fLocalLLMURL].Value()) == "" {
			m.inputs[fLocalLLMURL].SetValue(localllm.DefaultURL)
		}
	}

	// WorkType
	for _, part := range strings.Split(cfg.WorkType, ",") {
		part = strings.TrimSpace(part)
		for j, opt := range wtOptions {
			if strings.EqualFold(part, opt) {
				m.wtSelected[j] = true
			}
		}
	}

	// Notify channels — discovered from notifier registry at runtime
	ncAvail := notifier.Available()
	m.ncSelected = make([]bool, len(ncAvail))
	if len(cfg.NotifyChannels) == 0 {
		for i := range m.ncSelected {
			m.ncSelected[i] = true
		}
	} else {
		for _, ch := range cfg.NotifyChannels {
			for j, opt := range ncAvail {
				if strings.EqualFold(ch, opt.ID) {
					m.ncSelected[j] = true
				}
			}
		}
	}

	// Tags
	m.jobTitleTags = parseTags(cfg.TargetJobTitles)
	m.jobIntent = strings.TrimSpace(cfg.JobIntent)
	m.locationTags = geo.ParseLocationTags(cfg.TargetLocations)

	// Currency
	m.currencyIdx = 0
	for i, c := range currencies {
		if c.Code == cfg.Currency {
			m.currencyIdx = i
			break
		}
	}

	// Salary — try preset match first, fall back to custom
	if cfg.MinSalary != "" {
		amount, err := strconv.Atoi(cfg.MinSalary)
		if err == nil {
			cur := currencies[m.currencyIdx]
			matched := false
			for i, p := range cur.Presets {
				if p == amount {
					m.salaryPreset = i
					matched = true
					break
				}
			}
			if !matched {
				m.salaryCustom = cfg.MinSalary
			}
		}
	}

	m.inputs[m.focused].Focus()

	// Career Scraper — check if the service is running on load.
	m.scraperOffline = !scraper.Running()
	if scraper.Installed() {
		m.scraperInstalled = scraper.InstalledBackends()
	}

	// Restore cached analysis when possible — do not re-run AI on every launch.
	if !skipResumeCheck {
		path := strings.TrimSpace(cfg.ResumePath)
		if path != "" {
			if cached, ok := resume.LoadFreshCache(path, m.aiAssist); ok {
				m.resumeAnalysisDone = true
				m.resumeAnalysisResult = cached.Result
				m.lastAnalyzedPath = path
				m.resumeAnalyzing = false
			} else {
				m.pendingResumeAnalyze = true
				m.resumeAnalyzing = true // show spinner until InitCmd finishes
			}
		}
	}
	m.initApplySafetyFromCfg(
		cfg.ApplyConsent, cfg.ApplyConsentAt,
		cfg.MaxAppsPerRun, cfg.MaxAppsPerDay, cfg.ApplyDelaySec,
		cfg.CompanyBlocklist, cfg.WorkAuth, cfg.NoticePeriodDays, cfg.OfficeDaysPerWeek,
		cfg.CoverLetterMode, cfg.CoverLetterText,
	)
	m.initOutreachFromCfg(cfg)
	m.inputs[fCoverLetterText].CharLimit = 4000
	m.inputs[fCompanyBlocklist].CharLimit = 500
	m.loadResumeLibrary()
	return m
}

// InitCmd returns commands that should run when the app starts —
// called from AppModel.Init() since sub-models don't get their own Init() call.
func (m FormModel) InitCmd() tea.Cmd {
	var cmds []tea.Cmd
	if m.aiAssist && m.aiProvider == "local" {
		cmds = append(cmds, refreshLocalLLMCmd(m.inputs[fLocalLLMURL].Value()))
	}
	// Hydrate Resume tab from cache without calling the LLM.
	if m.resumeAnalysisDone {
		res := m.resumeAnalysisResult
		path := m.lastAnalyzedPath
		gen := m.resumeAnalysisGen
		cmds = append(cmds, func() tea.Msg {
			return ResumeAnalysisDoneMsg{Gen: gen, Result: res, Path: path, FromCache: true}
		})
	} else if m.pendingResumeAnalyze && !m.skipResumeCheck {
		path := strings.TrimSpace(m.inputs[fResumePath].Value())
		if path != "" {
			cmds = append(cmds,
				resumeAnalysisStartCmd(path, m.resumeAnalysisGen),
				analyzeResumeCmd(path, m.resumeAnalysisGen, m.aiOptions()),
				resumeSpinnerTickCmd(),
			)
		}
	}
	return tea.Batch(cmds...)
}

// ConsumesEscape is true when Escape should stay inside Config (dialogs / autocomplete),
// not jump to app chrome tab-navigation mode.
func (m FormModel) ConsumesEscape() bool {
	if len(m.jobTitlesPending) > 0 {
		return true
	}
	if m.focused == fResumePath && len(m.acSuggestions) > 0 {
		return true
	}
	if m.focused == fLocations && len(m.acSuggestions) > 0 {
		return true
	}
	if m.focused == fMinSalary && m.salaryCustom != "" {
		return true
	}
	return false
}

// CustomFieldActive returns true when the focused field uses left/right internally.
// Used by app.go to avoid stealing those keys for tab switching.
func (m FormModel) CustomFieldActive() bool {
	switch m.focused {
	case fWorkType, fCurrency, fMinSalary, fNotifyChannels, fAIAssist, fAIProvider, fApplyConsent, fWorkAuth, fCoverLetterMode:
		return true
	case fJobTitles, fLocations:
		// Description / tag fields need ←→ for cursor movement while typing.
		return true
	}
	// Other text inputs: keep ←→ for cursor once you're editing text.
	if !isCustomField(m.focused) {
		ti := m.inputs[m.focused]
		return ti.Value() != "" || ti.Position() > 0
	}
	return false
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m FormModel) Init() tea.Cmd { return textinput.Blink }

func (m FormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		m.saved = false
		m.err = nil
		m.notifyBanner = ""
		return m.handleKey(msg)

	case SavedMsg:
		m.saved = true
		return m, nil
	case ErrMsg:
		m.err = msg.err
		return m, nil

	case jobTitlesSuggestMsg:
		if msg.Gen != m.jobTitlesSuggestGen {
			return m, nil
		}
		m.jobTitlesSuggesting = false
		if msg.Err != nil {
			m.jobTitlesSuggestErr = msg.Err.Error()
			m.err = msg.Err
			return m, nil
		}
		m.jobTitlesSuggestErr = ""
		m.err = nil
		if strings.TrimSpace(msg.Intent) != "" {
			m.jobIntent = strings.TrimSpace(msg.Intent)
		}
		m.inputs[fJobTitles].SetValue("")
		// No existing titles → just use AI list.
		if len(m.jobTitleTags) == 0 {
			m.jobTitleTags = mergeJobTitleTags(nil, msg.Titles)
			m.jobTitleCursor = 0
			m.jobTitlesPending = nil
			m.notifyBanner = fmt.Sprintf("✓ Set %d job titles from your description", len(msg.Titles))
			return m, m.saveCmd()
		}
		// Ask add vs replace.
		m.jobTitlesPending = mergeJobTitleTags(nil, msg.Titles)
		m.notifyBanner = ""
		m.focused = fJobTitles
		return m, nil

	case ResumeAnalysisDoneMsg:
		if msg.Gen == m.resumeAnalysisGen {
			m.resumeAnalyzing = false
			m.resumeAnalysisDone = true
			m.resumeAnalysisResult = msg.Result
			m.pendingResumeAnalyze = false
			path := msg.Path
			if path == "" {
				path = strings.TrimSpace(m.inputs[fResumePath].Value())
			}
			m.lastAnalyzedPath = path
			if !msg.FromCache && path != "" {
				_ = resume.SaveCache(path, m.aiAssist, msg.Result)
			}
			// Re-save so IsComplete() re-evaluates with the new analysis result.
			return m, m.saveCmd()
		}
		return m, nil

	case resumeSpinnerTickMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		if m.resumeAnalyzing || m.llmInstalling || m.jobTitlesSuggesting {
			return m, resumeSpinnerTickCmd()
		}
		return m, nil

	case localLLMStatusMsg:
		if msg.Err != nil {
			m.llmOffline = true
			m.llmStatus = msg.Err.Error()
			m.refreshLLMOptions(nil)
		} else {
			m.llmOffline = false
			m.llmStatus = ""
			m.refreshLLMOptions(msg.Installed)
		}
		return m, nil

	case localLLMPullDoneMsg:
		m.llmInstalling = false
		if msg.Err != nil {
			m.err = msg.Err
			m.llmStatus = msg.Err.Error()
			return m, nil
		}
		m.inputs[fLocalLLMModel].SetValue(msg.Model)
		m.llmStatus = "installed " + msg.Model
		m.notifyBanner = "✓ Local model ready: " + msg.Model
		return m, tea.Batch(m.saveCmd(), refreshLocalLLMCmd(m.inputs[fLocalLLMURL].Value()))

	case scraperStatusMsg:
		m.scraperInstalling = false
		if msg.err != nil {
			m.scraperStatus = msg.err.Error()
			m.scraperOffline = true
		} else {
			m.scraperOffline = !msg.running
			if msg.installed != nil {
				m.scraperInstalled = msg.installed
			}
			if msg.running {
				m.scraperStatus = "ready"
			} else {
				m.scraperStatus = "failed to start"
			}
		}
		return m, nil
	}

	// Forward to active textinput — auto-save on every edit
	if !isCustomField(m.focused) {
		var cmd tea.Cmd
		m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
		return m, tea.Batch(cmd, m.saveCmd())
	}
	return m, nil
}

func (m FormModel) handleKey(msg tea.KeyMsg) (FormModel, tea.Cmd) {
	key := msg.String()

	// ── Tag input special keys ──────────────────────────────────────────────
	if m.focused == fJobTitles {
		// Pending AI titles: choose add vs replace before anything else.
		if len(m.jobTitlesPending) > 0 {
			switch key {
			case "a", "A":
				n := len(m.jobTitlesPending)
				m.jobTitleTags = mergeJobTitleTags(m.jobTitleTags, m.jobTitlesPending)
				m.jobTitlesPending = nil
				m.jobTitleCursor = 0
				m.notifyBanner = fmt.Sprintf("✓ Added %d titles (kept existing)", n)
				return m, m.saveCmd()
			case "r", "R":
				n := len(m.jobTitlesPending)
				m.jobTitleTags = append([]string(nil), m.jobTitlesPending...)
				m.jobTitlesPending = nil
				m.jobTitleCursor = 0
				m.notifyBanner = fmt.Sprintf("✓ Replaced with %d new titles", n)
				return m, m.saveCmd()
			case "esc", "n", "N":
				m.jobTitlesPending = nil
				m.notifyBanner = "Discarded AI titles"
				return m, nil
			default:
				return m, nil // ignore other keys until chosen
			}
		}
		switch key {
		case "ctrl+g":
			return m.startJobTitlesSuggest(strings.TrimSpace(m.inputs[fJobTitles].Value()))
		case "up", "k", "left", "h":
			// Vertical list: ↑/↓ move among titles when not typing.
			if m.inputs[fJobTitles].Value() == "" && len(m.jobTitleTags) > 0 {
				m.clampJobTitleCursor()
				if m.jobTitleCursor > 0 {
					m.jobTitleCursor--
					return m, nil
				}
				// First title: ↑ leaves field; hj/← stay put (don't type into input).
				if key == "up" {
					break // fall through to previous-field nav
				}
				return m, nil
			}
		case "down", "j", "right", "l":
			if m.inputs[fJobTitles].Value() == "" && len(m.jobTitleTags) > 0 {
				m.clampJobTitleCursor()
				if m.jobTitleCursor < len(m.jobTitleTags)-1 {
					m.jobTitleCursor++
					return m, nil
				}
				if key == "down" {
					break // fall through to next-field nav
				}
				return m, nil
			}
		case "enter":
			val := strings.TrimSpace(m.inputs[fJobTitles].Value())
			if val != "" {
				// Enter always adds the literal text as typed — ctrl+g is the
				// explicit "expand this via AI" action. (Enter used to hand
				// off to AI expansion here when AI Assist was on, with
				// ctrl+enter as the only literal-add escape hatch — but most
				// terminals don't send a distinct sequence for ctrl+enter, so
				// that path silently never fired for most users.)
				m.jobTitleTags = mergeJobTitleTags(m.jobTitleTags, []string{val})
				m.inputs[fJobTitles].SetValue("")
				m.jobTitleCursor = len(m.jobTitleTags) - 1
				return m, m.saveCmd()
			}
			// Empty enter with saved intent + AI → regenerate
			if m.aiAssist && strings.TrimSpace(m.jobIntent) != "" {
				return m.startJobTitlesSuggest("")
			}
			// Empty → fall through to next field
		case "backspace", "x", "delete":
			if m.inputs[fJobTitles].Value() == "" && len(m.jobTitleTags) > 0 {
				m.clampJobTitleCursor()
				idx := m.jobTitleCursor
				m.jobTitleTags = append(m.jobTitleTags[:idx], m.jobTitleTags[idx+1:]...)
				m.clampJobTitleCursor()
				return m, m.saveCmd()
			}
		}
	}
	if m.focused == fLocations {
		// Autocomplete navigation for city picker
		if len(m.acSuggestions) > 0 {
			switch key {
			case "down", "ctrl+n":
				if m.acIdx < len(m.acSuggestions)-1 {
					m.acIdx++
				}
				return m, nil
			case "up", "ctrl+p":
				if m.acIdx > 0 {
					m.acIdx--
				}
				return m, nil
			case "esc":
				m.acSuggestions = nil
				m.acIdx = -1
				return m, nil
			case "tab", "enter":
				idx := m.acIdx
				if idx < 0 {
					idx = 0
				}
				sel := m.acSuggestions[idx]
				if m.addLocationTag(sel) {
					m.inputs[fLocations].SetValue("")
					m.acSuggestions = nil
					m.acIdx = -1
					return m, m.saveCmd()
				}
				return m, nil
			}
		}
		switch key {
		case "enter", "tab":
			val := strings.TrimSpace(m.inputs[fLocations].Value())
			if val == "" {
				break // fall through to field nav
			}
			if m.addLocationTag(val) {
				m.inputs[fLocations].SetValue("")
				m.acSuggestions = nil
				m.acIdx = -1
				return m, m.saveCmd()
			}
			// Not in index — keep typing; refresh suggestions
			m.updateLocationAC(val)
			return m, nil
		case "backspace":
			if m.inputs[fLocations].Value() == "" && len(m.locationTags) > 0 {
				m.locationTags = m.locationTags[:len(m.locationTags)-1]
				return m, m.saveCmd()
			}
		}
	}

	// ── AI Assist yes/no ─────────────────────────────────────────────────
	if m.focused == fAIAssist {
		switch key {
		case "left", "h":
			m.aiAssistCursor = 0
			m.applyAIAssistChoice()
			cmd := m.saveCmd()
			if m.needsAIProfile() {
				path := strings.TrimSpace(m.inputs[fResumePath].Value())
				var c tea.Cmd
				m, c = m.startResumeAnalysis(path)
				return m, tea.Batch(cmd, c)
			}
			return m, cmd
		case "right", "l":
			m.aiAssistCursor = 1
			m.applyAIAssistChoice()
			return m, m.saveCmd()
		case " ", "enter":
			m.applyAIAssistChoice()
			cmd := m.saveCmd()
			if m.needsAIProfile() {
				path := strings.TrimSpace(m.inputs[fResumePath].Value())
				var c tea.Cmd
				m, c = m.startResumeAnalysis(path)
				return m, tea.Batch(cmd, c)
			}
			return m, cmd
		case "tab", "down", "shift+tab", "up":
			m.applyAIAssistChoice()
			// fall through to nav; analyze after nav if needed
		default:
			return m, nil
		}
	}

	// ── AI Backend local / api ───────────────────────────────────────────
	if m.focused == fAIProvider {
		switch key {
		case "left", "h":
			m.aiProviderCursor = 0
			m.applyAIProviderChoice()
			cmd := m.saveCmd()
			if m.aiProvider == "local" {
				return m, tea.Batch(cmd, refreshLocalLLMCmd(m.inputs[fLocalLLMURL].Value()))
			}
			return m, cmd
		case "right", "l":
			m.aiProviderCursor = 1
			m.applyAIProviderChoice()
			return m, m.saveCmd()
		case " ", "enter":
			m.applyAIProviderChoice()
			cmd := m.saveCmd()
			if m.aiProvider == "local" {
				return m, tea.Batch(cmd, refreshLocalLLMCmd(m.inputs[fLocalLLMURL].Value()))
			}
			return m, cmd
		case "tab", "down", "shift+tab", "up":
			m.applyAIProviderChoice()
			// fall through to nav (refresh kicked after nav if local — see below)
		default:
			return m, nil
		}
	}

	// ── Local LLM model picker / offline setup ───────────────────────────
	if m.focused == fLocalLLMModel {
		switch key {
		case "tab":
			m.focused = m.nextVisibleField(m.focused, +1)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			return m, tea.Batch(textinput.Blink, m.saveCmd())
		case "shift+tab", "esc":
			m.focused = m.nextVisibleField(m.focused, -1)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			return m, tea.Batch(textinput.Blink, m.saveCmd())
		default:
			if m.llmOffline {
				return m.handleLLMSetupKey(key)
			}
			return m.handleLLMPickerKey(key)
		}
	}

	// ── Career Scraper setup / targets ────────────────────────────────────
	if m.focused == fScraperTargets {
		venvReady := scraper.Installed()
		// When running: always show catalog so user can install more backends
		if !m.scraperOffline {
			venvReady = true
		}
		if !venvReady {
			// navigate the setup-action menu (install venv / start / retry)
			opts := scraper.SetupOptions()
			n := len(opts)
			switch key {
			case "up", "k":
				if m.scraperSetupCursor <= 0 {
					m.focused = m.nextVisibleField(m.focused, -1)
					if !isCustomField(m.focused) {
						m.inputs[m.focused].Focus()
					}
					return m, tea.Batch(textinput.Blink, m.saveCmd())
				}
				m.scraperSetupCursor--
				return m, nil
			case "down", "j":
				if m.scraperSetupCursor >= n-1 {
					m.focused = m.nextVisibleField(m.focused, +1)
					if !isCustomField(m.focused) {
						m.inputs[m.focused].Focus()
					}
					return m, tea.Batch(textinput.Blink, m.saveCmd())
				}
				m.scraperSetupCursor++
				return m, nil
			case " ", "enter":
				return m, m.runScraperSetupOption(opts[m.scraperSetupCursor])
			}
		} else {
			// navigate the backend catalog picker
			n := len(scraper.Catalog)
			switch key {
			case "up", "k":
				if m.scraperBackendCursor <= 0 {
					m.focused = m.nextVisibleField(m.focused, -1)
					if !isCustomField(m.focused) {
						m.inputs[m.focused].Focus()
					}
					return m, tea.Batch(textinput.Blink, m.saveCmd())
				}
				m.scraperBackendCursor--
				return m, nil
			case "down", "j":
				if m.scraperBackendCursor >= n-1 {
					m.focused = m.nextVisibleField(m.focused, +1)
					if !isCustomField(m.focused) {
						m.inputs[m.focused].Focus()
					}
					return m, tea.Batch(textinput.Blink, m.saveCmd())
				}
				m.scraperBackendCursor++
				return m, nil
			case " ", "enter":
				b := scraper.Catalog[m.scraperBackendCursor]
				m.scraperInstalling = true
				m.scraperStatus = "Installing " + b.Name + "..."
				return m, func() tea.Msg {
					err := scraper.InstallBackend(context.Background(), b, nil)
					if err != nil {
						return scraperStatusMsg{err: err}
					}
					_ = scraper.Start("", "")
					_ = scraper.WaitReady(20 * time.Second)
					return scraperStatusMsg{running: scraper.Running(), installed: scraper.InstalledBackends()}
				}
			}
		}
		// shared tab/shift-tab for both sub-menus
		switch key {
		case " ", "enter":
			// already handled above — dummy
		case "tab":
			m.focused = m.nextVisibleField(m.focused, +1)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			return m, tea.Batch(textinput.Blink, m.saveCmd())
		case "shift+tab", "esc":
			m.focused = m.nextVisibleField(m.focused, -1)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			return m, tea.Batch(textinput.Blink, m.saveCmd())
		default:
			return m, nil
		}
	}

	// ── WorkType checkbox ──────────────────────────────────────────────────
	if m.focused == fWorkType {
		switch key {
		case "left", "h":
			m.wtCursor = (m.wtCursor - 1 + 3) % 3
			return m, nil
		case "right", "l":
			m.wtCursor = (m.wtCursor + 1) % 3
			return m, nil
		case " ", "enter":
			m.wtSelected[m.wtCursor] = !m.wtSelected[m.wtCursor]
			return m, m.saveCmd()
		case "tab", "down", "shift+tab", "up":
			// fall through to nav
		default:
			return m, nil
		}
	}

	// ── Notify Channels checkbox ─────────────────────────────────────────
	if m.focused == fNotifyChannels {
		n := len(m.ncSelected)
		if n == 0 {
			switch key {
			case "tab", "down", "shift+tab", "up", "enter":
				// fall through to nav
			default:
				return m, nil
			}
		} else {
			switch key {
			case "left", "h":
				m.ncCursor = (m.ncCursor - 1 + n) % n
				return m, nil
			case "right", "l":
				m.ncCursor = (m.ncCursor + 1) % n
				return m, nil
			case " ", "enter":
				m.ncSelected[m.ncCursor] = !m.ncSelected[m.ncCursor]
				return m, m.saveCmd()
			case "tab", "down", "shift+tab", "up":
				// fall through to nav
			default:
				return m, nil
			}
		}
	}

	// ── Currency selector ──────────────────────────────────────────────────
	if m.focused == fCurrency {
		switch key {
		case "left", "h":
			m.currencyIdx = (m.currencyIdx - 1 + len(currencies)) % len(currencies)
			m.salaryPreset = 0
			m.salaryCustom = ""
			return m, m.saveCmd()
		case "right", "l":
			m.currencyIdx = (m.currencyIdx + 1) % len(currencies)
			m.salaryPreset = 0
			m.salaryCustom = ""
			return m, m.saveCmd()
		case "tab", "down", "shift+tab", "up", "enter":
			// fall through to nav
		default:
			return m, nil
		}
	}

	// ── Salary picker (presets + custom typing) ────────────────────────────
	if m.focused == fMinSalary {
		presets := currencies[m.currencyIdx].Presets
		switch key {
		case "left", "h":
			if m.salaryCustom == "" {
				if m.salaryPreset > 0 {
					m.salaryPreset--
				}
				return m, m.saveCmd()
			}
			return m, nil
		case "right", "l":
			if m.salaryCustom == "" {
				if m.salaryPreset < len(presets)-1 {
					m.salaryPreset++
				}
				return m, m.saveCmd()
			}
			return m, nil
		case "backspace":
			if m.salaryCustom != "" {
				m.salaryCustom = m.salaryCustom[:len(m.salaryCustom)-1]
			}
			return m, m.saveCmd()
		case "esc":
			// Cancel custom input, revert to first preset
			m.salaryCustom = ""
			if m.salaryPreset < 0 {
				m.salaryPreset = 0
			}
			return m, m.saveCmd()
		case "tab", "down", "shift+tab", "up", "enter":
			// fall through to nav
		default:
			// Digit → accumulate into custom value
			if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
				m.salaryCustom += key
				m.salaryPreset = -1
				return m, m.saveCmd()
			}
			return m, nil
		}
	}

	// ── Apply Safety custom widgets ────────────────────────────────────────
	if m2, cmd, ok := m.updateApplySafetyKeys(key); ok {
		return m2, cmd
	}

	// ── Nexus-generated resume library (ctrl+j/k · enter to use) ───────────
	if m.focused == fResumePath {
		m.loadResumeLibrary()
	}
	if m.focused == fResumePath && len(m.resumeLib) > 0 {
		switch key {
		case "ctrl+j":
			m.resumeLibFocus = true
			m.acSuggestions = nil
			m.acIdx = -1
			if m.resumeLibIdx < len(m.resumeLib)-1 {
				m.resumeLibIdx++
			}
			return m, nil
		case "ctrl+k":
			m.resumeLibFocus = true
			m.acSuggestions = nil
			m.acIdx = -1
			if m.resumeLibIdx > 0 {
				m.resumeLibIdx--
			}
			return m, nil
		case "enter":
			if m.resumeLibFocus {
				return m.applyResumeLibrarySelection()
			}
		}
	}

	// ── Autocomplete navigation (resume path only) ────────────────────────
	if m.focused == fResumePath && len(m.acSuggestions) > 0 {
		switch key {
		case "down", "ctrl+n":
			if m.acIdx < len(m.acSuggestions)-1 {
				m.acIdx++
			}
			return m, nil
		case "up", "ctrl+p":
			if m.acIdx > 0 {
				m.acIdx--
			}
			return m, nil
		case "esc":
			m.acSuggestions = nil
			m.acIdx = -1
			return m, nil
		case "tab", "enter":
			// Select highlighted item, or the first suggestion if none highlighted yet.
			idx := m.acIdx
			if idx < 0 {
				idx = 0
			}
			sel := m.acSuggestions[idx]
			m.inputs[fResumePath].SetValue(sel)
			m.acSuggestions = nil
			m.acIdx = -1
			if strings.HasSuffix(sel, "/") {
				// Directory — re-expand so the user can keep drilling down.
				m.updateAC(sel)
				return m, nil
			}
			// File selected — blur field, analyze only if path changed, move next.
			m.inputs[fResumePath].Blur()
			m.focused = m.nextVisibleField(fResumePath, +1)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			cmds := []tea.Cmd{textinput.Blink, m.saveCmd()}
			if !m.skipResumeCheck && m.resumePathChanged(sel) {
				var c tea.Cmd
				m, c = m.startResumeAnalysis(sel)
				cmds = append(cmds, c)
			}
			return m, tea.Batch(cmds...)
		}
	}

	// ── Resume path — trigger async analysis on blur ──────────────────────
	if m.focused == fResumePath {
		switch key {
		case "tab", "down", "shift+tab", "up", "enter":
			// Dismiss autocomplete on blur.
			m.acSuggestions = nil
			m.acIdx = -1
			path := strings.TrimSpace(m.inputs[fResumePath].Value())
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Blur()
			}
			dir := +1
			if key == "shift+tab" || key == "up" {
				dir = -1
			}
			m.focused = m.nextVisibleField(fResumePath, dir)
			if !isCustomField(m.focused) {
				m.inputs[m.focused].Focus()
			}
			cmds := []tea.Cmd{textinput.Blink, m.saveCmd()}
			if path != "" && !m.skipResumeCheck && m.resumePathChanged(path) {
				var c tea.Cmd
				m, c = m.startResumeAnalysis(path)
				cmds = append(cmds, c)
			} else if path == "" {
				m.resumeAnalyzing = false
				m.resumeAnalysisDone = false
				m.lastAnalyzedPath = ""
			}
			return m, tea.Batch(cmds...)
		}
	}

	// ── Field navigation ───────────────────────────────────────────────────
	switch key {
	case "tab", "enter", "down":
		if !isCustomField(m.focused) {
			m.inputs[m.focused].Blur()
		}
		m.focused = m.nextVisibleField(m.focused, +1)
		if !isCustomField(m.focused) {
			m.inputs[m.focused].Focus()
		}
		cmds := []tea.Cmd{textinput.Blink, m.saveCmd()}
		if m.aiAssist && m.aiProvider == "local" {
			cmds = append(cmds, refreshLocalLLMCmd(m.inputs[fLocalLLMURL].Value()))
		}
		if m.needsAIProfile() {
			path := strings.TrimSpace(m.inputs[fResumePath].Value())
			var c tea.Cmd
			m, c = m.startResumeAnalysis(path)
			cmds = append(cmds, c)
		}
		return m, tea.Batch(cmds...)

	case "shift+tab", "up":
		if !isCustomField(m.focused) {
			m.inputs[m.focused].Blur()
		}
		m.focused = m.nextVisibleField(m.focused, -1)
		if !isCustomField(m.focused) {
			m.inputs[m.focused].Focus()
		}
		return m, tea.Batch(textinput.Blink, m.saveCmd())
	}

	// ctrl+x clears a masked/token field
	if key == "ctrl+x" && isMaskedField(m.focused) {
		m.inputs[m.focused].Reset()
		return m, tea.Batch(textinput.Blink, m.saveCmd())
	}

	// ctrl+t sends a test notification for Discord/Telegram fields
	if key == "ctrl+t" && isNotifyField(m.focused) {
		return m, func() tea.Msg { return TestNotifyMsg{Cfg: m.toConfig()} }
	}

	// Forward to textinput — auto-save as you type (no ctrl+s)
	if !isCustomField(m.focused) {
		var cmd tea.Cmd
		m.inputs[m.focused], cmd = m.inputs[m.focused].Update(tea.KeyMsg(msg))
		if m.focused == fResumePath {
			m.resumeLibFocus = false
			m.updateAC(m.inputs[fResumePath].Value())
		}
		if m.focused == fLocations {
			m.updateLocationAC(m.inputs[fLocations].Value())
		}
		return m, tea.Batch(cmd, m.saveCmd())
	}
	return m, nil
}

func isCustomField(f int) bool {
	return f == fWorkType || f == fCurrency || f == fMinSalary || f == fNotifyChannels || f == fAIAssist || f == fAIProvider || f == fLocalLLMModel || f == fApplyConsent || f == fWorkAuth || f == fCoverLetterMode
}

func (m *FormModel) applyAIAssistChoice() {
	m.aiAssist = m.aiAssistCursor == 0
	if !m.aiAssist {
		return
	}
	// Enabling AI — default to local LLM if no backend chosen yet.
	if m.aiProvider != "local" && m.aiProvider != "api" {
		m.aiProvider = "local"
		m.aiProviderCursor = 0
	}
}

// needsAIProfile is true when Assist is on but we have no usable cached profile.
func (m FormModel) needsAIProfile() bool {
	if !m.aiAssist {
		return false
	}
	p := m.resumeAnalysisResult.Profile
	return p == nil || p.Summary == ""
}

func (m *FormModel) applyAIProviderChoice() {
	if m.aiProviderCursor == 0 {
		m.aiProvider = "local"
		if strings.TrimSpace(m.inputs[fLocalLLMURL].Value()) == "" {
			m.inputs[fLocalLLMURL].SetValue(localllm.DefaultURL)
		}
	} else {
		m.aiProvider = "api"
	}
}

// fieldVisible returns false for AI credential fields that don't apply
// given the current Use AI Assist / AI Backend choices.
func (m FormModel) fieldVisible(f int) bool {
	switch f {
	case fAIProvider:
		return m.aiAssist
	case fAnthropicKey, fOpenAIKey:
		return m.aiAssist && m.aiProvider == "api"
	case fLocalLLMURL, fLocalLLMModel:
		return m.aiAssist && m.aiProvider == "local"
	case fCoverLetterText:
		return m.coverLetterMode == "template"
	default:
		return true
	}
}

// nextVisibleField walks dir (+1/-1) until a visible, unlocked field is found.
func (m FormModel) nextVisibleField(from, dir int) int {
	next := from
	for i := 0; i < fieldCount; i++ {
		next = (next + dir + fieldCount) % fieldCount
		if !m.fieldVisible(next) {
			continue
		}
		if m.resumeInvalid() && isLockedByResume(next) {
			return fResumePath
		}
		return next
	}
	return from
}

// maskDots returns a string of n bullet characters, capped at a visible max.
func maskDots(n int) string {
	if n <= 0 {
		return "—"
	}
	const max = 32
	if n > max {
		n = max
	}
	dots := make([]rune, n)
	for i := range dots {
		dots[i] = '•'
	}
	return string(dots)
}

func isMaskedField(f int) bool {
	return f == fLinkedInKey || f == fIndeedKey || f == fAnthropicKey || f == fOpenAIKey ||
		f == fGmailPassword || f == fHunterKey || f == fApolloKey ||
		f == fDiscordWebhook || f == fTelegramBotToken
}

func isNotifyField(f int) bool {
	return f == fDiscordWebhook || f == fTelegramBotToken || f == fTelegramChatID || f == fNotifyChannels
}

// resumeInvalid returns true when analysis finished and the file is not a valid resume.
// While still analyzing (or not yet triggered) this returns false so fields stay open.
// Always returns false when skipResumeCheck is enabled.
func (m FormModel) resumeInvalid() bool {
	return !m.skipResumeCheck && m.resumeAnalysisDone && !m.resumeAnalysisResult.Valid
}

// isLockedByResume returns true for fields that should be inaccessible when the
// resume file is invalid. Personal info and provider key fields stay open.
func isLockedByResume(f int) bool {
	return f > fResumePath && f <= fMinSalary
}

// updateAC recomputes filesystem autocomplete suggestions for the resume path field.
// It lists .pdf/.docx/.doc files and directories that match the current prefix.
func (m *FormModel) loadResumeLibrary() {
	vers, err := resume.ListVersions()
	if err != nil {
		m.resumeLib = nil
		return
	}
	m.resumeLib = vers
	if m.resumeLibIdx >= len(m.resumeLib) {
		m.resumeLibIdx = max(0, len(m.resumeLib)-1)
	}
}

func (m FormModel) applyResumeLibrarySelection() (FormModel, tea.Cmd) {
	if m.resumeLibIdx < 0 || m.resumeLibIdx >= len(m.resumeLib) {
		return m, nil
	}
	sel := m.resumeLib[m.resumeLibIdx].PDFPath
	m.inputs[fResumePath].SetValue(sel)
	m.resumeLibFocus = false
	m.acSuggestions = nil
	m.acIdx = -1
	m.inputs[fResumePath].Blur()
	m.focused = m.nextVisibleField(fResumePath, +1)
	if !isCustomField(m.focused) {
		m.inputs[m.focused].Focus()
	}
	cmds := []tea.Cmd{textinput.Blink, m.saveCmd()}
	if !m.skipResumeCheck && m.resumePathChanged(sel) {
		var c tea.Cmd
		m, c = m.startResumeAnalysis(sel)
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

func (m FormModel) renderResumeLibrary() string {
	// Always read disk so Config stays in sync after Resume → New resume.
	vers, err := resume.ListVersions()
	if err != nil || len(vers) == 0 {
		return "\n    " + mutedStyle.Render("No Nexus PDFs yet — generate one under Resume → New resume")
	}
	idx := m.resumeLibIdx
	if idx < 0 || idx >= len(vers) {
		idx = 0
	}
	var b strings.Builder
	b.WriteString("\n    " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render(
		fmt.Sprintf("Nexus generated (%d)  ·  ctrl+j/k pick  ·  enter use  ·  or type your own path", len(vers)),
	))
	limit := len(vers)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		v := vers[i]
		line := v.DisplayLine()
		prefix := "  "
		style := mutedStyle
		if i == idx && m.resumeLibFocus {
			prefix = "▶ "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Bold(true)
		} else if i == idx {
			prefix = "· "
			style = primaryStyle
		}
		b.WriteString("\n    " + style.Render(prefix+line))
	}
	if len(vers) > limit {
		b.WriteString("\n    " + mutedStyle.Render(fmt.Sprintf("  … +%d more in ~/.nexus/resumes/", len(vers)-limit)))
	}
	return b.String()
}

func (m *FormModel) updateAC(input string) {
	if input == "" {
		m.acSuggestions = nil
		m.acIdx = -1
		return
	}

	// Expand leading ~
	expanded := input
	if input == "~" || input == "~/" {
		home, _ := os.UserHomeDir()
		expanded = home + "/"
	} else if strings.HasPrefix(input, "~/") {
		home, _ := os.UserHomeDir()
		expanded = filepath.Join(home, input[2:])
		if strings.HasSuffix(input, "/") {
			expanded += "/"
		}
	}

	// Split into the directory to list and the prefix to filter by.
	var dir, prefix string
	if strings.HasSuffix(expanded, "/") {
		dir = expanded
		prefix = ""
	} else {
		dir = filepath.Dir(expanded)
		prefix = filepath.Base(expanded)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		m.acSuggestions = nil
		return
	}

	var suggestions []string
	for _, e := range entries {
		name := e.Name()
		// Skip hidden files unless the user explicitly typed a dot.
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}
		full := filepath.Join(dir, name)
		if e.IsDir() {
			suggestions = append(suggestions, full+"/")
		} else {
			ext := strings.ToLower(filepath.Ext(name))
			if ext == ".pdf" || ext == ".docx" || ext == ".doc" {
				suggestions = append(suggestions, full)
			}
		}
		if len(suggestions) >= 8 {
			break
		}
	}

	m.acSuggestions = suggestions
	m.acIdx = -1
}

// buildProviderKeys assembles the provider key map for config, omitting empty values.
func buildProviderKeys(linkedIn, indeed string) map[string]string {
	m := make(map[string]string)
	if strings.TrimSpace(linkedIn) != "" {
		m["linkedin"] = strings.TrimSpace(linkedIn)
	}
	if strings.TrimSpace(indeed) != "" {
		m["indeed"] = strings.TrimSpace(indeed)
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// analyzeResumeCmd validates the resume and, when AI Assist is on, builds a career profile.
func analyzeResumeCmd(path string, gen int, ai resume.AIOptions) tea.Cmd {
	return func() tea.Msg {
		r := resume.AnalyzeFull(path, ai)
		return ResumeAnalysisDoneMsg{Gen: gen, Result: r, Path: path}
	}
}

func resumeAnalysisStartCmd(path string, gen int) tea.Cmd {
	return func() tea.Msg { return ResumeAnalysisStartMsg{Gen: gen, Path: path} }
}

func (m FormModel) aiOptions() resume.AIOptions {
	return resume.AIOptions{
		Enabled:      m.aiAssist,
		Provider:     m.aiProviderValue(),
		LocalURL:     m.inputs[fLocalLLMURL].Value(),
		LocalModel:   m.inputs[fLocalLLMModel].Value(),
		OpenAIKey:    m.inputs[fOpenAIKey].Value(),
		AnthropicKey: m.inputs[fAnthropicKey].Value(),
	}
}

// resumePathChanged reports whether path differs from the last analyzed file.
func (m FormModel) resumePathChanged(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	return filepath.Clean(path) != filepath.Clean(m.lastAnalyzedPath)
}

// startResumeAnalysis kicks off analysis (used on path change or manual refresh).
func (m FormModel) startResumeAnalysis(path string) (FormModel, tea.Cmd) {
	path = strings.TrimSpace(path)
	if path == "" || m.skipResumeCheck {
		return m, nil
	}
	m.resumeAnalysisGen++
	m.resumeAnalyzing = true
	m.resumeAnalysisDone = false
	gen := m.resumeAnalysisGen
	return m, tea.Batch(
		resumeAnalysisStartCmd(path, gen),
		analyzeResumeCmd(path, gen, m.aiOptions()),
		resumeSpinnerTickCmd(),
	)
}

// resumeSpinnerTickCmd fires every 80ms to animate the spinner.
func resumeSpinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return resumeSpinnerTickMsg(t)
	})
}

// ── Save ──────────────────────────────────────────────────────────────────────

func (m FormModel) toConfig() *config.Config {
	cur := currencies[m.currencyIdx]
	salary := ""
	if m.salaryPreset >= 0 && m.salaryPreset < len(cur.Presets) {
		salary = strconv.Itoa(cur.Presets[m.salaryPreset])
	} else if m.salaryCustom != "" {
		salary = m.salaryCustom
	}
	return &config.Config{
		FirstName:              m.inputs[fFirstName].Value(),
		LastName:               m.inputs[fLastName].Value(),
		Email:                  m.inputs[fEmail].Value(),
		Phone:                  m.inputs[fPhone].Value(),
		LinkedInID:             m.inputs[fLinkedInID].Value(),
		ResumePath:             m.inputs[fResumePath].Value(),
		City:                   m.inputs[fCity].Value(),
		YearsOfExperience:      m.inputs[fYearsExp].Value(),
		TargetJobTitles:        strings.Join(m.jobTitleTags, ", "),
		JobIntent:              m.jobIntent,
		WorkType:               m.workTypeValue(),
		TargetLocations:        geo.JoinLocationTags(m.locationTags),
		Currency:               cur.Code,
		MinSalary:              salary,
		ProviderKeys:           buildProviderKeys(m.inputs[fLinkedInKey].Value(), m.inputs[fIndeedKey].Value()),
		AIAssist:               m.aiAssist,
		AIProvider:             m.aiProviderValue(),
		AnthropicKey:           m.inputs[fAnthropicKey].Value(),
		OpenAIKey:              m.inputs[fOpenAIKey].Value(),
		LocalLLMURL:            m.inputs[fLocalLLMURL].Value(),
		LocalLLMModel:          m.inputs[fLocalLLMModel].Value(),
		GmailAppPassword:       m.inputs[fGmailPassword].Value(),
		HunterKey:              m.inputs[fHunterKey].Value(),
		ApolloKey:              m.inputs[fApolloKey].Value(),
		NotifyChannels:         m.notifyChannelValue(),
		DiscordWebhookURL:      m.inputs[fDiscordWebhook].Value(),
		TelegramBotToken:       m.inputs[fTelegramBotToken].Value(),
		TelegramChatID:         m.inputs[fTelegramChatID].Value(),
		ApplyConsent:           m.applyConsent,
		ApplyConsentAt:         m.applyConsentAt,
		MaxAppsPerRun:          parsePositiveInt(m.inputs[fMaxPerRun].Value(), 10),
		MaxAppsPerDay:          parsePositiveInt(m.inputs[fMaxPerDay].Value(), 25),
		ApplyDelaySec:          parsePositiveInt(m.inputs[fApplyDelaySec].Value(), 3),
		CompanyBlocklist:       strings.TrimSpace(m.inputs[fCompanyBlocklist].Value()),
		WorkAuth:               m.workAuth,
		NoticePeriodDays:       strings.TrimSpace(m.inputs[fNoticePeriod].Value()),
		OfficeDaysPerWeek:      strings.TrimSpace(m.inputs[fOfficeDays].Value()),
		CoverLetterMode:        m.coverLetterMode,
		CoverLetterText:        m.inputs[fCoverLetterText].Value(),
		OutreachConsent:        m.outreachConsent,
		OutreachConsentAt:      m.outreachConsentAt,
		OutreachMode:           m.outreachMode,
		MaxEmailsPerDay:        m.maxEmailsPerDay,
		MaxLinkedInPerDay:      m.maxLinkedInPerDay,
		EmailSubjectTpl:        m.emailSubjectTpl,
		EmailBodyTpl:           m.emailBodyTpl,
		LinkedInMsgTpl:         m.linkedinMsgTpl,
		LinkedInMode:           m.linkedinMode,
		LinkedInSessionCookie:  m.linkedinSessionCookie,
		OutreachAutoQueue:      m.outreachAutoQueue,
		OutreachAICompose:      m.outreachAICompose,
		OutreachAIReview:       m.outreachAIReview,
		OutreachGenModel:       m.outreachGenModel,
		OutreachCheckModel:     m.outreachCheckModel,
		OutreachMinScore:       m.outreachMinScore,
		OutreachMaxRetries:     m.outreachMaxRetries,
		OutreachSMTPVerify:     m.outreachSMTPVerify,
		GmailOAuthClientID:     m.gmailOAuthClientID,
		GmailOAuthClientSecret: m.gmailOAuthClientSecret,
		GmailOAuthRefreshToken: m.gmailOAuthRefreshToken,
	}
}

func (m FormModel) saveCmd() tea.Cmd {
	complete := m.IsComplete()
	return func() tea.Msg {
		cfg := m.toConfig()
		// Preserve externally-managed secrets: nexus-gmailauth writes Gmail
		// OAuth tokens straight to config.json while the TUI is running, so a
		// stale in-memory form must not wipe them on the next save.
		if cur, err := config.Load(); err == nil && cur != nil {
			if strings.TrimSpace(cfg.GmailOAuthRefreshToken) == "" {
				cfg.GmailOAuthRefreshToken = cur.GmailOAuthRefreshToken
			}
			if strings.TrimSpace(cfg.GmailOAuthClientID) == "" {
				cfg.GmailOAuthClientID = cur.GmailOAuthClientID
			}
			if strings.TrimSpace(cfg.GmailOAuthClientSecret) == "" {
				cfg.GmailOAuthClientSecret = cur.GmailOAuthClientSecret
			}
		}
		if err := config.Save(cfg); err != nil {
			return ErrMsg{err}
		}
		if complete {
			return ProfileCompleteMsg{Cfg: cfg}
		}
		return SavedMsg{Cfg: cfg}
	}
}

func (m FormModel) workTypeValue() string {
	var parts []string
	for i, sel := range m.wtSelected {
		if sel {
			parts = append(parts, wtOptions[i])
		}
	}
	return strings.Join(parts, ", ")
}

func (m FormModel) aiProviderValue() string {
	if !m.aiAssist {
		return ""
	}
	if m.aiProvider == "api" {
		return "api"
	}
	return "local"
}

func (m FormModel) notifyChannelValue() []string {
	avail := notifier.Available()
	var out []string
	for i, sel := range m.ncSelected {
		if sel && i < len(avail) {
			out = append(out, avail[i].ID)
		}
	}
	return out
}

// ── Focus helpers ─────────────────────────────────────────────────────────────

// IsComplete returns true when all required fields have values.
func (m FormModel) IsComplete() bool {
	return len(m.MissingFields()) == 0
}

// MissingFields returns human-readable names of unfilled required fields.
func (m FormModel) MissingFields() []string {
	var missing []string
	req := []struct {
		idx  int
		name string
	}{
		{fFirstName, "First Name"},
		{fLastName, "Last Name"},
		{fEmail, "Email"},
		{fPhone, "Phone"},
		{fLinkedInID, "LinkedIn ID"},
		{fResumePath, "Resume Path"},
	}
	for _, r := range req {
		val := strings.TrimSpace(m.inputs[r.idx].Value())
		if val == "" {
			missing = append(missing, r.name)
		} else if r.idx == fResumePath && m.resumeAnalysisDone && !m.resumeAnalysisResult.Valid {
			missing = append(missing, "Resume (invalid file)")
		}
	}
	if len(m.jobTitleTags) == 0 {
		missing = append(missing, "Target Job Titles")
	}
	wtAny := false
	for _, s := range m.wtSelected {
		if s {
			wtAny = true
		}
	}
	if !wtAny {
		missing = append(missing, "Work Type")
	}
	if m.salaryPreset < 0 && m.salaryCustom == "" {
		missing = append(missing, "Min Salary")
	}
	return missing
}

func (m FormModel) BlurAll() FormModel {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
	return m
}

func (m FormModel) FocusCurrent() (FormModel, tea.Cmd) {
	if !m.fieldVisible(m.focused) {
		m.focused = m.nextVisibleField(m.focused, +1)
	}
	if isCustomField(m.focused) {
		return m, nil
	}
	return m, m.inputs[m.focused].Focus()
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m FormModel) View() string {
	var b strings.Builder

	// Onboarding banner — shown until profile is complete
	missing := m.MissingFields()
	if len(missing) > 0 {
		banner := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPurple)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorPurpleMuted)).
			Padding(0, 2).
			Render(
				lipgloss.NewStyle().Bold(true).Render("Complete your profile to start applying") + "\n" +
					mutedStyle.Render("Still needed: ") +
					lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).Render(strings.Join(missing, "  ·  ")),
			)
		b.WriteString("\n  " + banner + "\n")
	}

	locked := m.resumeInvalid()

	currentSection := -1
	for i := 0; i < fieldCount; i++ {
		if !m.fieldVisible(i) {
			continue
		}
		for si, sec := range formSections {
			if sec.start == i && si != currentSection {
				currentSection = si
				b.WriteString(sectionStyle.Render(fmt.Sprintf("  %s", sec.title)) + "\n")
				// Before the key-input fields, show always-active providers.
				if sec.start == fLinkedInKey {
					b.WriteString(m.renderProviderStatusSection())
				}
				if sec.start == fAIAssist {
					b.WriteString(m.renderAISectionHint())
				}
			}
		}

		fieldLocked := locked && isLockedByResume(i)
		active := i == m.focused && !fieldLocked

		var prefix, label, widget string
		if fieldLocked {
			lockedStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorGreyMid)).
				Faint(true)
			prefix = "  "
			label = lockedStyle.Render(fmt.Sprintf("%-22s", fieldLabels[i]))
			widget = lockedStyle.Render("🔒 fix resume path to unlock")
		} else {
			lbl := labelInactiveStyle
			prefix = "  "
			if active {
				lbl = labelActiveStyle
				prefix = "▶ "
			}
			label = lbl.Render(fmt.Sprintf("%-22s", fieldLabels[i]))
			widget = m.renderField(i, active)
		}
		b.WriteString(fmt.Sprintf("%s%s  %s\n", prefix, label, widget))
	}

	b.WriteString("\n")
	if m.saved {
		b.WriteString(savedStyle.Render("  ✓ Auto-saved") + "\n")
	}
	if m.notifyBanner != "" {
		b.WriteString(savedStyle.Render("  "+m.notifyBanner) + "\n")
	}
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  ✗ %v", m.err)) + "\n")
	}
	return m.fitFormHeight(b.String())
}

func (m FormModel) renderField(i int, active bool) string {
	switch i {

	case fLinkedInID:
		id := m.inputs[fLinkedInID].Value()
		if active {
			preview := ""
			if id != "" {
				preview = "  " + mutedStyle.Render("→ linkedin.com/in/"+id)
			}
			return m.inputs[i].View() + preview
		}
		if id == "" {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(id) + "  " + mutedStyle.Render("→ linkedin.com/in/"+id)

	case fResumePath:
		path := m.inputs[fResumePath].Value()
		suffix := m.resumeStatusSuffix()
		if active {
			line := m.inputs[i].View() + suffix
			if len(m.acSuggestions) > 0 {
				line += m.renderAC()
			}
			line += m.renderResumeLibrary()
			return line
		}
		line := ""
		if path == "" {
			line = mutedStyle.Render("—")
		} else {
			line = primaryStyle.Render(path) + suffix
		}
		// Compact teaser when not focused — full picker only while editing.
		return line + m.renderResumeLibraryTeaser()

	case fJobTitles:
		return m.renderJobTitlesField(active)

	case fLocations:
		line := m.renderTagField(m.locationTags, m.inputs[fLocations], active)
		if active && len(m.acSuggestions) > 0 {
			line += m.renderAC()
		}
		return line

	case fWorkType:
		return m.renderWorkType(active)

	case fAIAssist:
		return m.renderAIAssist(active)

	case fAIProvider:
		return m.renderAIProvider(active)

	case fCurrency:
		return m.renderCurrency(active)

	case fMinSalary:
		return m.renderSalary(active)

	case fLinkedInKey, fIndeedKey, fAnthropicKey, fOpenAIKey:
		return m.renderProviderKeyField(i, active)

	case fDiscordWebhook:
		webhook := m.inputs[fDiscordWebhook].Value()
		if active {
			help := mutedStyle.Render("Discord: Server Settings → Integrations → Webhooks → New Webhook → Copy URL  •  ctrl+x clears  •  ctrl+t test")
			return m.inputs[i].View() + "\n    " + help
		}
		if webhook == "" {
			return mutedStyle.Render("—")
		}
		return mutedStyle.Render(maskDots(len(webhook)))

	case fTelegramBotToken:
		if active {
			help := mutedStyle.Render("Telegram: message @BotFather → /newbot → copy the token  •  ctrl+x clears  •  ctrl+t test")
			return m.inputs[i].View() + "\n    " + help
		}
		if m.inputs[fTelegramBotToken].Value() == "" {
			return mutedStyle.Render("—")
		}
		return mutedStyle.Render(maskDots(len(m.inputs[fTelegramBotToken].Value())))

	case fTelegramChatID:
		if active {
			help := mutedStyle.Render("Telegram: message @userinfobot or add bot to group, then copy the chat ID  •  ctrl+t test")
			return m.inputs[i].View() + "\n    " + help
		}
		val := m.inputs[fTelegramChatID].Value()
		if val == "" {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(val)

	case fNotifyChannels:
		return m.renderNotifyChannels(active)

	case fApplyConsent:
		return m.renderApplyConsent(active)

	case fWorkAuth:
		return m.renderWorkAuth(active)

	case fCoverLetterMode:
		return m.renderCoverLetterMode(active)

	case fLocalLLMURL:
		if active {
			help := mutedStyle.Render("Ollama default http://localhost:11434  ·  install from ollama.com if needed")
			return m.inputs[i].View() + "\n    " + help
		}
		val := m.inputs[i].Value()
		if val == "" {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(val)

	case fLocalLLMModel:
		return m.renderLocalLLMPicker(active)

	case fGmailPassword, fHunterKey, fApolloKey:
		if active {
			clue := "  " + mutedStyle.Render("ctrl+x clears")
			return m.inputs[i].View() + clue
		}
		val := m.inputs[i].Value()
		if val == "" {
			return mutedStyle.Render("—")
		}
		return mutedStyle.Render(maskDots(len(val)))

	case fScraperTargets:
		if m.scraperOffline || !scraper.Running() {
			return m.renderScraperSetupMenu()
		}
		// Show installed backends; if active also show full catalog to install more
		installedSet := make(map[string]bool)
		for _, id := range m.scraperInstalled {
			installedSet[id] = true
		}
		var parts []string
		for _, b := range scraper.Catalog {
			if installedSet[b.ID] {
				parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Render("● "+b.Name))
			}
		}
		if len(parts) == 0 {
			return m.renderScraperSetupMenu()
		}
		out := strings.Join(parts, "  ")
		if m.scraperStatus != "" {
			out += "  " + mutedStyle.Render(m.scraperStatus)
		}
		if active {
			out += "\n" + m.renderBackendCatalog()
		}
		return out

	default:
		if active {
			return m.inputs[i].View()
		}
		val := m.inputs[i].Value()
		if val == "" {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(val)
	}
}

// resumeStatusSuffix returns the inline badge shown beside the resume path.
func (m FormModel) resumeStatusSuffix() string {
	if m.skipResumeCheck {
		return "  " + mutedStyle.Render("(validation skipped)")
	}
	switch {
	case m.resumeAnalyzing:
		frame := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPurple)).
			Render(spinnerFrames[m.spinnerFrame])
		label := "Analyzing resume..."
		if m.aiAssist {
			label = "AI analyzing resume — open Resume tab for live status..."
		}
		return "  " + frame + " " + mutedStyle.Render(label)
	case m.resumeAnalysisDone && m.resumeAnalysisResult.Valid:
		msg := m.resumeAnalysisResult.Message
		if m.resumeAnalysisResult.Profile != nil && m.resumeAnalysisResult.Profile.Error == "" && m.resumeAnalysisResult.Profile.Summary != "" {
			msg += " · see Resume tab"
		}
		return "  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).
			Render("✓ "+msg)
	case m.resumeAnalysisDone && !m.resumeAnalysisResult.Valid:
		return "  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorRed)).
			Render("✗ "+m.resumeAnalysisResult.Err)
	}
	return ""
}

// renderProviderKeyField renders a masked API key field with an active/inactive badge.
func (m FormModel) renderProviderKeyField(i int, active bool) string {
	val := m.inputs[i].Value()
	var badge string
	if val != "" {
		badge = "  " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen)).Render("● Active")
	} else {
		badge = "  " + mutedStyle.Render("○ Inactive — enter key to activate")
	}
	if active {
		clue := "  " + mutedStyle.Render("ctrl+x clears")
		return m.inputs[i].View() + badge + clue
	}
	if val == "" {
		return mutedStyle.Render("—") + badge
	}
	// Show masked value + badge when inactive
	return mutedStyle.Render(maskDots(len(m.inputs[i].Value()))) + badge
}

// renderProviderStatusSection renders the always-active providers (no user key needed)
// as a status list above the key input fields.
func (m FormModel) renderProviderStatusSection() string {
	var b strings.Builder
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorGreen))
	b.WriteString("\n")
	for _, p := range allProviders {
		if p.keyField != -1 {
			continue // shown as form fields below
		}
		b.WriteString(fmt.Sprintf("  %-22s  %s\n",
			mutedStyle.Render(p.name),
			activeStyle.Render("● Always active"),
		))
	}
	return b.String()
}

// renderAC renders the autocomplete dropdown below the resume path input.
func (m FormModel) renderAC() string {
	var b strings.Builder
	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorPurple)).
		Bold(true)
	dirStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorPurpleMuted))

	for i, s := range m.acSuggestions {
		isDir := strings.HasSuffix(s, "/")
		label := s
		if isDir {
			label = dirStyle.Render(s)
		} else {
			label = primaryStyle.Render(s)
		}

		if i == m.acIdx {
			b.WriteString("\n    " + selectedStyle.Render("▶ ") + label)
		} else {
			b.WriteString("\n      " + label)
		}
	}
	b.WriteString("\n    " + mutedStyle.Render("↑↓ navigate · enter/tab select · esc dismiss"))
	return b.String()
}

// renderTagField renders inline tag pills + text input (single line).
func (m FormModel) renderTagField(tags []string, input textinput.Model, active bool) string {
	if !active {
		if len(tags) == 0 {
			return mutedStyle.Render("—")
		}
		styled := make([]string, len(tags))
		for i, t := range tags {
			styled[i] = primaryStyle.Render(t)
		}
		return strings.Join(styled, mutedStyle.Render("  ·  "))
	}

	// Active: inline [tag ×] pills using text characters (no borders — borders break line height)
	var parts []string
	for _, tag := range tags {
		pill := tagRemoveStyle.Render("[") +
			tagStyle.Render(tag) +
			tagRemoveStyle.Render(" ×]")
		parts = append(parts, pill)
	}
	parts = append(parts, input.View())
	hint := mutedStyle.Render("  ↑↓ pick · enter add · backspace remove")
	return strings.Join(parts, " ") + hint
}

// renderWorkType renders the inline checkbox row.
func (m FormModel) renderWorkType(active bool) string {
	if !active {
		val := m.workTypeValue()
		if val == "" {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(val)
	}
	parts := make([]string, 3)
	for i, opt := range wtOptions {
		box := "[ ]"
		boxColor := colorGrey
		if m.wtSelected[i] {
			box = "[✓]"
			boxColor = colorGreen
		}
		boxStr := lipgloss.NewStyle().Foreground(lipgloss.Color(boxColor)).Render(box)
		if i == m.wtCursor {
			parts[i] = boxStr + " " + lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(opt)
		} else {
			parts[i] = boxStr + " " + primaryStyle.Render(opt)
		}
	}
	return strings.Join(parts, "  ") + mutedStyle.Render("   h/l move · enter/space toggle · tab next")
}

// renderNotifyChannels renders the notification channel checkbox row with
// per-channel credential warnings when a channel is selected but unconfigured.
// Channel list comes from notifier.Available() so new integrations appear automatically.
func (m FormModel) renderNotifyChannels(active bool) string {
	avail := notifier.Available()
	ncfg := m.notifyConfigSnapshot()

	if !active {
		var on []string
		for i, ch := range avail {
			if i < len(m.ncSelected) && m.ncSelected[i] {
				on = append(on, ch.DisplayName)
			}
		}
		if len(on) == 0 {
			return mutedStyle.Render("— none selected")
		}
		return primaryStyle.Render(strings.Join(on, ", "))
	}

	parts := make([]string, len(avail))
	for i, ch := range avail {
		selected := i < len(m.ncSelected) && m.ncSelected[i]
		box := "[ ]"
		boxColor := colorGrey
		if selected {
			box = "[✓]"
			boxColor = colorGreen
		}
		boxStr := lipgloss.NewStyle().Foreground(lipgloss.Color(boxColor)).Render(box)
		label := ch.DisplayName
		if i == m.ncCursor {
			label = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(label)
		} else {
			label = primaryStyle.Render(label)
		}
		if selected && !ch.Configured(ncfg) {
			warn := lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange)).Render(" ⚠ " + ch.WarnMsg)
			parts[i] = boxStr + " " + label + warn
		} else {
			parts[i] = boxStr + " " + label
		}
	}
	return strings.Join(parts, "   ") + mutedStyle.Render("   ←→/h/l move · enter/space toggle · ctrl+t test · tab next")
}

// notifyConfigSnapshot builds a NotifyConfig from current form values for
// credential checks against the notifier registry.
func (m FormModel) notifyConfigSnapshot() *notifier.NotifyConfig {
	return &notifier.NotifyConfig{
		DiscordWebhookURL: m.inputs[fDiscordWebhook].Value(),
		TelegramBotToken:  m.inputs[fTelegramBotToken].Value(),
		TelegramChatID:    m.inputs[fTelegramChatID].Value(),
		EnabledChannels:   m.notifyChannelValue(),
	}
}

// renderCurrency renders all currencies inline with the selected one highlighted.
func (m FormModel) renderCurrency(active bool) string {
	cur := currencies[m.currencyIdx]
	if !active {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(cur.Code)
	}
	var parts []string
	for i, c := range currencies {
		if i == m.currencyIdx {
			parts = append(parts,
				lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorPurple)).
					Bold(true).
					Render("["+c.Code+"]"),
			)
		} else {
			parts = append(parts, mutedStyle.Render(c.Code))
		}
	}
	return strings.Join(parts, "  ") + mutedStyle.Render("   h/l to pick")
}

// renderSalary renders the preset salary picker or custom input.
func (m FormModel) renderSalary(active bool) string {
	cur := currencies[m.currencyIdx]

	if !active {
		if m.salaryCustom != "" {
			return primaryStyle.Render(cur.Symbol + m.salaryCustom)
		}
		if m.salaryPreset < 0 || m.salaryPreset >= len(cur.Presets) {
			return mutedStyle.Render("—")
		}
		return primaryStyle.Render(formatSalary(cur, cur.Presets[m.salaryPreset]))
	}

	// Custom typing mode
	if m.salaryCustom != "" || m.salaryPreset < 0 {
		cursor := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render("▋")
		val := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPurple)).Bold(true).Render(cur.Symbol + m.salaryCustom)
		return val + cursor + mutedStyle.Render("   backspace · esc for presets")
	}

	// Preset mode
	var parts []string
	for i, p := range cur.Presets {
		label := formatSalary(cur, p)
		if i == m.salaryPreset {
			parts = append(parts,
				lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorPurple)).
					Bold(true).
					Render("["+label+"]"),
			)
		} else {
			parts = append(parts,
				lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorGrey)).
					Render(label),
			)
		}
	}
	return strings.Join(parts, "  ") + mutedStyle.Render("   h/l pick · type to enter custom")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// updateLocationAC refreshes city suggestions for Target Locations.
func (m *FormModel) updateLocationAC(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		m.acSuggestions = nil
		m.acIdx = -1
		return
	}
	hits := geo.Search(input, 8)
	suggestions := make([]string, 0, len(hits))
	for _, c := range hits {
		suggestions = append(suggestions, c.Display())
	}
	m.acSuggestions = suggestions
	m.acIdx = -1
}

// addLocationTag resolves input against the geo index and appends a unique tag.
// Returns false if the city is not in the index.
func (m *FormModel) addLocationTag(raw string) bool {
	c, ok := geo.Resolve(raw)
	if !ok {
		return false
	}
	tag := c.Display()
	for _, existing := range m.locationTags {
		if strings.EqualFold(existing, tag) {
			return true // already present — treat as success
		}
	}
	m.locationTags = append(m.locationTags, tag)
	return true
}

func parseTags(s string) []string {
	var tags []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			tags = append(tags, part)
		}
	}
	return tags
}
