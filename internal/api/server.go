package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/companies"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/contacts"
	"github.com/manthan8219/nexus-job-assistant/internal/deliverability"
	"github.com/manthan8219/nexus-job-assistant/internal/engine"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// RunStatus mirrors the engine lifecycle.
type RunStatus string

const (
	StatusIdle    RunStatus = "idle"
	StatusRunning RunStatus = "running"
	StatusDone    RunStatus = "done"
	StatusError   RunStatus = "error"
	StatusStopped RunStatus = "stopped"
)

// ProviderStatus holds per-provider search progress.
type ProviderStatus struct {
	Status string `json:"status"`
	Count  int    `json:"count,omitempty"`
	ErrMsg string `json:"errMsg,omitempty"`
}

// DashRecent is a single line in the live/recent feed.
type DashRecent struct {
	Label  string `json:"label"`
	Status string `json:"status"`
}

// Server is the HTTP API server for Nexus.
type Server struct {
	cfg   *config.Config
	store *store.Store
	eng   *engine.Engine
	addr  string

	mu        sync.RWMutex
	status    RunStatus
	errMsg    string
	dryRun    bool
	autoApply bool
	lastJob   string
	lastJobAt time.Time

	providerProgress map[string]ProviderStatus
	foundCount       int
	liveFeed         []DashRecent
	recent           []DashRecent
	applied          int
	skipped          int
	failed           int
	appliedToday     int
	logLines         []string
	cancel           context.CancelFunc
	notifier         notifier.MultiNotifier
	companies        *companies.DB // company footprint store (~/.nexus/companies.db)
	contacts         *contacts.DB  // saved OSINT contacts store (~/.nexus/contacts.db)

	notifyMu     sync.Mutex
	subscribers  map[chan struct{}]struct{} // mission-stream wake-up channels
	sseHeartbeat time.Duration              // interval between periodic snapshot pushes

	// txtResolver is the DNS backend for deliverability audits. Nil means
	// net.DefaultResolver; tests inject a fake so no real DNS is touched.
	txtResolver deliverability.TxtResolver
}

// New creates an API server.
func New(cfg *config.Config, st *store.Store, eng *engine.Engine, addr string) *Server {
	discordURL, tgToken, tgChatID, channels := cfg.NotifyFields()
	mn := notifier.FromConfig(&notifier.NotifyConfig{
		DiscordWebhookURL:  discordURL,
		TelegramBotToken:   tgToken,
		TelegramChatID:     tgChatID,
		EnabledChannels:    channels,
		Email:              cfg.Email,
		GmailAppPassword:   cfg.GmailAppPassword,
		EmailNotifications: cfg.EmailNotifications,
	})
	// Open the companies store best-effort (embedded catalogs only — no
	// network); handlers degrade gracefully if nil.
	cdb, _ := companies.OpenDefaultEmbedded()
	// Open the saved-contacts store best-effort; handlers degrade if nil.
	ktdb, _ := contacts.OpenDefault()
	// Every real outreach send attempt lands an audit entry in the store.
	if st != nil {
		outreach.SetSentLogger(func(e store.OutreachLogEntry) {
			_ = st.SaveOutreachLog(e)
		})
	}
	return &Server{
		cfg:              cfg,
		store:            st,
		eng:              eng,
		addr:             addr,
		status:           StatusIdle,
		providerProgress: make(map[string]ProviderStatus),
		liveFeed:         make([]DashRecent, 0),
		recent:           make([]DashRecent, 0),
		notifier:         mn,
		companies:        cdb,
		contacts:         ktdb,
		subscribers:      make(map[chan struct{}]struct{}),
		sseHeartbeat:     15 * time.Second,
	}
}

// ListenAndServe starts the HTTP server and blocks until SIGINT/SIGTERM.
func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	handler := s.withMiddleware(mux)

	httpServer := &http.Server{
		Addr:         s.addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		log.Printf("API server shutting down...")
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	// Daily safe dry-run scheduler (runs even when no browser is open).
	go s.scheduleDailyRuns(ctx)

	log.Printf("Nexus API server listening on %s", s.addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("api server: %w", err)
	}
	return nil
}

// registerRoutes wires all endpoints to the mux.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Health
	mux.HandleFunc("GET /health", s.handleHealth)

	// Config
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	mux.HandleFunc("PATCH /api/config", s.handlePatchConfig)
	mux.HandleFunc("GET /api/config/complete", s.handleGetConfigComplete)

	// Geo (location autocomplete)
	mux.HandleFunc("GET /api/geo/search", s.handleGeoSearch)

	// File system (resume path autocomplete)
	mux.HandleFunc("GET /api/fs/autocomplete", s.handleFSAutocomplete)

	// LLM (local AI)
	mux.HandleFunc("GET /api/llm/status", s.handleLLMStatus)
	mux.HandleFunc("POST /api/llm/pull", s.handleLLMPull)
	mux.HandleFunc("GET /api/llm/pull/{model}/status", s.handleLLMPullStatus)

	// Career scraper
	mux.HandleFunc("GET /api/scraper/status", s.handleScraperStatus)
	mux.HandleFunc("POST /api/scraper/install", s.handleScraperInstall)
	mux.HandleFunc("POST /api/scraper/start", s.handleScraperStart)

	// Mission (dashboard)
	mux.HandleFunc("GET /api/mission", s.handleGetMission)
	mux.HandleFunc("GET /api/mission/stream", s.handleStreamMission)

	// Run control
	mux.HandleFunc("POST /api/run", s.handlePostRun)
	mux.HandleFunc("POST /api/run/apply-selected", s.handlePostRunApplySelected)
	mux.HandleFunc("DELETE /api/run", s.handleDeleteRun)

	// Jobs
	mux.HandleFunc("GET /api/jobs", s.handleGetJobs)
	mux.HandleFunc("POST /api/jobs", s.handlePostJobs)
	mux.HandleFunc("PATCH /api/jobs/{id}/outcome", s.handlePatchJobOutcome)
	mux.HandleFunc("POST /api/jobs/{id}/dismiss", s.handlePostJobDismiss)
	mux.HandleFunc("POST /api/applications/{id}/approved", s.handlePostApplicationApproved)

	// Companies
	mux.HandleFunc("GET /api/companies", s.handleGetCompanies)
	mux.HandleFunc("PUT /api/companies", s.handlePutCompany)
	mux.HandleFunc("POST /api/companies/refresh", s.handlePostCompaniesRefresh)
	mux.HandleFunc("GET /api/companies/{name}/jobs", s.handleGetCompanyJobs)

	// Contacts
	mux.HandleFunc("GET /api/contacts/search", s.handleGetContactsSearch)
	mux.HandleFunc("GET /api/contacts/saved", s.handleGetContactsSaved)
	mux.HandleFunc("PUT /api/contacts/saved", s.handlePutContactsSaved)
	mux.HandleFunc("DELETE /api/contacts/saved/{id}", s.handleDeleteContactsSaved)

	// Outreach
	mux.HandleFunc("GET /api/outreach/setup", s.handleGetOutreachSetup)
	mux.HandleFunc("PUT /api/outreach/setup", s.handlePutOutreachSetup)
	mux.HandleFunc("GET /api/outreach/items", s.handleGetOutreachItems)
	mux.HandleFunc("POST /api/outreach/build", s.handlePostOutreachBuild)
	mux.HandleFunc("POST /api/outreach/send/{id}", s.handlePostOutreachSend)
	mux.HandleFunc("GET /api/outreach/log", s.handleGetOutreachLog)

	// Logs
	mux.HandleFunc("GET /api/logs", s.handleGetLogs)
	mux.HandleFunc("DELETE /api/logs", s.handleDeleteLogs)

	// Usage
	mux.HandleFunc("GET /api/usage", s.handleGetUsage)

	// Resume
	mux.HandleFunc("GET /api/resume/analyze", s.handleGetResumeAnalyze)
	mux.HandleFunc("POST /api/resume/analyze", s.handlePostResumeAnalyze)
	mux.HandleFunc("POST /api/resume/upload", s.handlePostResumeUpload)
	mux.HandleFunc("GET /api/resume/projects", s.handleGetResumeProjects)
	mux.HandleFunc("PUT /api/resume/projects", s.handlePutResumeProjects)
	mux.HandleFunc("DELETE /api/resume/projects/{id}", s.handleDeleteResumeProjects)
	mux.HandleFunc("GET /api/resume/skills", s.handleGetResumeSkills)
	mux.HandleFunc("PUT /api/resume/skills", s.handlePutResumeSkills)
	mux.HandleFunc("POST /api/resume/improve", s.handlePostResumeImprove)
	mux.HandleFunc("GET /api/resume/library", s.handleGetResumeLibrary)

	// Job titles
	mux.HandleFunc("POST /api/job-titles/suggest", s.handlePostJobTitlesSuggest)

	// Deliverability
	mux.HandleFunc("GET /api/deliverability/audit", s.handleGetDeliverabilityAudit)

	// Analytics
	mux.HandleFunc("GET /api/analytics", s.handleGetAnalytics)

	// Notifications
	mux.HandleFunc("GET /api/notify/channels", s.handleGetNotifyChannels)
	mux.HandleFunc("POST /api/notify/test", s.handlePostNotifyTest)
}

// logLine adds a line to the in-memory log buffer (capped at 1000).
func (s *Server) logLine(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.logLines) > 1000 {
		s.logLines = s.logLines[len(s.logLines)-500:]
	}
	s.logLines = append(s.logLines, line)
}
