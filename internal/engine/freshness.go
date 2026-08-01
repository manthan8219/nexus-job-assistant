package engine

// Package engine — freshness.go
// Fresh-job priority + stale-job cutoff (KAN-18): when enabled in config, the
// run loop applies the newest listings first and skips postings older than a
// configurable cutoff instead of spending an application on a stale job.

import (
	"sort"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// staleJob reports whether job is older than cutoffDays (by PostedAt) at now.
// Unknown posting dates (zero PostedAt) are never stale — we fail open rather
// than drop a listing we cannot date. A non-positive cutoff disables the check.
func staleJob(job provider.Job, now time.Time, cutoffDays int) bool {
	if cutoffDays <= 0 || job.PostedAt.IsZero() {
		return false
	}
	return job.PostedAt.Before(now.AddDate(0, 0, -cutoffDays))
}

// sortFreshFirst orders jobs most-recently-posted first. The sort is stable,
// so jobs with equal (or unknown) posting dates keep their arrival order. Jobs
// without a posting date sort last — never dropped, just deprioritized behind
// dated listings.
func sortFreshFirst(jobs []provider.Job) {
	sort.SliceStable(jobs, func(i, j int) bool {
		return jobs[i].PostedAt.After(jobs[j].PostedAt)
	})
}
