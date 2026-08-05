package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/auth"
	"github.com/manthan8219/nexus-job-assistant/internal/companies"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/contacts"
	"github.com/manthan8219/nexus-job-assistant/internal/deliverability"
	"github.com/manthan8219/nexus-job-assistant/internal/engine"
	"github.com/manthan8219/nexus-job-assistant/internal/nexusdir"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
	"github.com/manthan8219/nexus-job-assistant/internal/supabase"
	"github.com/manthan8219/nexus-job-assistant/internal/userstore"
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
	eng   *engine.Engine // legacy single-user engine
	addr  string

	// runState is the legacy single-user run state (auth off / tests).
	// Multi-tenant mode keeps one runState per user in s.runs.
	runState

	mu sync.RWMutex // guards config reads/writes

	notifier  notifier.MultiNotifier
	companies *companies.DB // company footprint store (~/.nexus/companies.db)
	contacts  *contacts.DB  // saved OSINT contacts store (~/.nexus/contacts.db)

	// auth verifies identity tokens from the configured provider. Nil means
	// auth is disabled and the API runs in legacy unauthenticated mode.
	auth *auth.Verifier
	// users resolves each authenticated request to its own data island; nil
	// when auth is disabled (legacy single-user layout).
	users *userstore.Registry

	// runs holds one run state per authenticated user (multi-tenant mode).
	runsMu sync.Mutex
	runs   map[string]*runState

	// loopCtx/stopLoops own the per-user scheduler goroutines (multi-tenant
	// mode); created in New, cancelled in ListenAndServe shutdown.
	loopCtx   context.Context
	stopLoops context.CancelFunc

	sseHeartbeat time.Duration // interval between periodic snapshot pushes

	// worker is the always-on outreach pipeline (find contact → AI draft →
	// ready) that runs in API mode. Nil when no store is available.
	worker *outreach.Worker

	// txtResolver is the DNS backend for deliverability audits. Nil means
	// net.DefaultResolver; tests inject a fake so no real DNS is touched.
	txtResolver deliverability.TxtResolver
}

// New creates an API server.
func New(cfg *config.Config, st *store.Store, eng *engine.Engine, addr string) *Server {
	mn := notifiersFromConfig(cfg)
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
	// Multi-tenant islands: when auth is enabled every authenticated request
	// resolves to its own data directory under NEXUS_HOME/users/<userID> and
	// NEXUS_ADMIN_EMAILS may claim the legacy single-user data once; otherwise
	// the legacy process-level layout is used unchanged.
	authV := auth.NewFromEnv()
	var ureg *userstore.Registry
	if authV != nil {
		ureg = userstore.NewRegistry(filepath.Join(nexusdir.Home(), "users"), adminEmails(), 0)
	}

	loopCtx, stopLoops := context.WithCancel(context.Background())
	return &Server{
		cfg:   cfg,
		store: st,
		eng:   eng,
		addr:  addr,
		runState: runState{
			cfg:              cfg,
			apps:             st,
			status:           StatusIdle,
			providerProgress: make(map[string]ProviderStatus),
			liveFeed:         make([]DashRecent, 0),
			recent:           make([]DashRecent, 0),
			logLines:         make([]string, 0),
			subscribers:      make(map[chan struct{}]struct{}),
		},
		notifier:     mn,
		companies:    cdb,
		contacts:     ktdb,
		auth:         authV,
		users:        ureg,
		worker:       wireOutreachWorker(cfg, st, eng),
		runs:         make(map[string]*runState),
		loopCtx:      loopCtx,
		stopLoops:    stopLoops,
		sseHeartbeat: 15 * time.Second,
	}
}

// wireOutreachWorker builds the API-mode outreach worker and connects the
// engine's OnApplied hook to its auto-queue, so every recorded application
// flows into the find-contact → AI-draft → ready pipeline. Returns nil when
// there is no store. Split out so tests can exercise the wiring hermetically
// without opening the server's real ~/.nexus databases.
func wireOutreachWorker(cfg *config.Config, st *store.Store, eng *engine.Engine) *outreach.Worker {
	if st == nil {
		return nil
	}
	wk := outreach.NewWorker(st, func() (*config.Config, error) { return cfg, nil })
	if eng != nil && eng.OnApplied == nil {
		eng.OnApplied = wk.EnqueueAuto
	}
	return wk
}

// ListenAndServe starts the HTTP server and blocks until SIGINT/SIGTERM.
func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	handler := s.withMiddleware(s.withAuth(mux))

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

	// Daily safe dry-run scheduler + inbox scan run even when no browser is
	// open. Per-user loops (multi-tenant mode) are owned by loopCtx and stop
	// with the server.
	if s.stopLoops != nil {
		defer s.stopLoops()
	}
	go s.scheduleDailyRuns(ctx, &s.runState)

	// Inbox hiring-email scan scheduler (configurable interval; 0 = off).
	go s.scheduleInboxScan(ctx, &s.runState)

	// Supabase connection check - log once at startup so storage/DB wiring
	// problems surface immediately instead of failing later mid-run.
	if sc := supabase.FromConfig(s.cfg); sc != nil {
		res := sc.Check(ctx)
		s.logLine("supabase check: database=" + supabaseBool(res.DatabaseSkip, res.DatabaseOK) + " storage=" + supabaseBool(res.StorageSkip, res.StorageOK) + " resumes=" + supabaseBool(res.ResumeBucket, res.ResumeBucket))
		if !res.OK() {
			s.logLine("supabase NOT OK - run cmd/supabase-check for details")
		}
	} else {
		s.logLine("supabase not configured - using local storage (SQLite/JSON)")
	}

	// Start the always-on outreach worker for API mode (KAN-15): find contact,
	// AI-draft, review, and mark items ready for every recorded application.
	if s.worker != nil {
		s.worker.Start(ctx)
		defer s.worker.Finish()
	}

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

	// Auth
	mux.HandleFunc("GET /api/auth/status", s.handleGetAuthStatus)
	mux.HandleFunc("GET /api/auth/me", s.handleGetAuthMe)

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
	mux.HandleFunc("PUT /api/outreach/items/{id}/variant", s.handlePutOutreachItemVariant)
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
	mux.HandleFunc("GET /api/resume/templates", s.handleGetResumeTemplates)
	mux.HandleFunc("GET /api/resume/templates/{id}/preview.pdf", s.handleGetResumeTemplatePreviewPDF)
	mux.HandleFunc("POST /api/resume/templates/{id}/preview", s.handlePostResumeTemplatePreview)
	mux.HandleFunc("GET /api/resume/projects", s.handleGetResumeProjects)
	mux.HandleFunc("PUT /api/resume/projects", s.handlePutResumeProjects)
	mux.HandleFunc("DELETE /api/resume/projects/{id}", s.handleDeleteResumeProjects)
	mux.HandleFunc("GET /api/resume/skills", s.handleGetResumeSkills)
	mux.HandleFunc("PUT /api/resume/skills", s.handlePutResumeSkills)
	mux.HandleFunc("POST /api/resume/improve", s.handlePostResumeImprove)
	mux.HandleFunc("GET /api/resume/library", s.handleGetResumeLibrary)
	mux.HandleFunc("GET /api/resume/library/{id}/pdf", s.handleGetResumeLibraryPDF)

	// Job titles
	mux.HandleFunc("POST /api/job-titles/suggest", s.handlePostJobTitlesSuggest)

	// AI — list a provider's models by configured/typed API key
	mux.HandleFunc("POST /api/ai/models", s.handleGetAIModels)

	// Deliverability
	mux.HandleFunc("GET /api/deliverability/audit", s.handleGetDeliverabilityAudit)

	// Inbox highlights
	mux.HandleFunc("GET /api/highlights", s.handleGetHighlights)
	// Analytics
	mux.HandleFunc("GET /api/analytics", s.handleGetAnalytics)

	// Notifications
	mux.HandleFunc("GET /api/notify/channels", s.handleGetNotifyChannels)
	mux.HandleFunc("POST /api/notify/test", s.handlePostNotifyTest)
	mux.HandleFunc("POST /api/notify/summary", s.handlePostNotifySummary)
}

// supabaseBool renders a health state as ok/FAIL for a status line.
func supabaseBool(skipped, ok bool) string {
	switch {
	case skipped:
		return "skipped"
	case ok:
		return "ok"
	default:
		return "FAIL"
	}
}

// logLine is defined in runstate.go as the legacy alias to the embedded run
// state's capped log buffer (multi-tenant states log through runState.appendLog).
