package api

import (
	"context"
	"net/http"
)

// handlePostRun starts an engine run in the background.
func (s *Server) handlePostRun(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DryRun    bool `json:"dryRun"`
		AutoApply bool `json:"autoApply"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	s.mu.Lock()
	if s.status == StatusRunning {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, "engine is already running")
		return
	}

	// Reset state for a new run
	s.status = StatusRunning
	s.errMsg = ""
	s.dryRun = input.DryRun
	s.autoApply = input.AutoApply
	s.lastJob = ""
	s.foundCount = 0
	s.liveFeed = make([]DashRecent, 0)
	s.recent = make([]DashRecent, 0)
	s.providerProgress = make(map[string]ProviderStatus)

	// Reset and configure engine
	if s.eng != nil {
		s.eng.Reset()
		s.eng.DryRun = input.DryRun
		s.eng.AutoApply = input.AutoApply && s.cfg.ApplyConsent
		s.eng.MaxPerRun = s.cfg.MaxAppsPerRun
		if s.eng.MaxPerRun <= 0 {
			s.eng.MaxPerRun = 10
		}
		s.eng.MinDelay = s.cfg.ApplyDelaySec
		if s.eng.MinDelay <= 0 {
			s.eng.MinDelay = 8
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()
	s.changed()

	// Run the engine in a goroutine
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.mu.Lock()
				s.status = StatusError
				s.errMsg = "engine panic"
				s.mu.Unlock()
				s.changed()
			}
		}()

		// Drain engine channels
		go s.drainEngineChannels(ctx)

		runErr := s.eng.RunOnce(ctx)

		s.mu.Lock()
		if runErr != nil {
			s.status = StatusError
			s.errMsg = runErr.Error()
		} else {
			s.status = StatusDone
		}
		s.mu.Unlock()
		s.changed()
	}()

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleDeleteRun stops a running engine.
func (s *Server) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.status == StatusRunning {
		s.status = StatusStopped
	}
	s.mu.Unlock()
	s.changed()

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// drainEngineChannels reads from the engine's channels and updates server state.
func (s *Server) drainEngineChannels(ctx context.Context) {
	if s.eng == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case p, ok := <-s.eng.ProgressCh:
			if !ok {
				return
			}
			s.mu.Lock()
			s.providerProgress[p.Provider] = ProviderStatus{
				Status: p.Status,
				Count:  p.Count,
				ErrMsg: p.ErrMsg,
			}
			s.mu.Unlock()
			s.changed()

		case r, ok := <-s.eng.ResultCh:
			if !ok {
				return
			}
			s.mu.Lock()
			s.lastJob = r.Job.Title + " @ " + r.Job.Company
			s.liveFeed = append(s.liveFeed, DashRecent{
				Label:  r.Job.Title + " @ " + r.Job.Company,
				Status: r.Status,
			})
			s.recent = append(s.recent, DashRecent{
				Label:  r.Job.Title + " @ " + r.Job.Company,
				Status: r.Status,
			})
			if len(s.liveFeed) > 100 {
				s.liveFeed = s.liveFeed[len(s.liveFeed)-50:]
			}
			if len(s.recent) > 10 {
				s.recent = s.recent[len(s.recent)-10:]
			}
			switch r.Status {
			case "applied":
				s.applied++
			case "skipped":
				s.skipped++
			case "failed":
				s.failed++
			}
			s.mu.Unlock()
			s.changed()

		case l, ok := <-s.eng.LogCh:
			if !ok {
				return
			}
			s.logLine(l)
		}
	}
}
