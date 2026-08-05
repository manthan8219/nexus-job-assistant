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
	go srv.drainEngineChannels(ctx, eng, &srv.runState)

	eng.ResultCh <- engine.Result{Job: provider.Job{Title: "A", Company: "X"}, Status: "found"}
	eng.ResultCh <- engine.Result{Job: provider.Job{Title: "B", Company: "Y"}, Status: "found"}
	eng.ResultCh <- engine.Result{Job: provider.Job{Title: "C", Company: "Z"}, Status: "applied"}
	eng.ResultCh <- engine.Result{Job: provider.Job{Title: "D", Company: "W"}, Status: "queued"}
	close(eng.ResultCh) // drain returns after the channel closes

	rs := &srv.runState
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rs.mu.RLock()
		done := rs.foundCount == 2 && rs.applied == 1
		rs.mu.RUnlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if rs.foundCount != 2 {
		t.Errorf("foundCount = %d; want 2", rs.foundCount)
	}
	if rs.applied != 1 {
		t.Errorf("applied = %d; want 1", rs.applied)
	}
}
