package api

import (
	"net/http"
	"time"
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

// ReadyCheck mirrors the frontend ReadyCheck type.
type ReadyCheck struct {
	Key   string `json:"key"`
	OK    bool   `json:"ok"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// handleGetMission returns the full dashboard snapshot.
func (s *Server) handleGetMission(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	status := s.status
	errMsg := s.errMsg
	dryRun := s.dryRun
	autoApply := s.autoApply
	applied := s.applied
	skipped := s.skipped
	failed := s.failed
	appliedToday := s.appliedToday
	lastJob := s.lastJob
	providers := s.providerList()
	progress := copyMap(s.providerProgress)
	foundCount := s.foundCount
	liveFeed := s.liveFeed
	recent := s.recent
	resumePath := ""
	aiOn := false
	hasTitles := false
	hasConsent := false
	if s.cfg != nil {
		resumePath = s.cfg.ResumePath
		aiOn = s.cfg.AIAssist
		hasTitles = s.cfg.TargetJobTitles != ""
		hasConsent = s.cfg.ApplyConsent
	}
	s.mu.RUnlock()

	// If idle, pull stats from the store
	if status == StatusIdle && s.store != nil {
		a, sk, f, _ := s.store.Stats()
		applied = a
		skipped = sk
		failed = f
		today, _ := s.store.CountAppliedSince(time.Now().Truncate(24 * time.Hour))
		appliedToday = today
	}

	maxPerDay := 25
	if s.cfg != nil && s.cfg.MaxAppsPerDay > 0 {
		maxPerDay = s.cfg.MaxAppsPerDay
	}

	checks := s.buildChecks()
	onboardingComplete := true
	for _, c := range checks {
		if !c.OK {
			onboardingComplete = false
			break
		}
	}

	modeName := "Search & Apply"
	modeHint := "Search all boards, score, and apply"
	nextAction := "Configure your profile first"
	if s.cfg != nil && s.cfg.FirstName != "" && s.cfg.Email != "" && hasTitles {
		switch status {
		case StatusIdle:
			nextAction = "Start a run to search and apply"
		case StatusRunning:
			nextAction = "Run in progress"
		default:
			nextAction = "Run complete — review results"
		}
	}

	snapshot := MissionSnapshot{
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

	writeJSON(w, http.StatusOK, snapshot)
}

// providerList returns the list of provider names from the engine.
func (s *Server) providerList() []string {
	if s.eng == nil {
		return []string{}
	}
	return s.eng.ProviderNames()
}

// buildChecks returns readiness checks for the onboarding card.
func (s *Server) buildChecks() []ReadyCheck {
	if s.cfg == nil {
		return []ReadyCheck{}
	}
	return []ReadyCheck{
		{Key: "name", OK: s.cfg.FirstName != "" && s.cfg.LastName != "",
			Label: "Full name", Hint: "Fill your name in Config"},
		{Key: "email", OK: s.cfg.Email != "",
			Label: "Email address", Hint: "Fill your email in Config"},
		{Key: "resume", OK: s.cfg.ResumePath != "",
			Label: "Resume path", Hint: "Set resume path in Config"},
		{Key: "titles", OK: s.cfg.TargetJobTitles != "",
			Label: "Job titles", Hint: "Set target job titles in Config"},
		{Key: "locations", OK: s.cfg.TargetLocations != "",
			Label: "Locations", Hint: "Set target locations in Config"},
		{Key: "consent", OK: s.cfg.ApplyConsent,
			Label: "Apply consent", Hint: "Grant consent in Config"},
	}
}

func copyMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
