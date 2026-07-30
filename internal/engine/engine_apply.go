package engine

// Package engine — engine_apply.go
// Apply gating: rate limits and consent (from config, with safe defaults), the
// fit-score gate, and the company blocklist. These guards must stay intact on
// every path that submits an application (see AGENTS.md §14).

import "strings"

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

// maxAppsPerDay returns the configured daily application cap (default 25).
func (e *Engine) maxAppsPerDay() int {
	if e.cfg != nil && e.cfg.MaxAppsPerDay > 0 {
		return e.cfg.MaxAppsPerDay
	}
	return 25
}

// minFitScore returns the configured fit-gate threshold (0 = off).
func (e *Engine) minFitScore() int {
	if e.cfg != nil && e.cfg.MinFitScore > 0 {
		return e.cfg.MinFitScore
	}
	return 0
}

// fitGateBlocked reports whether a pre-apply fit score blocks the application.
// Score 0 means "unscored" (AI off, resume unreadable, LLM error) and never
// blocks — the gate fails open so scoring outages can't stall a run.
func fitGateBlocked(score, min int) bool {
	return min > 0 && score > 0 && score < min
}

// companyBlocked reports whether company matches any entry in the comma-separated
// blocklist (case-insensitive substring match).
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
