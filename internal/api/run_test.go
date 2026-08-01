package api

import (
	"context"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/engine"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
)

// TestDrainEngineChannelsCountsFound verifies the drain loop counts discovery
// events into foundCount (the dashboard "jobs found this run" number).
func TestDrainEngineChannelsCountsFound(t *testing.T) {
	eng := &engine.Engine{
		ResultCh:   make(chan engine.Result, 16),
		LogCh:      make(chan string, 16),
		ProgressCh: make(chan engine.ProviderProgress, 16),
	}
	srv := &Server{eng: eng}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.drainEngineChannels(ctx)

	eng.ResultCh <- engine.Result{Job: provider.Job{Title: "A", Company: "X"}, Status: "found"}
	eng.ResultCh <- engine.Result{Job: provider.Job{Title: "B", Company: "Y"}, Status: "found"}
	eng.ResultCh <- engine.Result{Job: provider.Job{Title: "C", Company: "Z"}, Status: "applied"}
	eng.ResultCh <- engine.Result{Job: provider.Job{Title: "D", Company: "W"}, Status: "queued"}
	close(eng.ResultCh) // drain returns after the channel closes

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		srv.mu.RLock()
		done := srv.foundCount == 2 && srv.applied == 1
		srv.mu.RUnlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	srv.mu.RLock()
	defer srv.mu.RUnlock()
	if srv.foundCount != 2 {
		t.Errorf("foundCount = %d; want 2", srv.foundCount)
	}
	if srv.applied != 1 {
		t.Errorf("applied = %d; want 1", srv.applied)
	}
}
