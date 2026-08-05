package ui

// Package ui — form.go
// The Config form model: the FormModel struct and its constructor/lifecycle
// (NewFormModel, Init, InitCmd, ConsumesEscape, CustomFieldActive). Message
// handling lives in form_update.go, key dispatch in form_keys.go, rendering in
// form_view.go / form_render.go, and each field concern in its own form_*.go.

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/geo"
	"github.com/manthan8219/nexus-job-assistant/internal/localllm"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
	"github.com/manthan8219/nexus-job-assistant/internal/scraper"
)

// FormModel is the Bubble Tea model for the Config form.
type FormModel struct {
	inputs [fieldCount]textinput.Model

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

	// JobPilot-generated resume library (PDFs under ~/.nexus/resumes)
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

// NewFormModel builds a Config form model from an existing config (or defaults),
// wiring up text inputs, AI/backend state, tags, currency/salary, scraper
// status, and cached resume analysis.
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
		cfg.AnthropicKey, cfg.OpenAIKey,
		cfg.GoogleKey, cfg.DeepSeekKey, cfg.GroqKey, cfg.MistralKey,
		cfg.TogetherKey, cfg.OpenRouterKey, cfg.XAIKey,
		cfg.LocalLLMURL, cfg.LocalLLMModel,
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
			i == fGoogleKey || i == fDeepSeekKey || i == fGroqKey || i == fMistralKey ||
			i == fTogetherKey || i == fOpenRouterKey || i == fXAIKey ||
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

// Init satisfies tea.Model; the form blinks its active input.
func (m FormModel) Init() tea.Cmd { return textinput.Blink }
