package engine

import (
	"context"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

func TestTailorHighFit(t *testing.T) {
	tests := []struct {
		name       string
		score, min int
		want       bool
	}{
		{"unscored never tailors", 0, 0, false},
		{"unscored with floor never tailors", 0, 70, false},
		{"no floor any positive score", 40, 0, true},
		{"at floor tailors", 70, 70, true},
		{"below floor does not tailor", 65, 70, false},
		{"above floor tailors", 92, 70, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tailorHighFit(tt.score, tt.min); got != tt.want {
				t.Errorf("tailorHighFit(%d, %d) = %v; want %v", tt.score, tt.min, got, tt.want)
			}
		})
	}
}

// TailorPerJob off must leave the profile untouched.
func TestTailorBeforeApply_Disabled(t *testing.T) {
	e := newTestEngine(t, &config.Config{TailorPerJob: false, AIAssist: false})
	in := provider.Profile{ResumePath: "/base/resume.pdf"}
	out := e.tailorBeforeApply(context.Background(), jobFor("fake"), 90, in)
	if out.ResumePath != in.ResumePath {
		t.Errorf("ResumePath = %q; want %q (unchanged when tailoring off)", out.ResumePath, in.ResumePath)
	}
}

// TailorPerJob on with AI off must fail open: no score, no tailoring, and the
// original profile is used — an application must never be blocked.
func TestTailorBeforeApply_AIOffFailsOpen(t *testing.T) {
	e := newTestEngine(t, &config.Config{TailorPerJob: true, AIAssist: false})
	in := provider.Profile{ResumePath: "/base/resume.pdf"}
	out := e.tailorBeforeApply(context.Background(), jobFor("fake"), 0, in)
	if out.ResumePath != in.ResumePath {
		t.Errorf("ResumePath = %q; want %q (fail open with AI off)", out.ResumePath, in.ResumePath)
	}
}

// TailorPerJob on but low fit: a score below the floor must skip tailoring.
func TestTailorBeforeApply_LowFitSkips(t *testing.T) {
	e := newTestEngine(t, &config.Config{TailorPerJob: true, AIAssist: false, MinFitScore: 80})
	in := provider.Profile{ResumePath: "/base/resume.pdf"}
	out := e.tailorBeforeApply(context.Background(), jobFor("fake"), 40, in)
	if out.ResumePath != in.ResumePath {
		t.Errorf("ResumePath = %q; want %q (low fit must skip tailoring)", out.ResumePath, in.ResumePath)
	}
}

// End-to-end: TailorPerJob with AI off must not change the apply flow — the
// fake provider still gets exactly one Apply with the base resume.
func TestProcessJob_TailorAIOffStillApplies(t *testing.T) {
	fp := &fakeProvider{name: "fake"}
	e := newTestEngine(t, &config.Config{ApplyConsent: true, AIAssist: false, TailorPerJob: true})
	e.AutoApply = true
	e.DryRun = false
	e.providers = []provider.Provider{fp}
	e.syncApplySafety()
	job := jobFor("fake")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = runProcessJob(t, ctx, e, job)
	if fp.applyN != 1 {
		t.Errorf("tailoring must not block the apply; got %d Apply calls", fp.applyN)
	}
	if len(fp.applied) != 1 {
		t.Fatalf("expected one recorded apply; got %d", len(fp.applied))
	}
	if fp.applied[0].Company != "Acme" {
		t.Errorf("unexpected applied job: %+v", fp.applied[0])
	}
}
