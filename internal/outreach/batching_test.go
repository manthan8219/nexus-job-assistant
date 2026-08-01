package outreach

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestBatchSize(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want int
	}{
		{"nil config defaults", nil, 5},
		{"zero defaults", &config.Config{}, 5},
		{"configured", &config.Config{OutreachBatchSize: 3}, 3},
	}
	for _, tt := range tests {
		if got := BatchSize(tt.cfg); got != tt.want {
			t.Errorf("BatchSize(%v) = %d; want %d", tt.cfg, got, tt.want)
		}
	}
}

func TestBatchPause(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want time.Duration
	}{
		{"nil config defaults", nil, 60 * time.Second},
		{"zero defaults", &config.Config{}, 60 * time.Second},
		{"configured", &config.Config{OutreachBatchPauseSec: 15}, 15 * time.Second},
	}
	for _, tt := range tests {
		if got := BatchPause(tt.cfg); got != tt.want {
			t.Errorf("BatchPause(%v) = %v; want %v", tt.cfg, got, tt.want)
		}
	}
}

func TestRelayEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantAddr    string
		wantHost    string
		wantFrom    string
		wantEnabled bool
	}{
		{"nil config disabled", nil, "", "", "", false},
		{"empty host disabled", &config.Config{Email: "me@gmail.com"}, "", "", "", false},
		{
			"default port and from fallback",
			&config.Config{SmtpRelayHost: "relay.example.com", Email: "me@gmail.com"},
			"relay.example.com:587", "relay.example.com", "me@gmail.com", true,
		},
		{
			"custom port",
			&config.Config{SmtpRelayHost: "relay.example.com", SmtpRelayPort: 465, Email: "me@gmail.com"},
			"relay.example.com:465", "relay.example.com", "me@gmail.com", true,
		},
		{
			"custom from wins",
			&config.Config{SmtpRelayHost: "relay.example.com", SmtpRelayFrom: "noreply@mydomain.com", Email: "me@gmail.com"},
			"relay.example.com:587", "relay.example.com", "noreply@mydomain.com", true,
		},
		{
			"port clamped when negative",
			&config.Config{SmtpRelayHost: "relay.example.com", SmtpRelayPort: -1, Email: "me@gmail.com"},
			"relay.example.com:587", "relay.example.com", "me@gmail.com", true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, host, from, enabled := relayEndpoint(tt.cfg)
			if addr != tt.wantAddr || host != tt.wantHost || from != tt.wantFrom || enabled != tt.wantEnabled {
				t.Errorf("relayEndpoint() = (%q, %q, %q, %v); want (%q, %q, %q, %v)",
					addr, host, from, enabled, tt.wantAddr, tt.wantHost, tt.wantFrom, tt.wantEnabled)
			}
		})
	}
}

func TestSendInBatches(t *testing.T) {
	items := func(n int) []Item {
		var out []Item
		for i := 0; i < n; i++ {
			out = append(out, Item{ID: string(rune('a' + i))})
		}
		return out
	}

	t.Run("fewer than one batch sends all without pausing", func(t *testing.T) {
		sentN := 0
		var paused []time.Duration
		n, _ := SendInBatches(context.Background(), &config.Config{}, items(3),
			func(it Item) error { sentN++; return nil },
			func(d time.Duration) { paused = append(paused, d) })
		if n != 3 || sentN != 3 || len(paused) != 0 {
			t.Errorf("sent=%d paused=%v; want 3 sends, no pause", n, paused)
		}
	})

	t.Run("more than one batch pauses between batches", func(t *testing.T) {
		cfg := &config.Config{OutreachBatchSize: 3, OutreachBatchPauseSec: 7}
		var sent []string
		var paused []time.Duration
		n, _ := SendInBatches(context.Background(), cfg, items(7),
			func(it Item) error { sent = append(sent, it.ID); return nil },
			func(d time.Duration) { paused = append(paused, d) })
		if n != 7 {
			t.Errorf("sent = %d; want 7", n)
		}
		if len(paused) != 2 { // 3|3|1 → two inter-batch pauses
			t.Errorf("pauses = %v; want 2 pauses", paused)
		}
		for _, d := range paused {
			if d != 7*time.Second {
				t.Errorf("pause duration = %v; want 7s", d)
			}
		}
	})

	t.Run("send errors are collected without stopping the batch", func(t *testing.T) {
		fail := map[string]bool{"b": true}
		var paused []time.Duration
		n, errs := SendInBatches(context.Background(), &config.Config{OutreachBatchSize: 2}, items(4),
			func(it Item) error {
				if fail[it.ID] {
					return errors.New("nope")
				}
				return nil
			},
			func(d time.Duration) { paused = append(paused, d) })
		if n != 3 {
			t.Errorf("sent = %d; want 3 (one failure)", n)
		}
		if len(errs) != 1 || errs[0].Error() != "nope" {
			t.Errorf("errs = %v; want [nope]", errs)
		}
		if len(paused) != 1 { // 2|2 → one pause, error didn't stop it
			t.Errorf("pauses = %v; want 1", paused)
		}
	})

	t.Run("context cancellation stops the run early", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		first := true
		n, errs := SendInBatches(ctx, &config.Config{OutreachBatchSize: 3}, items(9),
			func(it Item) error {
				if first {
					first = false
					cancel()
				}
				return nil
			},
			func(time.Duration) {})
		if n != 1 {
			t.Errorf("sent = %d; want 1 before cancellation", n)
		}
		if len(errs) == 0 {
			t.Error("cancellation must surface as an error")
		}
	})

	t.Run("empty input sends nothing", func(t *testing.T) {
		var sent, paused []time.Duration
		n, errs := SendInBatches(context.Background(), &config.Config{}, nil,
			func(Item) error { sent = append(sent, 0); return nil },
			func(d time.Duration) { paused = append(paused, d) })
		if n != 0 || len(sent) != 0 || len(paused) != 0 || len(errs) != 0 {
			t.Errorf("empty: sent=%d paused=%v errs=%v; want all zero", n, paused, errs)
		}
	})
}
