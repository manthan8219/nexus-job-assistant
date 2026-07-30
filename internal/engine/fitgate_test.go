package engine

import "testing"

func TestFitGateBlocked(t *testing.T) {
	tests := []struct {
		name       string
		score, min int
		want       bool
	}{
		{"gate off (min 0)", 30, 0, false},
		{"below threshold blocks", 40, 65, true},
		{"at threshold passes", 65, 65, false},
		{"above threshold passes", 90, 65, false},
		{"unscored never blocks", 0, 65, false},
		{"unscored with gate off", 0, 0, false},
		{"perfect score passes", 100, 70, false},
		{"one below blocks", 64, 65, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitGateBlocked(tt.score, tt.min); got != tt.want {
				t.Errorf("fitGateBlocked(%d, %d) = %v; want %v", tt.score, tt.min, got, tt.want)
			}
		})
	}
}
