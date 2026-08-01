package api

import (
	"testing"
	"time"
)

func TestResponseScoreFor(t *testing.T) {
	now := time.Now()
	hourAgo := now.Add(-time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)
	monthAgo := now.Add(-31 * 24 * time.Hour)

	tests := []struct {
		name          string
		fit           int
		postedAt      time.Time
		providerReply int
		wantMin       int
		wantMax       int
	}{
		{"fresh + high fit + strong provider is high", 95, hourAgo, 100, 70, 100},
		{"no history degrades gracefully", 0, hourAgo, 0, 20, 50},
		{"zero fit + stale posting stays low", 0, monthAgo, 0, 0, 30},
		{"zero time never scores zero with freshness", 50, now, 0, 30, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, summary := responseScoreFor(tt.fit, tt.postedAt, tt.providerReply)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("score = %d; want in [%d,%d] (summary %q)", score, tt.wantMin, tt.wantMax, summary)
			}
			if score > 0 && summary == "" {
				t.Errorf("positive score must carry a why line")
			}
		})
	}

	// Clamped to 0-100 even with extreme inputs.
	if score, _ := responseScoreFor(10000, weekAgo, 10000); score > 100 {
		t.Errorf("score = %d; want <= 100", score)
	}
	if score, _ := responseScoreFor(-10, time.Time{}, -5); score < 0 {
		t.Errorf("score = %d; want >= 0", score)
	}

	// Deterministic: identical inputs give identical scores.
	a, _ := responseScoreFor(80, hourAgo, 50)
	b, _ := responseScoreFor(80, hourAgo, 50)
	if a != b {
		t.Errorf("scores not deterministic: %d != %d", a, b)
	}
}
