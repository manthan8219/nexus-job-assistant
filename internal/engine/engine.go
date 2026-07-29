package engine

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/manthanmanthan/nexus/data"
	"github.com/manthanmanthan/nexus/internal/config"
	"github.com/manthanmanthan/nexus/internal/geo"
	"github.com/manthanmanthan/nexus/internal/notifier"
	"github.com/manthanmanthan/nexus/internal/provider"
	"github.com/manthanmanthan/nexus/internal/provider/arbeitnow"
	"github.com/manthanmanthan/nexus/internal/provider/ashby"
	"github.com/manthanmanthan/nexus/internal/provider/bamboohr"
	"github.com/manthanmanthan/nexus/internal/provider/breezy"
	"github.com/manthanmanthan/nexus/internal/provider/fourday"
	"github.com/manthanmanthan/nexus/internal/provider/getonbrd"
	"github.com/manthanmanthan/nexus/internal/provider/greenhouse"
	"github.com/manthanmanthan/nexus/internal/provider/hackernews"
	"github.com/manthanmanthan/nexus/internal/provider/himalayas"
	"github.com/manthanmanthan/nexus/internal/provider/jobicy"
	"github.com/manthanmanthan/nexus/internal/provider/jobspresso"
	"github.com/manthanmanthan/nexus/internal/provider/jobvite"
	"github.com/manthanmanthan/nexus/internal/provider/justjoin"
	"github.com/manthanmanthan/nexus/internal/provider/lever"
	"github.com/manthanmanthan/nexus/internal/provider/nodesk"
	"github.com/manthanmanthan/nexus/internal/provider/nofluffjobs"
	"github.com/manthanmanthan/nexus/internal/provider/personio"
	"github.com/manthanmanthan/nexus/internal/provider/pinpoint"
	"github.com/manthanmanthan/nexus/internal/provider/recruitee"
	"github.com/manthanmanthan/nexus/internal/provider/remoteok"
	"github.com/manthanmanthan/nexus/internal/provider/remotive"
	"github.com/manthanmanthan/nexus/internal/provider/smartrecruiters"
	"github.com/manthanmanthan/nexus/internal/provider/teamtailor"
	"github.com/manthanmanthan/nexus/internal/provider/thehub"
	"github.com/manthanmanthan/nexus/internal/provider/themuse"
	"github.com/manthanmanthan/nexus/internal/provider/weworkremotely"
	"github.com/manthanmanthan/nexus/internal/provider/workable"
	"github.com/manthanmanthan/nexus/internal/provider/workday"
	"github.com/manthanmanthan/nexus/internal/provider/workingnomads"
	"github.com/manthanmanthan/nexus/internal/provider/wttj"
	"github.com/manthanmanthan/nexus/internal/provider/careerscraper"
	"github.com/manthanmanthan/nexus/internal/provider/linkedin"
	scr "github.com/manthanmanthan/nexus/internal/scraper"
	"github.com/manthanmanthan/nexus/internal/resume"
	"github.com/manthanmanthan/nexus/internal/store"
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
	cfg          *config.Config
	store        *store.Store
	providers    []provider.Provider
	Notifier     notifier.MultiNotifier // notification channels (can be nil)
	LogCh        chan string            // human-readable log lines (→ Logs tab)
	ResultCh     chan Result            // per-job results
	ProgressCh   chan ProviderProgress  // per-provider search progress (→ Dashboard)
	MaxPerRun    int
	MinDelay     int
	DryRun       bool
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

// New builds an Engine. If companiesPath is non-empty it overrides the embedded companies list.
func New(cfg *config.Config, st *store.Store, companiesPath string) (*Engine, error) {
	var providers []provider.Provider

	// Greenhouse
	var gh *greenhouse.Client
	var err error
	if companiesPath != "" {
		gh, err = greenhouse.NewFromFile(companiesPath)
	} else {
		gh, err = greenhouse.New(data.CompaniesJSON)
	}
	if err != nil {
		return nil, fmt.Errorf("greenhouse init: %w", err)
	}
	providers = append(providers, gh)

	// Ashby — always active, no user key required
	ab, err := ashby.New(data.AshbyCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, ab)
	}

	// SmartRecruiters — always active, no user key required
	sr, err := smartrecruiters.New(data.SmartRecruitersCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, sr)
	}

	lv, err := lever.New(data.LeverCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, lv)
	}

	wk, err := workable.New(data.WorkableCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, wk)
	}

	// Board-wide aggregators — no company list or API key required.
	providers = append(providers, remoteok.New())
	providers = append(providers, remotive.New())
	providers = append(providers, arbeitnow.New())
	providers = append(providers, jobicy.New())
	providers = append(providers, hackernews.New())

	// Workday — per-company ATS, sourced from data/workday_companies.json
	wd, err := workday.New(data.WorkdayCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, wd)
	}

	// Board-wide aggregators (Group A: JSON)
	providers = append(providers, himalayas.New())
	providers = append(providers, fourday.New())
	providers = append(providers, workingnomads.New())
	providers = append(providers, themuse.New())
	providers = append(providers, thehub.New())
	providers = append(providers, getonbrd.New())
	providers = append(providers, nofluffjobs.New())
	providers = append(providers, justjoin.New())
	providers = append(providers, wttj.New())

	// Board-wide aggregators (Group B: RSS/XML)
	providers = append(providers, weworkremotely.New())
	providers = append(providers, jobspresso.New())
	providers = append(providers, nodesk.New())

	// Per-company ATSes (Group C)
	bhr, err := bamboohr.New(data.BambooHRCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, bhr)
	}

	rct, err := recruitee.New(data.RecruiteeCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, rct)
	}

	bzy, err := breezy.New(data.BreezyCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, bzy)
	}

	ppt, err := pinpoint.New(data.PinpointCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, ppt)
	}

	jv, err := jobvite.New(data.JobviteCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, jv)
	}

	tt, err := teamtailor.New(data.TeamtailorCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, tt)
	}

	ps, err := personio.New(data.PersonioCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, ps)
	}

	// Career page scraper — only added when the Python service is installed and running.
	// It auto-discovers /jobs and /careers pages for companies without a known ATS.
	if scr.Installed() {
		if !scr.Running() {
			ollamaURL := cfg.LocalLLMURL
			if ollamaURL == "" {
				ollamaURL = "http://localhost:11434"
			}
			_ = scr.Start(cfg.LocalLLMModel, ollamaURL)
			_ = scr.WaitReady(15 * time.Second)
		}
		if scr.Running() {
			providers = append(providers, careerscraper.New(nil, cfg.LocalLLMModel, cfg.LocalLLMURL))
			providers = append(providers, linkedin.New(3))
		}
	}

	return &Engine{
		cfg:        cfg,
		store:      st,
		providers:  providers,
		Notifier:   notifierFromCfg(cfg),
		LogCh:      make(chan string, 100),
		ResultCh:   make(chan Result, 500),
		ProgressCh: make(chan ProviderProgress, 100),
		MaxPerRun:  10,
		MinDelay:   8,
	}, nil
}

// RebuildNotifier re-reads notification credentials from cfg and replaces the
// current MultiNotifier. Call this whenever the config is saved so new or
// removed channels take effect immediately without restarting.
func (e *Engine) RebuildNotifier(cfg *config.Config) {
	e.Notifier = notifierFromCfg(cfg)
}

// notifierFromCfg is the single wiring point between config and notifier.
// All channels are built here via notifier.FromConfig.
func notifierFromCfg(cfg *config.Config) notifier.MultiNotifier {
	discordURL, tgToken, tgChatID, channels := cfg.NotifyFields()
	return notifier.FromConfig(&notifier.NotifyConfig{
		DiscordWebhookURL: discordURL,
		TelegramBotToken:  tgToken,
		TelegramChatID:    tgChatID,
		EnabledChannels:   channels,
	})
}

func (e *Engine) log(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	select {
	case e.LogCh <- msg:
	default:
	}
}

func (e *Engine) sendProgress(p ProviderProgress) {
	select {
	case e.ProgressCh <- p:
	default:
	}
}

func (e *Engine) sendResult(r Result) {
	select {
	case e.ResultCh <- r:
	default:
		// UI lagging — drop rather than stall the apply pipeline
	}
}

// syncApplySafety pulls rate limits from config (with safe defaults).
func (e *Engine) syncApplySafety() {
	if e.cfg == nil {
		return
	}
	if e.cfg.MaxAppsPerRun > 0 {
		e.MaxPerRun = e.cfg.MaxAppsPerRun
	} else if e.MaxPerRun <= 0 {
		e.MaxPerRun = 10
	}
	if e.cfg.ApplyDelaySec > 0 {
		e.MinDelay = e.cfg.ApplyDelaySec
	} else if e.MinDelay <= 0 {
		e.MinDelay = 3
	}
	// Consent is mandatory for real auto-apply.
	if e.AutoApply && !e.cfg.ApplyConsent {
		e.AutoApply = false
		e.log("Auto-apply blocked — give Apply Consent in Config first")
	}
}

func (e *Engine) maxAppsPerDay() int {
	if e.cfg != nil && e.cfg.MaxAppsPerDay > 0 {
		return e.cfg.MaxAppsPerDay
	}
	return 25
}

func companyBlocked(company, blocklist string) bool {
	company = strings.ToLower(strings.TrimSpace(company))
	if company == "" || strings.TrimSpace(blocklist) == "" {
		return false
	}
	for _, part := range strings.Split(blocklist, ",") {
		b := strings.ToLower(strings.TrimSpace(part))
		if b != "" && strings.Contains(company, b) {
			return true
		}
	}
	return false
}

// RunOnce executes one full search → apply cycle across all providers.
// Searches run in parallel across providers; applications are sequential.
func (e *Engine) RunOnce(ctx context.Context) error {
	var scoreWg sync.WaitGroup
	defer func() {
		scoreWg.Wait() // wait for all background fit-scoring goroutines
		close(e.LogCh)
		close(e.ResultCh)
		close(e.ProgressCh)
	}()
	profile := profileFromConfig(e.cfg)
	criteria := criteriaFromConfig(e.cfg)

	e.syncApplySafety()
	e.loadResumeText()
	// Expand per-company ATS lists from ~/.nexus/companies.db using countries
	// extracted from Config Target Locations (e.g. Bengaluru, India → IN).
	countries := countriesFromConfig(e.cfg)
	e.expandBoardsFromCompanyDB(countries)
	e.log("Starting parallel search across %d provider(s) · region %s", len(e.providers), regionSummary(countries))
	e.log("Limits: %d/run · %d/day · delay %ds · consent=%v", e.MaxPerRun, e.maxAppsPerDay(), e.MinDelay, e.cfg != nil && e.cfg.ApplyConsent)
	e.log("Titles: %v | WorkType: %s | Locations: %v", criteria.Titles, criteria.WorkType, criteria.Locations)

	// Notify: run started
	e.Notifier.Send(ctx, notifier.Event{
		Kind:    notifier.EventRunStarted,
		Message: fmt.Sprintf("Searching **%d** providers for titles: %v", len(e.providers), criteria.Titles),
	})

	// ── Pipelined search → apply ───────────────────────────────────────────
	// As soon as ANY provider returns jobs, we surface them in the UI and
	// start applying — no waiting for every board to finish scraping.
	type foundBatch struct {
		name string
		jobs []provider.Job
		err  error
	}

	jobCh := make(chan provider.Job, 128)
	foundCh := make(chan foundBatch, len(e.providers))

	var searchWg sync.WaitGroup
	for _, p := range e.providers {
		if e.OnlyProvider != "" && p.Name() != e.OnlyProvider {
			continue
		}
		searchWg.Add(1)
		go func(prov provider.Provider) {
			defer searchWg.Done()
			timeout := 60 * time.Second
			if prov.Name() == "careerscraper" || prov.Name() == "linkedin" {
				timeout = 30 * time.Minute // scraping many pages takes time
			}
			searchCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			e.log("[%s] Searching...", prov.Name())
			e.sendProgress(ProviderProgress{Provider: prov.Name(), Status: "searching"})
			jobs, err := prov.Search(searchCtx, criteria)
			if err != nil {
				e.log("[%s] Search error: %v", prov.Name(), err)
				e.sendProgress(ProviderProgress{Provider: prov.Name(), Status: "error", ErrMsg: err.Error()})
				foundCh <- foundBatch{name: prov.Name(), err: err}
				return
			}
			e.sendProgress(ProviderProgress{Provider: prov.Name(), Status: "done", Count: len(jobs)})
			foundCh <- foundBatch{name: prov.Name(), jobs: jobs}
		}(p)
	}

	go func() {
		searchWg.Wait()
		close(foundCh)
	}()

	// Fan-in: emit "found" live + enqueue for apply (dedup by URL).
	var enqueueWg sync.WaitGroup
	enqueueWg.Add(1)
	go func() {
		defer enqueueWg.Done()
		defer close(jobCh)
		seen := make(map[string]bool)
		totalFound := 0
		for batch := range foundCh {
			if batch.err != nil {
				continue
			}
			e.log("[%s] Found %d matching jobs — streaming to live feed", batch.name, len(batch.jobs))
			for _, job := range batch.jobs {
				if job.URL == "" || seen[job.URL] {
					continue
				}
				seen[job.URL] = true
				totalFound++
				e.log("  ✦ %s @ %s  (%s)", job.Title, job.Company, batch.name)
				e.sendResult(Result{Job: job, Status: "found", Reason: "discovered via " + batch.name})
				select {
				case <-ctx.Done():
					return
				case jobCh <- job:
				}
			}
		}
		e.log("Search complete — %d unique jobs queued for processing", totalFound)
	}()

	// Apply worker: processes jobs as they arrive (overlaps with remaining searches).
	applied := 0
	hitCap := false
	for job := range jobCh {
		if hitCap {
			continue
		}
		select {
		case <-ctx.Done():
			enqueueWg.Wait()
			return ctx.Err()
		default:
		}
		if applied >= e.MaxPerRun {
			e.log("Reached max applications per run (%d)", e.MaxPerRun)
			hitCap = true
			continue
		}
		n, stop := e.processJob(ctx, job, profile, &applied, &scoreWg)
		_ = n
		if stop {
			hitCap = true
		}
	}
	enqueueWg.Wait()

	e.log("Run complete — applied to %d job(s)", applied)
	e.Notifier.Send(ctx, notifier.Event{
		Kind:         notifier.EventRunComplete,
		TotalApplied: applied,
	})
	return nil
}

// processJob handles one job (skip / dry-run / queue / apply).
// Returns (countedTowardLimit, stopRun).
func (e *Engine) processJob(ctx context.Context, job provider.Job, profile provider.Profile, applied *int, scoreWg *sync.WaitGroup) (bool, bool) {
	exists, err := e.store.Exists(job.URL)
	if err != nil {
		e.log("Store error: %v", err)
		return false, false
	}
	if exists {
		if e.Verbose {
			e.log("  skip [already applied] %s @ %s", job.Title, job.Company)
		}
		return false, false
	}

	if e.cfg != nil && companyBlocked(job.Company, e.cfg.CompanyBlocklist) {
		e.log("  skip [blocklist] %s @ %s", job.Title, job.Company)
		_ = e.store.Insert(store.Application{
			Provider: job.Provider, Company: job.Company, Role: job.Title,
			URL: job.URL, Status: store.StatusSkipped, Reason: "company blocklist",
			AppliedAt: time.Now(), Location: job.Location, Remote: job.Remote,
			PostedAt: job.PostedAt, Description: job.Description,
		})
		e.sendResult(Result{Job: job, Status: "skipped", Reason: "company blocklist"})
		return false, false
	}

	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if todayN, err := e.store.CountAppliedSince(dayStart); err == nil && todayN >= e.maxAppsPerDay() {
		e.log("Reached daily apply cap (%d)", e.maxAppsPerDay())
		return false, true
	}

	// Determine status before scoring so we can insert immediately.
	var status store.Status
	var reason string
	var applyErr error

	if e.DryRun {
		status = store.StatusSkipped
		reason = "dry run — not submitted"
	} else if !e.AutoApply {
		status = store.StatusSkipped
		reason = "apply manually: " + job.URL
	} else {
		e.log("  → Applying: %s @ %s", job.Title, job.Company)
		result, err := e.providerFor(job.Provider).Apply(ctx, job, profile)
		applyErr = err
		if err != nil {
			e.log("  ✗ Error: %v", err)
			status = store.StatusFailed
			reason = err.Error()
		} else {
			status = store.Status(result.Status)
			reason = result.Reason
		}
	}

	// Insert immediately with fitScore=0 — scoring runs in background.
	_ = e.store.Insert(store.Application{
		Provider: job.Provider, Company: job.Company, Role: job.Title,
		URL: job.URL, Status: status, Reason: reason,
		AppliedAt: time.Now(), Location: job.Location, Remote: job.Remote,
		PostedAt: job.PostedAt, Description: job.Description,
	})

	// Emit result right away (fitScore=0 placeholder; updated after scoring).
	if e.DryRun {
		e.log("  [dry-run] %s @ %s (%s)", job.Title, job.Company, job.Location)
		e.sendResult(Result{Job: job, Status: "dry-run", Reason: reason})
		*applied++
	} else if !e.AutoApply {
		e.sendResult(Result{Job: job, Status: "queued", Reason: reason})
	} else if applyErr != nil {
		e.sendResult(Result{Job: job, Status: "failed", Err: applyErr})
	} else {
		e.sendResult(Result{Job: job, Status: string(status), Reason: reason})
	}

	// Background fit scoring — updates DB and re-emits result with score.
	jobURL := job.URL
	jobCopy := job
	scoreWg.Add(1)
	go func() {
		defer scoreWg.Done()
		defer func() { recover() }() // prevent panic from crashing program
		fitScore, fitSummary := e.scoreJob(context.Background(), jobCopy)
		_ = e.store.UpdateDescriptionFit(jobURL, jobCopy.Description, fitScore, fitSummary)
		// Re-emit result with score so UI can update.
		if e.DryRun {
			e.sendResult(Result{Job: jobCopy, Status: "dry-run", Reason: reason, FitScore: fitScore, FitSummary: fitSummary})
		} else if !e.AutoApply {
			e.sendResult(Result{Job: jobCopy, Status: "queued", Reason: reason, FitScore: fitScore, FitSummary: fitSummary})
		} else if applyErr != nil {
			e.sendResult(Result{Job: jobCopy, Status: "failed", Err: applyErr, FitScore: fitScore, FitSummary: fitSummary})
		} else {
			e.sendResult(Result{Job: jobCopy, Status: string(status), Reason: reason, FitScore: fitScore, FitSummary: fitSummary})
		}
		if fitScore > 0 {
			e.log("  ★ fit=%d %s @ %s", fitScore, jobCopy.Title, jobCopy.Company)
		}
	}()

	switch status {
	case store.StatusApplied:
		e.log("  ✓ Applied: %s @ %s", job.Title, job.Company)
		*applied++
		e.Notifier.Send(ctx, notifier.Event{
			Kind:     notifier.EventJobApplied,
			JobTitle: job.Title,
			Company:  job.Company,
			Location: job.Location,
			Provider: job.Provider,
		})
		d := humanDelay(e.MinDelay)
		e.log("  Waiting %s before next application...", d.Round(time.Second))
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return true, true
		}
		return true, false
	case store.StatusSkipped:
		if e.Verbose {
			e.log("  ~ Skipped: %s @ %s — %s", job.Title, job.Company, reason)
		}
	case store.StatusFailed:
		e.log("  ✗ Failed:  %s @ %s — %s", job.Title, job.Company, reason)
		e.Notifier.Send(ctx, notifier.Event{
			Kind:     notifier.EventJobFailed,
			JobTitle: job.Title,
			Company:  job.Company,
			Provider: job.Provider,
			Reason:   reason,
		})
	}
	return false, false
}

// providerFor returns the registered provider by name.
// Panics if the name is unknown (should never happen at runtime).
func (e *Engine) providerFor(name string) provider.Provider {
	for _, p := range e.providers {
		if p.Name() == name {
			return p
		}
	}
	panic("engine: unknown provider " + name)
}

func (e *Engine) loadResumeText() {
	e.resumeLoaded = true
	e.resumeText = ""
	e.resumeTextErr = nil
	if e.cfg == nil || strings.TrimSpace(e.cfg.ResumePath) == "" {
		e.resumeTextErr = fmt.Errorf("no resume path")
		return
	}
	text, err := resume.ExtractText(e.cfg.ResumePath)
	if err != nil {
		e.resumeTextErr = err
		return
	}
	e.resumeText = text
}

func (e *Engine) aiOptions() resume.AIOptions {
	if e.cfg == nil {
		return resume.AIOptions{}
	}
	return resume.AIOptions{
		Enabled:      e.cfg.AIAssist,
		Provider:     e.cfg.AIProvider,
		LocalURL:     e.cfg.LocalLLMURL,
		LocalModel:   e.cfg.LocalLLMModel,
		OpenAIKey:    e.cfg.OpenAIKey,
		AnthropicKey: e.cfg.AnthropicKey,
	}
}

// scoreJob runs one LLM fit call (caller must be sequential — processJob loop is).
func (e *Engine) scoreJob(ctx context.Context, job provider.Job) (int, string) {
	ai := e.aiOptions()
	if !ai.Enabled {
		return 0, ""
	}
	if !e.resumeLoaded {
		e.loadResumeText()
	}
	if e.resumeTextErr != nil || strings.TrimSpace(e.resumeText) == "" {
		e.log("  fit skip: resume text unavailable (%v)", e.resumeTextErr)
		return 0, ""
	}
	e.log("  scoring fit vs resume: %s @ %s …", job.Title, job.Company)
	fitCtx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	res, err := resume.ScoreJobFit(fitCtx, ai, e.resumeText, job)
	if err != nil {
		e.log("  fit error: %v", err)
		return 0, ""
	}
	e.log("  fit score: %d/100 — %s", res.Score, truncateReason(res.Summary, 80))
	return res.Score, res.Summary
}

func truncateReason(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// profileFromConfig converts config to a provider.Profile.
func profileFromConfig(cfg *config.Config) provider.Profile {
	return provider.Profile{
		FirstName:  cfg.FirstName,
		LastName:   cfg.LastName,
		Email:      cfg.Email,
		Phone:      cfg.Phone,
		ResumePath: cfg.ResumePath,
		LinkedInID: cfg.LinkedInID,
		City:       cfg.City,
		YearsExp:   cfg.YearsOfExperience,
		MinSalary:  cfg.MinSalary,
	}
}

// criteriaFromConfig converts config to a provider.SearchCriteria.
func criteriaFromConfig(cfg *config.Config) provider.SearchCriteria {
	salary, _ := strconv.Atoi(cfg.MinSalary)
	return provider.SearchCriteria{
		Titles:    greenhouse.ParseTitles(cfg.TargetJobTitles),
		Locations: geo.ExpandLocations(geo.ParseLocationTags(cfg.TargetLocations)),
		WorkType:  cfg.WorkType,
		MinSalary: salary,
	}
}

// humanDelay returns a random delay between minSecs and minSecs+12 seconds.
func humanDelay(minSecs int) time.Duration {
	return time.Duration(minSecs+rand.Intn(12)) * time.Second
}
