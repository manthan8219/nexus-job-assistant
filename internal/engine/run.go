package engine

// Package engine — run.go
// The run loop: RunOnce executes one full search → apply cycle (parallel
// provider searches, sequential applications), and processJob applies the
// skip/dry-run/queue/apply decision to a single job.

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// RunOnce executes one full search → apply cycle across all providers.
// Searches run in parallel across providers; applications are sequential.
func (e *Engine) RunOnce(ctx context.Context) error {
	var scoreWg sync.WaitGroup
	runStart := time.Now() // for the run-complete notification duration
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
	// totalFound counts unique jobs surfaced by the search; written by the
	// fan-in goroutine, read after enqueueWg.Wait() (happens-before via WaitGroup).
	var totalFound int
	enqueueWg.Add(1)
	go func() {
		defer enqueueWg.Done()
		defer close(jobCh)
		seen := make(map[string]bool)
		for batch := range foundCh {
			if batch.err != nil {
				continue
			}
			if e.cfg != nil && e.cfg.FreshJobPriority {
				sortFreshFirst(batch.jobs)
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
	failed := 0
	skipped := 0
	var runErrs []string
	var runJobs []notifier.JobEvent
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
		n, stop := e.processJob(ctx, job, profile, &applied, &failed, &skipped, &runErrs, &runJobs, &scoreWg)
		_ = n
		if stop {
			hitCap = true
		}
	}
	enqueueWg.Wait()

	e.log("Run complete — applied to %d job(s)", applied)
	ev := notifier.Event{
		Kind:        notifier.EventRunComplete,
		Timestamp:   time.Now(),
		RunDuration: time.Since(runStart),
		Errors:      runErrs,
		Jobs:        runJobs,
		Found:       totalFound,
	}
	if e.DryRun {
		ev.DryRun = true
		ev.Scanned = applied
	} else {
		ev.TotalApplied = applied
		ev.TotalFailed = failed
		ev.TotalSkipped = skipped
	}
	e.Notifier.Send(ctx, ev)
	return nil
}

// processJob handles one job (skip / dry-run / queue / apply).
// Returns (countedTowardLimit, stopRun). applied/failed/skipped accumulate the
// run's outcome counts for the run-complete notification; runErrs collects
// failure reasons; jobs collects per-job rows for the digest.
func (e *Engine) processJob(ctx context.Context, job provider.Job, profile provider.Profile, applied, failed, skipped *int, runErrs *[]string, jobs *[]notifier.JobEvent, scoreWg *sync.WaitGroup) (bool, bool) {
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
		e.skipJob(job, "company blocklist")
		return false, false
	}

	// Stale cutoff (KAN-18): listings older than the configured cutoff get an
	// honest skip instead of burning an application on a stale posting.
	if e.cfg != nil && staleJob(job, time.Now(), e.cfg.StaleJobCutoffDays) {
		reason := fmt.Sprintf("stale job — posted more than %d days ago", e.cfg.StaleJobCutoffDays)
		e.log("  skip [stale] %s @ %s — %s", job.Title, job.Company, reason)
		e.skipJob(job, reason)
		return false, false
	}

	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if todayN, err := e.store.CountAppliedSince(dayStart); err == nil && todayN >= e.maxAppsPerDay() {
		e.log("Reached daily apply cap (%d)", e.maxAppsPerDay())
		return false, true
	}

	// Fit gate: when MinFitScore is set and we would really apply, score the
	// job first and skip low-fit ones instead of burning an application on a
	// role the LLM says we won't get. Score 0 = unscored (AI off / resume
	// unreadable / LLM error) → fail open so scoring outages never block runs.
	preScore, preSummary := 0, ""
	if e.AutoApply && !e.DryRun && e.minFitScore() > 0 {
		preScore, preSummary = e.scoreJob(ctx, job)
		if fitGateBlocked(preScore, e.minFitScore()) {
			reason := fmt.Sprintf("low fit %d < min %d", preScore, e.minFitScore())
			e.log("  ⊘ fit gate: %s @ %s — %s", job.Title, job.Company, reason)
			_ = e.store.Insert(store.Application{
				Provider: job.Provider, Company: job.Company, Role: job.Title,
				URL: job.URL, Status: store.StatusSkipped, Reason: reason,
				AppliedAt: time.Now(), Location: job.Location, Remote: job.Remote,
				PostedAt: job.PostedAt, Description: job.Description,
				FitScore: preScore, FitSummary: preSummary,
			})
			e.sendResult(Result{Job: job, Status: "skipped", Reason: reason, FitScore: preScore, FitSummary: preSummary})
			return false, false
		}
	}

	// Determine status before scoring so we can insert immediately.
	var status store.Status
	var reason string
	var applyErr error
	var submittedPayload *provider.SubmittedPayload

	if e.DryRun {
		status = store.StatusQueued
		reason = "dry run — found but not submitted"
	} else if !e.AutoApply {
		status = store.StatusQueued
		reason = "queued — awaiting your approval"
	} else {
		e.log("  → Applying: %s @ %s", job.Title, job.Company)
		// Auto-tailor before high-fit apply (KAN-20): TailorPerJob swaps the
		// apply profile's resume for a tailored PDF when the fit clears the
		// floor. Fails open — the base resume is used if tailoring can't run.
		p := e.tailorBeforeApply(ctx, job, preScore, profile)
		result, err := e.providerFor(job.Provider).Apply(ctx, job, p)
		applyErr = err
		if err != nil {
			e.log("  ✗ Error: %v", err)
			status = store.StatusFailed
			reason = err.Error()
		} else {
			status = store.Status(result.Status)
			reason = result.Reason
			submittedPayload = result.Payload
		}
	}

	// Insert immediately — scoring runs in background unless the fit gate
	// already scored this job above (preScore > 0).
	recorded := store.Application{
		Provider: job.Provider, Company: job.Company, Role: job.Title,
		URL: job.URL, Status: status, Reason: reason,
		AppliedAt: time.Now(), Location: job.Location, Remote: job.Remote,
		PostedAt: job.PostedAt, Description: job.Description,
		FitScore: preScore, FitSummary: preSummary,
	}
	_ = e.store.Insert(recorded)
	// Persist the exact submission audit (KAN-33) on success — fail open.
	if status == store.StatusApplied && submittedPayload != nil {
		if data, merr := json.Marshal(submittedPayload); merr == nil {
			if perr := e.store.SetSubmittedPayloadByURL(job.URL, string(data)); perr != nil {
				e.log("Store error (payload): %v", perr)
			}
		}
	}
	// Kick the outreach pipeline (find HR email → draft → notify) if wired.
	if status == store.StatusApplied && e.OnApplied != nil {
		e.OnApplied(recorded)
	}

	// Emit result right away (fitScore=0 placeholder unless the fit gate
	// already scored this job; background scoring re-emits otherwise).
	if e.DryRun {
		e.log("  [dry-run] %s @ %s (%s)", job.Title, job.Company, job.Location)
		e.sendResult(Result{Job: job, Status: "dry-run", Reason: reason, FitScore: preScore, FitSummary: preSummary})
		*applied++
		*jobs = append(*jobs, notifier.JobEvent{
			Title: job.Title, Company: job.Company, URL: job.URL, Status: "found",
		})
	} else if !e.AutoApply {
		e.sendResult(Result{Job: job, Status: "queued", Reason: reason, FitScore: preScore, FitSummary: preSummary})
	} else if applyErr != nil {
		e.sendResult(Result{Job: job, Status: "failed", Err: applyErr, FitScore: preScore, FitSummary: preSummary})
	} else {
		e.sendResult(Result{Job: job, Status: string(status), Reason: reason, FitScore: preScore, FitSummary: preSummary})
	}

	// Background fit scoring — updates DB and re-emits result with score.
	// Skipped when the fit gate already scored this job above.
	if preScore == 0 {
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
	}

	switch status {
	case store.StatusApplied:
		e.log("  ✓ Applied: %s @ %s", job.Title, job.Company)
		*applied++
		*jobs = append(*jobs, notifier.JobEvent{
			Title: job.Title, Company: job.Company, URL: job.URL, Status: "applied",
		})
		e.Notifier.Send(ctx, notifier.Event{
			Kind:     notifier.EventJobApplied,
			JobTitle: job.Title,
			Company:  job.Company,
			Location: job.Location,
			Provider: job.Provider,
			Board:    job.Board,
			JobURL:   job.URL,
			PostedAt: job.PostedAt,
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
		*skipped++
		if e.Verbose {
			e.log("  ~ Skipped: %s @ %s — %s", job.Title, job.Company, reason)
		}
	case store.StatusFailed:
		*failed++
		*runErrs = append(*runErrs, reason)
		*jobs = append(*jobs, notifier.JobEvent{
			Title: job.Title, Company: job.Company, URL: job.URL, Status: "failed", Reason: reason,
		})
		e.log("  ✗ Failed:  %s @ %s — %s", job.Title, job.Company, reason)
		e.Notifier.Send(ctx, notifier.Event{
			Kind:     notifier.EventJobFailed,
			JobTitle: job.Title,
			Company:  job.Company,
			Location: job.Location,
			Provider: job.Provider,
			Board:    job.Board,
			JobURL:   job.URL,
			Reason:   reason,
		})
	case store.StatusQueued:
		// Dry-run jobs are counted as scanned in the dry-run branch above;
		// queue-mode jobs (no auto-apply) count as skipped.
		if !e.DryRun {
			*skipped++
		}
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
