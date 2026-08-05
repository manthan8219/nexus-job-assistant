package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/engine"
)

// MissionSnapshot mirrors the frontend's MissionSnapshot type.
type MissionSnapshot struct {
	EngineStatus       string                    `json:"engineStatus"`
	LastJob            string                    `json:"lastJob"`
	ErrMsg             string                    `json:"errMsg"`
	DryRun             bool                      `json:"dryRun"`
	AutoApply          bool                      `json:"autoApply"`
	HasConsent         bool                      `json:"hasConsent"`
	Applied            int                       `json:"applied"`
	Skipped            int                       `json:"skipped"`
	Failed             int                       `json:"failed"`
	AppliedToday       int                       `json:"appliedToday"`
	MaxPerDay          int                       `json:"maxPerDay"`
	ResumePath         string                    `json:"resumePath"`
	Checks             []ReadyCheck              `json:"checks"`
	ResumeReady        bool                      `json:"resumeReady"`
	HasTitles          bool                      `json:"hasTitles"`
	AIOn               bool                      `json:"aiOn"`
	OnboardingComplete bool                      `json:"onboardingComplete"`
	ModeName           string                    `json:"modeName"`
	ModeHint           string                    `json:"modeHint"`
	NextAction         string                    `json:"nextAction"`
	Providers          []string                  `json:"providers"`
	Progress           map[string]ProviderStatus `json:"progress"`
	FoundCount         int                       `json:"foundCount"`
	LiveFeed           []DashRecent              `json:"liveFeed"`
	Recent             []DashRecent              `json:"recent"`
}

// ReadyCheck mirrors the frontend ReadyCheck type. Optional checks (like AI
// Assist) inform the user but never block onboarding completion.
type ReadyCheck struct {
	Key      string `json:"key"`
	OK       bool   `json:"ok"`
	Label    string `json:"label"`
	Hint     string `json:"hint"`
	Optional bool   `json:"optional,omitempty"`
}

// handleGetMission returns the full dashboard snapshot for the requesting
// user (or the legacy state when auth is off).
func (s *Server) handleGetMission(w http.ResponseWriter, r *http.Request) {
	rs := s.runFor(r)
	writeJSON(w, http.StatusOK, s.missionSnapshotFor(rs))
}

// missionSnapshot is the legacy single-user snapshot (tests + non-auth mode).
func (s *Server) missionSnapshot() MissionSnapshot {
	return s.missionSnapshotFor(&s.runState)
}

// missionSnapshotFor builds the full dashboard snapshot the frontend renders
// for one run state (per-user in multi-tenant mode, legacy otherwise).
func (s *Server) missionSnapshotFor(rs *runState) MissionSnapshot {
	s.mu.RLock()
	cfg := rs.cfg
	if cfg == nil {
		cfg = s.cfg
	}
	s.mu.RUnlock()

	rs.mu.RLock()
	status := rs.status
	errMsg := rs.errMsg
	dryRun := rs.dryRun
	autoApply := rs.autoApply
	applied := rs.applied
	skipped := rs.skipped
	failed := rs.failed
	appliedToday := 0
	lastJob := rs.lastJob
	providers := s.providerList(rs)
	progress := copyMap(rs.providerProgress)
	foundCount := rs.foundCount
	liveFeed := rs.liveFeed
	recent := rs.recent
	rs.mu.RUnlock()

	resumePath := ""
	aiOn := false
	hasTitles := false
	hasConsent := false
	if cfg != nil {
		resumePath = cfg.ResumePath
		aiOn = cfg.AIAssist
		hasTitles = cfg.TargetJobTitles != ""
		hasConsent = cfg.ApplyConsent
	}

	// If idle, pull stats from the store.
	apps := rs.apps
	if apps == nil {
		apps = s.store
	}
	if status == StatusIdle && apps != nil {
		a, sk, f, _ := apps.Stats()
		applied = a
		skipped = sk
		failed = f
		today, _ := apps.CountAppliedSince(time.Now().Truncate(24 * time.Hour))
		appliedToday = today
	}

	maxPerDay := 25
	if cfg != nil && cfg.MaxAppsPerDay > 0 {
		maxPerDay = cfg.MaxAppsPerDay
	}

	checks := s.buildChecks(cfg)
	onboardingComplete := true
	for _, c := range checks {
		if c.Optional {
			continue // recommended, not required — never blocks completion
		}
		if !c.OK {
			onboardingComplete = false
			break
		}
	}

	modeName := "Search & Apply"
	modeHint := "Search all boards, score, and apply"
	nextAction := "Configure your profile first"
	if cfg != nil && cfg.FirstName != "" && cfg.Email != "" && hasTitles {
		switch status {
		case StatusIdle:
			nextAction = "Start a run to search and apply"
			// AI Assist is recommended, not required: once the profile
			// basics are done, nudge users who never turned it on so they
			// don't miss fit-scoring and tailored answers.
			if !cfg.AIAssist {
				nextAction = "Next: turn on AI Assist to unlock fit-scoring and tailored answers"
			}
		case StatusRunning:
			nextAction = "Run in progress"
		default:
			nextAction = "Run complete — review results"
		}
	}

	return MissionSnapshot{
		EngineStatus:       string(status),
		LastJob:            lastJob,
		ErrMsg:             errMsg,
		DryRun:             dryRun,
		AutoApply:          autoApply,
		HasConsent:         hasConsent,
		Applied:            applied,
		Skipped:            skipped,
		Failed:             failed,
		AppliedToday:       appliedToday,
		MaxPerDay:          maxPerDay,
		ResumePath:         resumePath,
		Checks:             checks,
		ResumeReady:        resumePath != "",
		HasTitles:          hasTitles,
		AIOn:               aiOn,
		OnboardingComplete: onboardingComplete,
		ModeName:           modeName,
		ModeHint:           modeHint,
		NextAction:         nextAction,
		Providers:          providers,
		Progress:           progress,
		FoundCount:         foundCount,
		LiveFeed:           liveFeed,
		Recent:             recent,
	}
}

// handleStreamMission streams the dashboard snapshot over Server-Sent Events.
// One event is sent immediately on connect, then again on every state change,
// and periodically as a heartbeat so the connection stays alive and a client
// that missed a wake-up always re-syncs to the latest state. Every event is a
// complete snapshot, so events are idempotent and a missed frame is harmless.
func (s *Server) handleStreamMission(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	rs := s.runFor(r)

	sub := rs.subscribe()
	defer rs.unsubscribe(sub)

	if err := writeMissionEvent(w, s.missionSnapshotFor(rs)); err != nil {
		return
	}
	flusher.Flush()

	var heartbeatCh <-chan time.Time
	if s.sseHeartbeat > 0 {
		ticker := time.NewTicker(s.sseHeartbeat)
		defer ticker.Stop()
		heartbeatCh = ticker.C
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-sub:
			if err := writeMissionEvent(w, s.missionSnapshotFor(rs)); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeatCh:
			if err := writeMissionEvent(w, s.missionSnapshotFor(rs)); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeMissionEvent writes one SSE frame carrying the full snapshot. JSON
// escaping guarantees the payload contains no raw newlines, so it fits on a
// single "data:" line as the SSE spec requires.
func writeMissionEvent(w io.Writer, snap MissionSnapshot) error {
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", b)
	return err
}

// providerList returns the list of provider names from the run state's engine.
func (s *Server) providerList(rs *runState) []string {
	var eng *engine.Engine
	if rs.perUser {
		eng = rs.eng
	} else {
		eng = s.eng
	}
	if eng == nil {
		return []string{}
	}
	return eng.ProviderNames()
}

// buildChecks returns readiness checks for the onboarding card.
func (s *Server) buildChecks(cfg *config.Config) []ReadyCheck {
	if cfg == nil {
		return []ReadyCheck{}
	}
	return []ReadyCheck{
		{Key: "name", OK: cfg.FirstName != "" && cfg.LastName != "",
			Label: "Full name", Hint: "Fill your name in Config"},
		{Key: "email", OK: cfg.Email != "",
			Label: "Email address", Hint: "Fill your email in Config"},
		{Key: "resume", OK: cfg.ResumePath != "",
			Label: "Resume path", Hint: "Set resume path in Config"},
		{Key: "titles", OK: cfg.TargetJobTitles != "",
			Label: "Job titles", Hint: "Set target job titles in Config"},
		{Key: "locations", OK: cfg.TargetLocations != "",
			Label: "Locations", Hint: "Set target locations in Config"},
		{Key: "consent", OK: cfg.ApplyConsent,
			Label: "Apply consent", Hint: "Grant consent in Config"},
		{Key: "ai-assist", OK: cfg.AIAssist, Optional: true,
			Label: "AI Assist on",
			Hint:  "AI Assist off — optional: fit-scoring & tailored answers"},
	}
}

func copyMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
