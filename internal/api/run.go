package api

import (
	"context"
	"errors"
	"net/http"
)

// Sentinel errors returned by startEngineRun so handlers can map status codes.
var (
	errEngineBusy        = errors.New("engine is already running")
	errEngineUnavailable = errors.New("engine not available")
)

// startEngineRun sets up and launches an engine run in the background with the
// same reset/configure/drain pipeline every trigger uses (manual run,
// apply-selected, scheduled daily dry-run). dryRun=true guarantees zero
// submissions (consent untouched).
func (s *Server) startEngineRun(dryRun, autoApply bool, run func(ctx context.Context) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == StatusRunning {
		return errEngineBusy
	}
	if s.eng == nil {
		return errEngineUnavailable
	}

	s.status = StatusRunning
	s.errMsg = ""
	s.dryRun = dryRun
	s.autoApply = autoApply
	s.lastJob = ""
	s.foundCount = 0
	s.liveFeed = make([]DashRecent, 0)
	s.recent = make([]DashRecent, 0)
	s.providerProgress = make(map[string]ProviderStatus)

	s.eng.Reset()
	s.eng.DryRun = dryRun
	s.eng.AutoApply = autoApply && s.cfg.ApplyConsent
	s.eng.MaxPerRun = s.cfg.MaxAppsPerRun
	if s.eng.MaxPerRun <= 0 {
		s.eng.MaxPerRun = 10
	}
	s.eng.MinDelay = s.cfg.ApplyDelaySec
	if s.eng.MinDelay <= 0 {
		s.eng.MinDelay = 8
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	if run == nil {
		run = func(runCtx context.Context) error { return s.eng.RunOnce(runCtx) }
	}
	runFn := run

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

		go s.drainEngineChannels(ctx)
		runErr := runFn(ctx)

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

	s.changed()
	return nil
}

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

	if err := s.startEngineRun(input.DryRun, input.AutoApply, nil); err != nil {
		if errors.Is(err, errEngineBusy) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handlePostRunApplySelected submits real applications for approved jobs
// (the review-queue apply flow). The engine enforces consent, caps, delays,
// and idempotency; this handler only wires the background run.
func (s *Server) handlePostRunApplySelected(w http.ResponseWriter, r *http.Request) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(input.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "no job ids provided")
		return
	}
	if cfg := s.cfgFor(r); cfg == nil || !cfg.ApplyConsent {
		writeError(w, http.StatusBadRequest, "give Apply Consent in Config before applying")
		return
	}

	ids := input.IDs
	if err := s.startEngineRun(false, true, func(ctx context.Context) error {
		return s.eng.ApplySelected(ctx, ids)
	}); err != nil {
		if errors.Is(err, errEngineBusy) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
			case "found":
				// One discovery event per unique job (the engine dedupes by URL).
				s.foundCount++
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
