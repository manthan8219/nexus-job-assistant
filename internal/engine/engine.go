package engine

// Package engine — engine.go
// Core engine types: the per-job Result and per-provider ProviderProgress
// events, and the Engine struct with its channel-reset and provider accessors.
// The constructor lives in engine_new.go, the run loop in run.go, and the
// concerns in engine_apply.go / engine_score.go / engine_config.go / engine_notify.go.

import (
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// Result is emitted for each job processed.
type Result struct {
	Job        provider.Job
	Status     string
	Reason     string
	Err        error
	FitScore   int
	FitSummary string
}

// ProviderProgress is emitted when a provider starts or finishes searching.
type ProviderProgress struct {
	Provider string
	Status   string // "searching" | "done" | "error"
	Count    int    // jobs found (when done)
	ErrMsg   string // error message (when error)
}

// Engine runs the job application loop.
type Engine struct {
	cfg        *config.Config
	store      *store.Store
	providers  []provider.Provider
	Notifier   notifier.MultiNotifier // notification channels (can be nil)
	LogCh      chan string            // human-readable log lines (→ Logs tab)
	ResultCh   chan Result            // per-job results
	ProgressCh chan ProviderProgress  // per-provider search progress (→ Dashboard)
	MaxPerRun  int
	MinDelay   int
	DryRun     bool
	// OnApplied is called right after an application is recorded with status
	// "applied" — used to kick off the outreach pipeline (find contact, draft
	// email). Must be cheap/non-blocking (e.g. a channel send). Optional.
	OnApplied    func(app store.Application)
	AutoApply    bool // when false, skip apply and record as skipped
	Verbose      bool
	OnlyProvider string

	// Cached for one run — resume text for sequential fit scoring.
	resumeText    string
	resumeTextErr error
	resumeLoaded  bool
}

// Reset recreates the channels so the engine can be run again after stopping.
func (e *Engine) Reset() {
	e.LogCh = make(chan string, 200)
	e.ResultCh = make(chan Result, 500)
	e.ProgressCh = make(chan ProviderProgress, 200)
}

// ProviderNames returns the names of all registered providers.
func (e *Engine) Cfg() *config.Config { return e.cfg }

func (e *Engine) ProviderNames() []string {
	names := make([]string, len(e.providers))
	for i, p := range e.providers {
		names[i] = p.Name()
	}
	return names
}
