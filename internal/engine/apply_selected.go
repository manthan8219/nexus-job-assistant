package engine

// Package engine — apply_selected.go
// ApplySelected submits real applications for user-approved queued jobs (the
// review-queue approve → apply flow). Guards mirror RunOnce/processJob:
// consent required, dry run blocks, per-run + daily caps, apply delay, and
// idempotency (never re-apply an already-applied job).

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/notifier"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// ApplySelected submits real applications for the given stored application ids.
func (e *Engine) ApplySelected(ctx context.Context, ids []int64) error {
	if e.cfg == nil || !e.cfg.ApplyConsent {
		return fmt.Errorf("engine: apply blocked — give Apply Consent in Config first")
	}
	if e.DryRun {
		return fmt.Errorf("engine: dry run is active — turn it off before applying")
	}
	if e.store == nil {
		return fmt.Errorf("engine: store not available")
	}
	e.syncApplySafety()
	profile := profileFromConfig(e.cfg)

	apps, err := e.store.GetByIDs(ids)
	if err != nil {
		return fmt.Errorf("engine: load applications: %w", err)
	}
	if len(apps) == 0 {
		return fmt.Errorf("engine: no applications found for the given ids")
	}

	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	appliedToday, err := e.store.CountAppliedSince(dayStart)
	if err != nil {
		return fmt.Errorf("engine: count today: %w", err)
	}

	runApplied := 0
	e.log("Applying %d approved job(s) · %d/run · %d/day", len(apps), e.MaxPerRun, e.maxAppsPerDay())
	for _, app := range apps {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Idempotency: never apply twice.
		if app.Status == store.StatusApplied {
			e.log("  = Already applied: %s @ %s — skipping", app.Role, app.Company)
			continue
		}

		if runApplied >= e.MaxPerRun {
			e.log("  ⛔ Per-run cap reached (%d) — stopping", e.MaxPerRun)
			break
		}
		if appliedToday+runApplied >= e.maxAppsPerDay() {
			e.log("  ⛔ Daily cap reached (%d) — stopping", e.maxAppsPerDay())
			break
		}

		job := storeAppToJob(app)
		prov := e.providerByName(app.Provider)
		if prov == nil {
			reason := "provider not registered: " + app.Provider
			e.log("  ✗ %s — skipping %s @ %s", reason, app.Role, app.Company)
			if uerr := e.store.SetStatus(app.ID, store.StatusFailed, reason); uerr != nil {
				e.log("Store error: %v", uerr)
			}
			e.sendResult(Result{Job: job, Status: "failed", Reason: reason})
			e.Notifier.Send(ctx, notifier.Event{
				Kind:     notifier.EventJobFailed,
				JobTitle: job.Title,
				Company:  job.Company,
				Provider: job.Provider,
				Reason:   reason,
			})
			continue
		}

		e.log("  → Applying: %s @ %s (%s)", job.Title, job.Company, job.Provider)
		result, err := prov.Apply(ctx, job, profile)

		status := store.StatusApplied
		reason := ""
		if err != nil {
			status = store.StatusFailed
			reason = err.Error()
		} else if result.Status == string(store.StatusSkipped) {
			status = store.StatusSkipped
			reason = result.Reason
		}

		if uerr := e.store.SetStatus(app.ID, status, reason); uerr != nil {
			e.log("Store error: %v", uerr)
		}
		// Persist the exact submission audit (KAN-33) — fail open, never
		// block an apply on a store error.
		if status == store.StatusApplied && result.Payload != nil {
			if data, merr := json.Marshal(result.Payload); merr == nil {
				if perr := e.store.SetSubmittedPayload(app.ID, string(data)); perr != nil {
					e.log("Store error (payload): %v", perr)
				}
			}
		}
		if aerr := e.store.SetApproved(app.ID, false); aerr != nil {
			e.log("Store error: %v", aerr)
		}
		e.sendResult(Result{Job: job, Status: string(status), Reason: reason, Err: err, FitScore: app.FitScore, FitSummary: app.FitSummary})

		switch status {
		case store.StatusApplied:
			runApplied++
			e.log("  ✓ Applied: %s @ %s", job.Title, job.Company)
			e.Notifier.Send(ctx, notifier.Event{
				Kind:     notifier.EventJobApplied,
				JobTitle: job.Title,
				Company:  job.Company,
				Location: job.Location,
				Provider: job.Provider,
			})
			if e.OnApplied != nil {
				app.Status = store.StatusApplied
				app.AppliedAt = time.Now()
				e.OnApplied(app)
			}
			d := humanDelay(e.MinDelay)
			if e.applyDelay != nil {
				d = e.applyDelay(e.MinDelay)
			}
			e.log("  Waiting %s before next application...", d.Round(time.Second))
			select {
			case <-time.After(d):
			case <-ctx.Done():
				return ctx.Err()
			}
		case store.StatusFailed:
			e.log("  ✗ Failed: %s @ %s — %s", job.Title, job.Company, reason)
			e.Notifier.Send(ctx, notifier.Event{
				Kind:     notifier.EventJobFailed,
				JobTitle: job.Title,
				Company:  job.Company,
				Provider: job.Provider,
				Reason:   reason,
			})
		case store.StatusSkipped:
			e.log("  ~ Skipped: %s @ %s — %s", job.Title, job.Company, reason)
		}
	}
	e.log("Apply-selected complete — applied to %d job(s)", runApplied)
	e.Notifier.Send(ctx, notifier.Event{Kind: notifier.EventRunComplete, TotalApplied: runApplied})
	return nil
}

// providerByName returns the registered provider with the given name, or nil.
func (e *Engine) providerByName(name string) provider.Provider {
	for _, p := range e.providers {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

// storeAppToJob converts a stored application back into a provider.Job so the
// apply-selected flow submits through the same provider interface.
func storeAppToJob(a store.Application) provider.Job {
	return provider.Job{
		Title:       a.Role,
		Company:     a.Company,
		Board:       a.Provider,
		Location:    a.Location,
		Remote:      a.Remote,
		URL:         a.URL,
		PostedAt:    a.PostedAt,
		Provider:    a.Provider,
		Description: a.Description,
	}
}
