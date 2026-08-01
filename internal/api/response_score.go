package api

// Package api — response_score.go
// Per-application response-probability score (KAN-19). Each job gets a 0-100
// reply-probability estimate on top of the resume-fit score: fit × posting
// freshness × the provider's observed reply probability (from the analytics
// funnel). It degrades gracefully — missing fit or thin provider history still
// yield a freshness-weighted score, and the UI hides anything not meaningful.

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const day = 24 * time.Hour

// responseScoreFor computes the reply-probability score + one-line why for one
// application. providerReply is the provider's observed 0-100 reply
// probability (0 when the provider has no applied history yet).
func responseScoreFor(fit int, postedAt time.Time, providerReply int) (int, string) {
	age := time.Since(postedAt)
	if postedAt.IsZero() || age < 0 {
		age = 0
	}

	var fresh float64
	switch {
	case age < day:
		fresh = 1.0
	case age < 3*day:
		fresh = 0.75
	case age < 7*day:
		fresh = 0.5
	case age < 30*day:
		fresh = 0.25
	default:
		fresh = 0.05
	}

	fitF := clamp01(float64(fit) / 100)
	prov := clamp01(float64(providerReply) / 100)
	score := int(math.Round((0.5*fitF + 0.3*fresh + 0.2*prov) * 100))
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	var parts []string
	if fit > 0 {
		parts = append(parts, fmt.Sprintf("fit %d", fit))
	}
	switch {
	case fresh >= 1:
		parts = append(parts, "posted recently")
	case fresh >= 0.5:
		parts = append(parts, "recent posting")
	case fresh >= 0.25:
		parts = append(parts, "older posting")
	default:
		parts = append(parts, "stale posting")
	}
	if providerReply > 0 {
		parts = append(parts, fmt.Sprintf("provider reply rate %d%%", providerReply))
	} else {
		parts = append(parts, "provider history is thin")
	}
	return score, strings.Join(parts, " · ")
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
