package api

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// dailyTickInterval is how often the scheduler checks whether the daily
// dry-run is due (kept coarse — the run fires at most once per day).
const dailyTickInterval = 30 * time.Second

// shouldFireDaily reports whether the scheduled daily dry-run should fire now:
// the feature is enabled, no run is busy, it has not fired on this calendar
// day, and the configured "HH:MM" time has passed.
func shouldFireDaily(now time.Time, at string, lastFiredDay string, enabled, busy bool) bool {
	if !enabled || at == "" || busy {
		return false
	}
	if lastFiredDay == now.Format("2006-01-02") {
		return false
	}
	h, m := parseHHMM(at)
	target := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	return !now.Before(target)
}

// parseHHMM parses a "HH:MM" 24h clock string into hour/minute (0,0 on error).
func parseHHMM(s string) (int, int) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	if errH != nil || errM != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0
	}
	return h, m
}

// scheduleDailyRuns fires one safe dry-run per day at the configured time while
// the API server is up, for the given run state (legacy state in single-user
// mode; one per user in multi-tenant mode). Dry runs never submit applications,
// so no additional consent is involved; the run respects the same caps/delays
// as any run.
func (s *Server) scheduleDailyRuns(ctx context.Context, rs *runState) {
	ticker := time.NewTicker(dailyTickInterval)
	defer ticker.Stop()

	var lastFiredDay string
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.RLock()
			cfg := rs.cfg
			if cfg == nil {
				cfg = s.cfg
			}
			at := ""
			var enabled bool
			if cfg != nil {
				enabled = cfg.DailyRunEnabled
				at = cfg.DailyRunAt
			}
			s.mu.RUnlock()
			rs.mu.RLock()
			busy := rs.status == StatusRunning
			rs.mu.RUnlock()

			if shouldFireDaily(now, at, lastFiredDay, enabled, busy) {
				lastFiredDay = now.Format("2006-01-02")
				rs.appendLog("⏰ scheduled daily dry-run firing")
				if err := s.launchRun(rs, nil, true, false, nil); err != nil {
					rs.appendLog("⏰ scheduled dry-run skipped: " + err.Error())
				}
			}
		}
	}
}
