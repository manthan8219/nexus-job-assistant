package api

import (
	"context"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/inbox"
	"github.com/manthan8219/nexus-job-assistant/internal/outreach"
)

// runInboxScanOnce runs the inbox hiring-email scan and persists new signals
// to the highlights store. It is a no-op when Gmail is not configured or the
// scan fails; errors are logged, never fatal. Used by the scheduler and safe
// to call from any handler.
func (s *Server) runInboxScanOnce() {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	if cfg == nil {
		return
	}
	fetcher := outreach.NewGmailIMAPFetcher(cfg)
	if fetcher == nil {
		s.logLine("inbox scan skipped: Email + Gmail app password not configured")
		return
	}
	days, max := inbox.DefaultScanDays, inbox.DefaultScanMax
	if cfg.InboxScanDays > 0 {
		days = cfg.InboxScanDays
	}
	if cfg.InboxScanMax > 0 {
		max = cfg.InboxScanMax
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	hs, err := inbox.Scan(ctx, days, max, fetcher, s.store)
	if err != nil {
		s.logLine("inbox scan failed: " + err.Error())
		return
	}
	p, perr := inbox.HighlightsPath()
	if perr != nil {
		s.logLine("inbox scan: highlights path: " + perr.Error())
		return
	}
	for _, h := range hs {
		if err := inbox.Upsert(p, h); err != nil {
			s.logLine("inbox scan: save highlight: " + err.Error())
		}
	}
	s.logLine("inbox scan complete: " + itoa(len(hs)) + " new signals")
}

// scheduleInboxScan runs the inbox hiring-email scan on a configurable
// interval (inbox_scan_minutes). 0 disables it. The goroutine exits when ctx
// is cancelled.
func (s *Server) scheduleInboxScan(ctx context.Context) {
	interval := time.Duration(0)
	for {
		s.mu.RLock()
		if s.cfg != nil {
			interval = time.Duration(s.cfg.InboxScanMinutes) * time.Minute
		}
		s.mu.RUnlock()
		if interval <= 0 {
			// Disabled — wait for a config change (re-check every minute).
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
				continue
			}
		}
		s.logLine("inbox scan scheduler: running every " + interval.String())
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				s.runInboxScanOnce()
			}
		}
	}
}
