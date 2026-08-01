package outreach

import (
	"testing"
	"time"
)

func TestWarmupCap(t *testing.T) {
	tests := []struct {
		name       string
		daysActive int
		rampDays   int
		maxCap     int
		want       int
	}{
		{"ramp disabled returns full cap", 1, 0, 10, 10},
		{"ramp disabled with maxCap zero", 1, 0, 0, 0},
		{"day one small", 1, 5, 10, 2},
		{"mid ramp", 3, 5, 10, 6},
		{"full cap reached at ramp end", 5, 5, 10, 10},
		{"beyond ramp stays full", 9, 5, 10, 10},
		{"zero daysActive clamped to one", 0, 5, 10, 2},
		{"larger cap linear", 5, 10, 25, 12},
		{"never below one", 1, 30, 25, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := warmupCap(tt.daysActive, tt.rampDays, tt.maxCap); got != tt.want {
				t.Errorf("warmupCap(%d, %d, %d) = %d; want %d", tt.daysActive, tt.rampDays, tt.maxCap, got, tt.want)
			}
		})
	}
}

func TestSendingDaysActive(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		items []Item
		want  int
	}{
		{"no sent items is day one", nil, 1},
		{"only linkedin ignores", []Item{{Channel: ChannelLinkedIn, SentAt: now}}, 1},
		{"sent today is day one", []Item{{Channel: ChannelEmail, SentAt: now}}, 1},
		{"sent yesterday is day two", []Item{{Channel: ChannelEmail, SentAt: now.Add(-24 * time.Hour)}}, 2},
		{"earliest sent wins", []Item{
			{Channel: ChannelEmail, SentAt: now.Add(-48 * time.Hour)},
			{Channel: ChannelEmail, SentAt: now},
		}, 3},
		{"future sent clamps to one", []Item{{Channel: ChannelEmail, SentAt: now.Add(72 * time.Hour)}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sendingDaysActive(tt.items, now); got != tt.want {
				t.Errorf("sendingDaysActive() = %d; want %d", got, tt.want)
			}
		})
	}
}
