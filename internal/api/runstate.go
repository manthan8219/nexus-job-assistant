// Per-run mutable state: one state for the legacy single-user server, and one
// per authenticated user in multi-tenant mode, so concurrent tenants never
// observe or mutate each other's engine runs, log buffers, or mission streams.
// Config reads/writes stay guarded by the server's s.mu (as before); the
// runState.mu guards the run lifecycle, counters, logs, and SSE subscribers.

package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/engine"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// runState is one run pipeline's mutable state: its engine, lifecycle status,
// live counters, log buffer, and mission-stream subscribers. Legacy
// single-user mode has a single state embedded in the Server; auth mode has
// one state per user (Server.runs keyed by user ID).
type runState struct {
	mu sync.RWMutex

	perUser bool // true when owned by a per-user island (multi-tenant mode)
	eng     *engine.Engine
	cfg     *config.Config // config this state runs against (island or legacy)
	apps    *store.Store   // store this state runs against (island or legacy)

	status    RunStatus
	errMsg    string
	dryRun    bool
	autoApply bool
	lastJob   string
	lastJobAt time.Time

	providerProgress map[string]ProviderStatus
	foundCount       int
	liveFeed         []DashRecent
	recent           []DashRecent
	applied          int
	skipped          int
	failed           int
	logLines         []string
	cancel           context.CancelFunc

	notifyMu    sync.Mutex
	subscribers map[chan struct{}]struct{} // mission-stream wake-up channels
}

// ensureInit makes a lazily-created (zero-value) state safe to use, filling
// only fields that are still nil so already-populated buffers are never wiped.
func (rs *runState) ensureInit() {
	rs.mu.Lock()
	if rs.providerProgress == nil {
		rs.providerProgress = make(map[string]ProviderStatus)
	}
	if rs.liveFeed == nil {
		rs.liveFeed = make([]DashRecent, 0)
	}
	if rs.recent == nil {
		rs.recent = make([]DashRecent, 0)
	}
	if rs.logLines == nil {
		rs.logLines = make([]string, 0)
	}
	if rs.subscribers == nil {
		rs.subscribers = make(map[chan struct{}]struct{})
	}
	rs.mu.Unlock()
}

// subscribe registers a wake-up channel for this state's mission stream.
func (rs *runState) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	rs.notifyMu.Lock()
	if rs.subscribers == nil {
		rs.subscribers = make(map[chan struct{}]struct{})
	}
	rs.subscribers[ch] = struct{}{}
	rs.notifyMu.Unlock()
	return ch
}

// unsubscribe removes a mission-stream subscriber.
func (rs *runState) unsubscribe(ch chan struct{}) {
	rs.notifyMu.Lock()
	delete(rs.subscribers, ch)
	rs.notifyMu.Unlock()
}

// changed wakes every subscriber so it pushes a fresh snapshot. Best-effort:
// snapshots are full-state, so a dropped wake-up is safe, and the heartbeat
// re-syncs any subscriber that fell behind.
func (rs *runState) changed() {
	rs.notifyMu.Lock()
	for ch := range rs.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	rs.notifyMu.Unlock()
}

// appendLog appends a line to the state's capped in-memory log buffer.
func (rs *runState) appendLog(line string) {
	rs.mu.Lock()
	if len(rs.logLines) > 1000 {
		rs.logLines = rs.logLines[len(rs.logLines)-500:]
	}
	rs.logLines = append(rs.logLines, line)
	rs.mu.Unlock()
}

// runFor returns the run state scoped to this request: the per-user state in
// multi-tenant mode (created lazily, starting that user's daily + inbox
// scheduler loops), or the embedded legacy state otherwise.
func (s *Server) runFor(r *http.Request) *runState {
	if st := s.userState(r); st != nil {
		s.runsMu.Lock()
		if rs, ok := s.runs[st.UserID]; ok {
			s.runsMu.Unlock()
			return rs
		}
		rs := &runState{perUser: true, cfg: st.Cfg, apps: st.Apps, status: StatusIdle}
		rs.ensureInit()
		s.runs[st.UserID] = rs
		s.runsMu.Unlock()
		if s.loopCtx != nil {
			go s.scheduleDailyRuns(s.loopCtx, rs)
			go s.scheduleInboxScan(s.loopCtx, rs)
		}
		return rs
	}
	s.runState.ensureInit()
	return &s.runState
}

// buildRunEngine returns the engine a run on rs should use. Legacy mode hands
// out the server's engine; per-user states build a fresh engine from the
// user's own config + store under the server read-lock, so a concurrent config
// save can never race the build and provider keys always reflect the latest
// saved config.
func (s *Server) buildRunEngine(rs *runState) (*engine.Engine, error) {
	if rs.perUser {
		if rs.cfg == nil || rs.apps == nil {
			return nil, errEngineUnavailable
		}
		s.mu.RLock()
		cfgSnapshot := *rs.cfg // shallow copy: config writers assign, never mutate
		st := rs.apps
		s.mu.RUnlock()
		return engine.New(&cfgSnapshot, st, "")
	}
	return s.eng, nil
}

// Legacy single-user aliases — kept so existing tests and the non-auth code
// paths keep reading the embedded state directly.
func (s *Server) changed()                     { s.runState.changed() }
func (s *Server) subscribe() chan struct{}     { return s.runState.subscribe() }
func (s *Server) unsubscribe(ch chan struct{}) { s.runState.unsubscribe(ch) }
func (s *Server) logLine(line string)          { s.runState.appendLog(line) }
