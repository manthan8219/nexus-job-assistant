package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestReplyProbability(t *testing.T) {
	tests := []struct {
		name                          string
		applied, replied, intv, offer int
		want                          int
	}{
		{"no applications scores zero", 0, 1, 0, 0, 0},
		{"zero applied with responses clamps to zero", 0, 0, 0, 0, 0},
		{"no responses scores zero", 10, 0, 0, 0, 0},
		{"all responses scores one hundred", 4, 2, 1, 1, 100},
		{"half respond", 10, 3, 1, 1, 50},
		{"everyone replies", 3, 3, 0, 0, 100},
		{"one of four", 4, 1, 0, 0, 25},
		{"cannot exceed one hundred", 2, 3, 0, 0, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := replyProbability(tt.applied, tt.replied, tt.intv, tt.offer); got != tt.want {
				t.Errorf("replyProbability(%d,%d,%d,%d) = %d; want %d", tt.applied, tt.replied, tt.intv, tt.offer, got, tt.want)
			}
		})
	}
}

// Per-provider and overall response-probability scores flow into the snapshot.
func TestAnalyticsSnapshot_ReplyScores(t *testing.T) {
	st, err := OpenPath(filepath.Join(t.TempDir(), "reply.db"))
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer st.Close()
	now := time.Now()
	seed := func(provider, url, status string, outcome Outcome) {
		if err := st.Insert(Application{
			Provider: provider, Company: "Acme", Role: "Engineer",
			URL: url, Status: Status(status), AppliedAt: now, Outcome: outcome,
		}); err != nil {
			t.Fatalf("seed %s: %v", url, err)
		}
	}
	seed("greenhouse", "g1", "applied", OutcomeReplied)
	seed("greenhouse", "g2", "applied", OutcomeNone)
	seed("lever", "l1", "applied", OutcomeInterview)
	seed("remoteok", "r1", "failed", OutcomeNone)

	snap, err := st.AnalyticsSnapshot()
	if err != nil {
		t.Fatalf("AnalyticsSnapshot: %v", err)
	}

	scores := map[string]int{}
	for _, py := range snap.PerProvider {
		scores[py.Provider] = py.ReplyProbability
	}
	if scores["greenhouse"] != 50 {
		t.Errorf("greenhouse replyProbability = %d; want 50", scores["greenhouse"])
	}
	if scores["lever"] != 100 {
		t.Errorf("lever replyProbability = %d; want 100", scores["lever"])
	}
	if scores["remoteok"] != 0 {
		t.Errorf("remoteok replyProbability = %d; want 0 (nothing applied)", scores["remoteok"])
	}
	// Overall: 2 responses out of 3 applications → 66.
	if snap.ResponseProbability != 66 {
		t.Errorf("overall responseProbability = %d; want 66", snap.ResponseProbability)
	}
}
