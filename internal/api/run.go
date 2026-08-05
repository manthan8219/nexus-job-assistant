// Run control handlers: start, apply-selected, stop. Every run is scoped to
// the requesting user's run state (multi-tenant mode) or the legacy embedded
// state (single-user mode), so concurrent tenants never observe each other's
// engine runs.

package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/manthan8219/nexus-job-assistant/internal/engine"
)

// Sentinel errors returned by run-start helpers so handlers can map status codes.
var (
	errEngineBusy        = errors.New("engine is already running")
	errEngineUnavailable = errors.New("engine not available")
)

// launchRun starts a background engine run on rs with the same reset/configure/
// drain pipeline every trigger uses (manual run, apply-selected, scheduled
// daily dry-run). eng may be nil to build one from the state; apply flows pass
// a prebuilt engine because their closure needs it. dryRun=true guarantees
// zero submissions (consent untouched).
func (s *Server) launchRun(rs *runState, eng *engine.Engine, dryRun, autoApply bool, run func(ctx context.Context) error) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.status == StatusRunning {
		return errEngineBusy
	}
	if eng == nil {
		var err error
		eng, err = s.buildRunEngine(rs)
		if err != nil {
			return err
		}
	}
	if eng == nil {
		return errEngineUnavailable
	}

	// Snapshot the run parameters from the state's config under the server
	// lock so a concurrent config save can never race these reads.
	s.mu.RLock()
	cfg := rs.cfg
	if cfg == nil {
		cfg = s.cfg
	}
	var applyConsent bool
	var maxPerRun, minDelay int
	if cfg != nil {
		applyConsent = cfg.ApplyConsent
		maxPerRun = cfg.MaxAppsPerRun
		minDelay = cfg.ApplyDelaySec
	}
	s.mu.RUnlock()

	rs.status = StatusRunning
	rs.errMsg = ""
	rs.dryRun = dryRun
	rs.autoApply = autoApply
	rs.lastJob = ""
	rs.foundCount = 0
	rs.liveFeed = make([]DashRecent, 0)
	rs.recent = make([]DashRecent, 0)
	rs.providerProgress = make(map[string]ProviderStatus)
	rs.eng = eng
	rs.cancel = nil

	eng.Reset()
	eng.DryRun = dryRun
	eng.AutoApply = autoApply && applyConsent
	eng.MaxPerRun = maxPerRun
	if eng.MaxPerRun <= 0 {
		eng.MaxPerRun = 10
	}
	eng.MinDelay = minDelay
	if eng.MinDelay <= 0 {
		eng.MinDelay = 8
	}

	ctx, cancel := context.WithCancel(context.Background())
	rs.cancel = cancel

	if run == nil {
		run = func(runCtx context.Context) error { return eng.RunOnce(runCtx) }
	}
	runFn := run

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				rs.mu.Lock()
				rs.status = StatusError
				rs.errMsg = "engine panic"
				rs.mu.Unlock()
				rs.changed()
			}
		}()

		go s.drainEngineChannels(ctx, eng, rs)
		runErr := runFn(ctx)

		rs.mu.Lock()
		if runErr != nil {
			rs.status = StatusError
			rs.errMsg = runErr.Error()
		} else {
			rs.status = StatusDone
		}
		rs.mu.Unlock()
		rs.changed()
	}()

	rs.changed()
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

	rs := s.runFor(r)
	if err := s.launchRun(rs, nil, input.DryRun, input.AutoApply, nil); err != nil {
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
	rs := s.runFor(r)
	rs.mu.Lock()
	busy := rs.status == StatusRunning
	rs.mu.Unlock()
	if busy {
		writeError(w, http.StatusConflict, errEngineBusy.Error())
		return
	}
	eng, err := s.buildRunEngine(rs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if eng == nil {
		writeError(w, http.StatusInternalServerError, errEngineUnavailable.Error())
		return
	}
	if err := s.launchRun(rs, eng, false, true, func(ctx context.Context) error {
		return eng.ApplySelected(ctx, ids)
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
	rs := s.runFor(r)
	rs.mu.Lock()
	if rs.cancel != nil {
		rs.cancel()
		rs.cancel = nil
	}
	if rs.status == StatusRunning {
		rs.status = StatusStopped
	}
	rs.mu.Unlock()
	rs.changed()

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// drainEngineChannels reads from the engine's channels and updates the run
// state. Called once per run with the run's engine and state.
func (s *Server) drainEngineChannels(ctx context.Context, eng *engine.Engine, rs *runState) {
	if eng == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case p, ok := <-eng.ProgressCh:
			if !ok {
				return
			}
			rs.mu.Lock()
			rs.providerProgress[p.Provider] = ProviderStatus{
				Status: p.Status,
				Count:  p.Count,
				ErrMsg: p.ErrMsg,
			}
			rs.mu.Unlock()
			rs.changed()

		case res, ok := <-eng.ResultCh:
			if !ok {
				return
			}
			rs.mu.Lock()
			rs.lastJob = res.Job.Title + " @ " + res.Job.Company
			rs.liveFeed = append(rs.liveFeed, DashRecent{
				Label:  res.Job.Title + " @ " + res.Job.Company,
				Status: res.Status,
			})
			rs.recent = append(rs.recent, DashRecent{
				Label:  res.Job.Title + " @ " + res.Job.Company,
				Status: res.Status,
			})
			if len(rs.liveFeed) > 100 {
				rs.liveFeed = rs.liveFeed[len(rs.liveFeed)-50:]
			}
			if len(rs.recent) > 10 {
				rs.recent = rs.recent[len(rs.recent)-10:]
			}
			switch res.Status {
			case "found":
				rs.foundCount++
			case "applied":
				rs.applied++
			case "skipped":
				rs.skipped++
			case "failed":
				rs.failed++
			}
			rs.mu.Unlock()
			rs.changed()

		case l, ok := <-eng.LogCh:
			if !ok {
				return
			}
			rs.appendLog(l)
		}
	}
}
