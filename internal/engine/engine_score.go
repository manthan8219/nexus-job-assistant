package engine

// Package engine — engine_score.go
// Resume-text loading and per-job LLM fit scoring for the apply loop.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/resume"
)

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
	return resume.AIOptionsFromConfig(e.cfg)
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
