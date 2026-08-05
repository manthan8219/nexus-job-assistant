package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/manthan8219/nexus-job-assistant/internal/auth"
	"github.com/manthan8219/nexus-job-assistant/internal/engine"
	"github.com/manthan8219/nexus-job-assistant/internal/userstore"
)

// userReq builds a request that withAuth would have produced for userID.
func userReq(t *testing.T, s *Server, userID, email string) *http.Request {
	t.Helper()
	st, err := s.users.For(userID, email)
	if err != nil {
		t.Fatalf("users.For(%s): %v", userID, err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/mission", nil)
	ctx := auth.WithUser(r.Context(), auth.User{ID: userID, Email: email})
	r = r.WithContext(withUserState(ctx, st))
	return r
}

// countBoundedOpenState simulate two tenants touching their run states
// concurrently; -race must stay quiet and each user must keep their own state.
func TestPerUserRunStatesAreIsolatedAndRaceFree(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NEXUS_HOME", root)
	s := &Server{
		auth:  auth.New(testAuthSecret, "https://abc.supabase.co/auth/v1", ""),
		users: userstore.NewRegistry(filepath.Join(root, "users"), nil, 0),
		runs:  make(map[string]*runState),
	}
	defer s.users.Close()

	rA := userReq(t, s, "user-a", "alice@example.com")
	rB := userReq(t, s, "user-b", "bob@example.com")

	rsA, rsB := s.runFor(rA), s.runFor(rB)
	if rsA == rsB {
		t.Fatal("runFor(user-a) == runFor(user-b); want separate states")
	}
	if got := s.runFor(rA); got != rsA {
		t.Error("runFor(user-a) twice returned different states; want the same")
	}

	// Hammer both states from many goroutines: runFor, snapshot, logs, SSE.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { // user A
			defer wg.Done()
			rs := s.runFor(rA)
			rs.mu.RLock()
			_ = rs.status
			rs.mu.RUnlock()
			rs.appendLog("a")
			_ = s.missionSnapshotFor(rs)
		}()
		go func() { // user B
			defer wg.Done()
			rs := s.runFor(rB)
			rs.mu.RLock()
			_ = rs.status
			rs.mu.RUnlock()
			_ = s.missionSnapshotFor(rs)
		}()
	}
	wg.Wait()
}

// TestPerUserRunsAreIndependent proves two tenants can hold a run each at the
// same time and one user's busy state never blocks the other's run.
func TestPerUserRunsAreIndependent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NEXUS_HOME", root)
	s := &Server{
		auth:  auth.New(testAuthSecret, "https://abc.supabase.co/auth/v1", ""),
		users: userstore.NewRegistry(filepath.Join(root, "users"), nil, 0),
		runs:  make(map[string]*runState),
	}
	defer s.users.Close()

	rsA := s.runFor(userReq(t, s, "user-a", "a@x.com"))
	rsB := s.runFor(userReq(t, s, "user-b", "b@x.com"))

	// A no-op engine whose channels are closed so the drain loop exits; the
	// run closure signals completion through channels (no sleeps, no network).
	newNoop := func(done chan struct{}) *engine.Engine {
		return &engine.Engine{
			ResultCh:   make(chan engine.Result),
			LogCh:      make(chan string),
			ProgressCh: make(chan engine.ProviderProgress),
		}
	}
	runNoop := func(eng *engine.Engine, done chan struct{}) func(context.Context) error {
		return func(context.Context) error {
			close(done)
			close(eng.LogCh)
			close(eng.ResultCh)
			close(eng.ProgressCh)
			return nil
		}
	}

	doneA := make(chan struct{})
	wakeA := rsA.subscribe()
	defer rsA.unsubscribe(wakeA)
	engA := newNoop(doneA)
	if err := s.launchRun(rsA, engA, true, false, runNoop(engA, doneA)); err != nil {
		t.Fatalf("launch A: %v", err)
	}

	// A is busy, B is idle — and B can start its own run right now.
	rsA.mu.RLock()
	if rsA.status != StatusRunning {
		t.Errorf("A status = %q; want running", rsA.status)
	}
	rsA.mu.RUnlock()
	if err := s.launchRun(rsA, nil, true, false, nil); !errors.Is(err, errEngineBusy) {
		t.Errorf("second launch on A error = %v; want errEngineBusy", err)
	}

	doneB := make(chan struct{})
	engB := newNoop(doneB)
	if err := s.launchRun(rsB, engB, true, false, runNoop(engB, doneB)); err != nil {
		t.Fatalf("launch B while A busy: %v", err)
	}

	<-doneA
	<-wakeA // fired after A's status flipped to done
	<-doneB

	rsA.mu.RLock()
	rsB.mu.RLock()
	gotA, gotB := rsA.status, rsB.status
	rsA.mu.RUnlock()
	rsB.mu.RUnlock()
	if gotA != StatusDone {
		t.Errorf("A status = %q; want done", gotA)
	}
	if gotB != StatusDone {
		t.Errorf("B status = %q; want done", gotB)
	}
}
