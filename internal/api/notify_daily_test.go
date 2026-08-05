package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

func TestHandleGetNotifyChannels(t *testing.T) {
	s := &Server{cfg: &config.Config{NotifyChannels: []string{"discord", "telegram"}}}
	rec := httptest.NewRecorder()
	s.handleGetNotifyChannels(rec, httptest.NewRequest(http.MethodGet, "/api/notify/channels", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d; want 200", rec.Code)
	}
	var body []NotifierChannel
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("channels = 0; want the built-in channel list")
	}
	enabled := map[string]bool{}
	for _, ch := range body {
		enabled[ch.ID] = ch.Enabled
	}
	if !enabled["discord"] || !enabled["telegram"] {
		t.Errorf("enabled map = %v; want discord and telegram enabled", enabled)
	}
}

// TestScheduleDailyRunsCancelled verifies the scheduler exits promptly when its
// context is cancelled (its owner and exit path), instead of ticking forever.
func TestScheduleDailyRunsCancelled(t *testing.T) {
	s := &Server{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.scheduleDailyRuns(ctx) // must return immediately on a cancelled ctx
}
