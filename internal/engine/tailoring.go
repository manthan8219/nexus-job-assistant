package engine

// Package engine — tailoring.go
// Auto-tailor before high-fit apply (KAN-20): when TailorPerJob is on, the
// engine generates an HR-reviewed, job-tailored resume kit for jobs that clear
// the fit floor and points the provider's apply profile at the tailored PDF.

import (
	"context"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/tailor"
)

// tailorHighFit reports whether a fit score clears the tailoring floor. With
// no floor configured (min 0) any positive score qualifies; an unscored job
// (0) is never tailored so AI outages never stall an application.
func tailorHighFit(score, min int) bool {
	return score > 0 && (min <= 0 || score >= min)
}

// tailorBeforeApply generates a job-tailored resume for a high-fit job and
// returns the apply profile pointed at the tailored PDF. It fails open: any
// error (AI off, LLM outage, unreadable resume) returns the original profile
// so an application is never blocked by tailoring.
func (e *Engine) tailorBeforeApply(ctx context.Context, job provider.Job, preScore int, p provider.Profile) provider.Profile {
	cfg := e.cfg
	if cfg == nil || !cfg.TailorPerJob {
		return p
	}
	score := preScore
	if score == 0 {
		// TailorPerJob wants a per-job kit even without a fit gate configured;
		// score now (AI off → 0 → no tailoring, fail open).
		score, _ = e.scoreJob(ctx, job)
	}
	if !tailorHighFit(score, e.minFitScore()) {
		return p
	}
	out, err := tailor.Generate(ctx, cfg, tailor.Input{
		Job:             job,
		ResumePath:      cfg.ResumePath,
		ResumeText:      e.resumeText,
		MaxRounds:       cfg.TailorMaxRounds,
		RegisterLibrary: true,
		Logf:            e.log,
	})
	if err != nil || strings.TrimSpace(out.ResumePDF) == "" {
		e.log("  ✂ tailoring unavailable for %s @ %s — applying with base resume (%v)", job.Title, job.Company, err)
		return p
	}
	e.log("  ✂ tailored resume for %s @ %s → %s", job.Title, job.Company, out.ResumePDF)
	p.ResumePath = out.ResumePDF
	return p
}
