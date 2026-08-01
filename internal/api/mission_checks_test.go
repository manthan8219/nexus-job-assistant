package api

import (
	"strings"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// completeConfig mirrors a fully onboarded profile (minus AI) so the tests
// exercise the "ready to run" branch of the mission snapshot.
func completeConfig(aiAssist bool) *config.Config {
	return &config.Config{
		FirstName:       "Ada",
		LastName:        "Lovelace",
		Email:           "ada@example.com",
		ResumePath:      "/tmp/resume.pdf",
		TargetJobTitles: "Engineer",
		TargetLocations: "Remote",
		ApplyConsent:    true,
		AIAssist:        aiAssist,
	}
}

func findCheck(checks []ReadyCheck, key string) *ReadyCheck {
	for i := range checks {
		if checks[i].Key == key {
			return &checks[i]
		}
	}
	return nil
}

func TestMissionChecks_AIAssistOptionalAndOff(t *testing.T) {
	s := New(completeConfig(false), nil, nil, "")
	snap := s.missionSnapshot()

	ai := findCheck(snap.Checks, "ai-assist")
	if ai == nil {
		t.Fatal("expected an ai-assist ReadyCheck in the mission snapshot")
	}
	if ai.OK {
		t.Errorf("ai-assist OK = true with AIAssist off; want false")
	}
	if !ai.Optional {
		t.Errorf("ai-assist Optional = false; want true (AI is recommended, not required)")
	}

	// AI Assist must not block onboarding completion.
	if !snap.OnboardingComplete {
		t.Errorf("OnboardingComplete = false; want true (AI Assist is optional)")
	}

	// Once the profile basics are done, the guided next action nudges toward
	// AI Assist so new users don't silently miss it.
	if !strings.Contains(snap.NextAction, "turn on AI Assist") {
		t.Errorf("NextAction = %q; want an AI Assist nudge", snap.NextAction)
	}
}

func TestMissionChecks_AIAssistOn(t *testing.T) {
	s := New(completeConfig(true), nil, nil, "")
	snap := s.missionSnapshot()

	ai := findCheck(snap.Checks, "ai-assist")
	if ai == nil {
		t.Fatal("expected an ai-assist ReadyCheck in the mission snapshot")
	}
	if !ai.OK {
		t.Errorf("ai-assist OK = false; want true when AIAssist is on")
	}
	if want := "Start a run to search and apply"; snap.NextAction != want {
		t.Errorf("NextAction = %q; want %q when AI Assist is on", snap.NextAction, want)
	}
}

func TestMissionNextAction_NoAIAssistNudgeWhenProfileIncomplete(t *testing.T) {
	// Missing target titles → the nudge stays hidden; the profile must be
	// ready first so the AI hint doesn't clutter the earlier onboarding steps.
	s := New(&config.Config{FirstName: "Ada", Email: "ada@example.com"}, nil, nil, "")
	snap := s.missionSnapshot()

	if strings.Contains(snap.NextAction, "turn on AI Assist") {
		t.Errorf("NextAction = %q; want no AI Assist nudge while the profile is incomplete", snap.NextAction)
	}
	if snap.OnboardingComplete {
		t.Errorf("OnboardingComplete = true for an incomplete profile; want false")
	}
}
