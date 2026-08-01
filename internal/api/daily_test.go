package api

import (
	"testing"
	"time"
)

func TestShouldFireDaily(t *testing.T) {
	at := "09:30"
	day := time.Date(2026, 7, 31, 10, 0, 0, 0, time.Local)
	before := time.Date(2026, 7, 31, 9, 29, 0, 0, time.Local)

	tests := []struct {
		name      string
		now       time.Time
		at        string
		lastFired string
		enabled   bool
		busy      bool
		want      bool
	}{
		{name: "due", now: day, at: at, enabled: true, want: true},
		{name: "before the time", now: before, at: at, enabled: true, want: false},
		{name: "already fired today", now: day, at: at, lastFired: "2026-07-31", enabled: true, want: false},
		{name: "disabled", now: day, at: at, enabled: false, want: false},
		{name: "no time configured", now: day, at: "", enabled: true, want: false},
		{name: "busy", now: day, at: at, enabled: true, busy: true, want: false},
		{name: "fires the next day", now: time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local), at: at, lastFired: "2026-07-31", enabled: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldFireDaily(tt.now, tt.at, tt.lastFired, tt.enabled, tt.busy)
			if got != tt.want {
				t.Errorf("shouldFireDaily() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestParseHHMM(t *testing.T) {
	tests := []struct {
		in           string
		wantH, wantM int
	}{
		{in: "09:30", wantH: 9, wantM: 30},
		{in: "00:00", wantH: 0, wantM: 0},
		{in: "23:59", wantH: 23, wantM: 59},
		{in: "24:00", wantH: 0, wantM: 0},
		{in: "9:30", wantH: 9, wantM: 30},
		{in: "bogus", wantH: 0, wantM: 0},
	}
	for _, tt := range tests {
		h, m := parseHHMM(tt.in)
		if h != tt.wantH || m != tt.wantM {
			t.Errorf("parseHHMM(%q) = %d:%d; want %d:%d", tt.in, h, m, tt.wantH, tt.wantM)
		}
	}
}
